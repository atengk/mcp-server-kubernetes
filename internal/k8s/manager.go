// Package k8s 封装 Kubernetes 客户端连接管理器，支持 In-Cluster 自动探测、本地 Kubeconfig 降级回退及多 Context 动态路由。
//
// @author Ateng
// @since 2026-10-06
package k8s

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

const (
	// InClusterContextName 标识集群内 In-Cluster 模式的专属上下文名称
	InClusterContextName = "in-cluster"
)

// Config 封装构建 ClientManager 所需的外部配置参数。
type Config struct {
	// KubeconfigPath 显式指定的 kubeconfig 配置文件路径（对应 --kubeconfig 参数）
	KubeconfigPath string

	// ContextName 显式覆盖的默认集群上下文名称（对应 --context 参数）
	ContextName string
}

// InClusterLoaderFunc 定义用于探测与加载集群内 In-Cluster 配置的函数原型。
type InClusterLoaderFunc func() (*rest.Config, error)

// RawConfigLoaderFunc 定义用于从文件加载原始 clientcmdapi.Config 的函数原型。
type RawConfigLoaderFunc func(path string) (*clientcmdapi.Config, error)

// ClientFactoryFunc 定义由 rest.Config 构建 kubernetes.Interface 的工厂函数。
type ClientFactoryFunc func(rc *rest.Config) (kubernetes.Interface, error)

type options struct {
	inClusterLoader InClusterLoaderFunc
	rawConfigLoader RawConfigLoaderFunc
	clientFactory   ClientFactoryFunc
}

// Option 定义配置 ClientManager 初始化特性的函数选项。
type Option func(*options)

// WithInClusterLoader 注入自定义 In-Cluster 配置探测器。
//
// @param fn 自定义 In-Cluster 配置探测函数
// @return Option 配置函数
func WithInClusterLoader(fn InClusterLoaderFunc) Option {
	return func(o *options) {
		o.inClusterLoader = fn
	}
}

// WithRawConfigLoader 注入自定义 Kubeconfig 原始配置加载器。
//
// @param fn 自定义 Kubeconfig 配置加载函数
// @return Option 配置函数
func WithRawConfigLoader(fn RawConfigLoaderFunc) Option {
	return func(o *options) {
		o.rawConfigLoader = fn
	}
}

// WithClientFactory 注入自定义 Kubernetes Typed Clientset 工厂函数。
//
// @param fn 自定义 Clientset 构建函数
// @return Option 配置函数
func WithClientFactory(fn ClientFactoryFunc) Option {
	return func(o *options) {
		o.clientFactory = fn
	}
}

// ClientManager 管理与一个或多个 Kubernetes 集群的连接池及动态路由。
type ClientManager struct {
	mu             sync.RWMutex
	cfg            Config
	opts           options
	isInCluster    bool
	inClusterRest  *rest.Config
	rawConfig      *clientcmdapi.Config
	currentContext string

	clientPool map[string]kubernetes.Interface
}

// NewClientManager 初始化 Kubernetes 客户端管理器，执行探测链路并在需要时加载 Kubeconfig。
//
// @param cfg 客户端配置对象
// @param opts 自定义选项，支持替换底层依赖以供单元测试
// @return *ClientManager 初始化的管理器实例
// @return error 探测与初始化失败时返回错误
func NewClientManager(cfg Config, opts ...Option) (*ClientManager, error) {
	mgrOpts := options{
		inClusterLoader: rest.InClusterConfig,
		rawConfigLoader: clientcmd.LoadFromFile,
		clientFactory:   func(rc *rest.Config) (kubernetes.Interface, error) { return kubernetes.NewForConfig(rc) },
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&mgrOpts)
		}
	}

	mgr := &ClientManager{
		cfg:        cfg,
		opts:       mgrOpts,
		clientPool: make(map[string]kubernetes.Interface),
	}

	// 1. 若未显式传入 KubeconfigPath，优先探测集群内 In-Cluster ServiceAccount
	if cfg.KubeconfigPath == "" {
		inClusterConfig, err := mgr.opts.inClusterLoader()
		if err == nil && inClusterConfig != nil {
			mgr.isInCluster = true
			mgr.inClusterRest = inClusterConfig
			mgr.currentContext = InClusterContextName
			return mgr, nil
		}
	}

	// 2. 显式指定配置或探测失败时，降级回退至 Out-of-Cluster (Kubeconfig) 模式
	resolvedPath := resolveKubeconfigPath(cfg.KubeconfigPath)
	rawCfg, err := mgr.opts.rawConfigLoader(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("加载 kubeconfig 配置失败 (%s): %w", resolvedPath, err)
	}
	if rawCfg == nil || len(rawCfg.Contexts) == 0 {
		return nil, errors.New("kubeconfig 中未发现任何有效的 context 配置")
	}

	mgr.rawConfig = rawCfg

	// 3. 判定默认使用的集群上下文（优先显式覆盖，其次使用 kubeconfig 默认）
	if cfg.ContextName != "" {
		if _, exists := rawCfg.Contexts[cfg.ContextName]; !exists {
			return nil, fmt.Errorf("指定的默认上下文 %q 在 kubeconfig 中不存在", cfg.ContextName)
		}
		mgr.currentContext = cfg.ContextName
	} else if rawCfg.CurrentContext != "" {
		mgr.currentContext = rawCfg.CurrentContext
	} else {
		allContexts := mgr.ListContexts()
		if len(allContexts) > 0 {
			mgr.currentContext = allContexts[0]
		}
	}

	return mgr, nil
}

// IsInCluster 返回当前是否运行于集群内 In-Cluster 模式。
//
// @return bool 为 true 表示处于 In-Cluster 模式
func (m *ClientManager) IsInCluster() bool {
	return m.isInCluster
}

// CurrentContext 返回当前默认生效的 Context 名称。
//
// @return string Context 标识符
func (m *ClientManager) CurrentContext() string {
	return m.currentContext
}

// ListContexts 返回当前已加载的全部集群 Context 名称列表（排序输出，无配置时返回空切片）。
//
// @return []string Context 名称列表
func (m *ClientManager) ListContexts() []string {
	if m.isInCluster {
		return []string{InClusterContextName}
	}

	if m.rawConfig == nil || len(m.rawConfig.Contexts) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(m.rawConfig.Contexts))
	for name := range m.rawConfig.Contexts {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// GetClient 根据传入的 contextName 动态路由并获取对应的 Kubernetes Typed Clientset。
// 若 contextName 为空，自动使用当前默认 Context；连接池保证并发安全复用。
//
// @param contextName 目标集群上下文名称（可选）
// @return kubernetes.Interface Kubernetes 客户端接口
// @return error 目标 Context 不存在或初始化失败时返回错误
func (m *ClientManager) GetClient(contextName string) (kubernetes.Interface, error) {
	ctxName := m.resolveTargetContext(contextName)

	m.mu.RLock()
	if c, ok := m.clientPool[ctxName]; ok {
		m.mu.RUnlock()
		return c, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查锁定，避免并发重复创建
	if c, ok := m.clientPool[ctxName]; ok {
		return c, nil
	}

	rc, err := m.buildRESTConfig(ctxName)
	if err != nil {
		return nil, err
	}

	client, err := m.opts.clientFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("创建 Context %q 的 Kubernetes 客户端失败: %w", ctxName, err)
	}

	m.clientPool[ctxName] = client
	return client, nil
}

// resolveTargetContext 解析实际使用的 Context 名称。
func (m *ClientManager) resolveTargetContext(contextName string) string {
	if contextName == "" {
		return m.currentContext
	}
	return contextName
}

// buildRESTConfig 根据 Context 名称组装 *rest.Config。
func (m *ClientManager) buildRESTConfig(ctxName string) (*rest.Config, error) {
	if m.isInCluster {
		if ctxName != "" && ctxName != InClusterContextName {
			return nil, fmt.Errorf("当前处于 In-Cluster 模式，不支持动态路由至非本地 Context %q", ctxName)
		}
		return m.inClusterRest, nil
	}

	if m.rawConfig == nil {
		return nil, errors.New("Kubeconfig 配置未初始化")
	}

	if _, exists := m.rawConfig.Contexts[ctxName]; !exists {
		return nil, fmt.Errorf("未找到指定的集群上下文: %q", ctxName)
	}

	clientConfig := clientcmd.NewNonInteractiveClientConfig(
		*m.rawConfig,
		ctxName,
		&clientcmd.ConfigOverrides{},
		nil,
	)

	rc, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("构建 Context %q 的 REST 配置失败: %w", ctxName, err)
	}

	return rc, nil
}

// resolveKubeconfigPath 寻找最终生效的 Kubeconfig 路径。
func resolveKubeconfigPath(customPath string) string {
	if customPath != "" {
		return customPath
	}

	if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
		return envPath
	}

	home := homedir.HomeDir()
	if home != "" {
		return filepath.Join(home, ".kube", "config")
	}

	return ""
}
