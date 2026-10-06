// Package k8s_test 针对 Kubernetes 客户端管理器与动态 Context 路由开展单测。
//
// @author Ateng
// @since 2026-10-06
package k8s_test

import (
	"context"
	"errors"
	"testing"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// buildSampleKubeConfig 构造用于测试的多集群 clientcmdapi.Config
func buildSampleKubeConfig() *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "dev-cluster"

	cfg.Clusters["cluster-dev"] = &clientcmdapi.Cluster{Server: "https://dev.k8s.local:6443"}
	cfg.Clusters["cluster-prod"] = &clientcmdapi.Cluster{Server: "https://prod.k8s.local:6443"}

	cfg.AuthInfos["user-dev"] = &clientcmdapi.AuthInfo{Token: "dev-token"}
	cfg.AuthInfos["user-prod"] = &clientcmdapi.AuthInfo{Token: "prod-token"}

	cfg.Contexts["dev-cluster"] = &clientcmdapi.Context{
		Cluster:  "cluster-dev",
		AuthInfo: "user-dev",
	}
	cfg.Contexts["prod-cluster"] = &clientcmdapi.Context{
		Cluster:  "cluster-prod",
		AuthInfo: "user-prod",
	}

	return cfg
}

func TestManager_InClusterDetection(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()

	// 模拟未指定 KubeconfigPath 时优先 In-Cluster 探测
	mgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return fakeClient, nil
		}),
	)
	if err != nil {
		t.Fatalf("In-Cluster 模式初始化失败: %v", err)
	}

	if !mgr.IsInCluster() {
		t.Errorf("预期处于 In-Cluster 模式")
	}

	if mgr.CurrentContext() != k8s.InClusterContextName {
		t.Errorf("预期当前 Context 为 %q, 实际得到 %q", k8s.InClusterContextName, mgr.CurrentContext())
	}

	client, err := mgr.GetClient("")
	if err != nil {
		t.Fatalf("获取默认 In-Cluster 客户端失败: %v", err)
	}
	if client != fakeClient {
		t.Errorf("获取的客户端与注入的 FakeClientset 不一致")
	}
}

func TestManager_ExplicitKubeconfigOverridesInCluster(t *testing.T) {
	rawConfig := buildSampleKubeConfig()
	var loadedPath string
	inClusterCalled := false

	// 当显式传入 KubeconfigPath 时，即使环境支持 In-Cluster 也必须显式覆盖跳过探测
	mgr, err := k8s.NewClientManager(
		k8s.Config{
			KubeconfigPath: "/custom/kube/config",
			ContextName:    "prod-cluster",
		},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			inClusterCalled = true
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		}),
		k8s.WithRawConfigLoader(func(path string) (*clientcmdapi.Config, error) {
			loadedPath = path
			return rawConfig, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return fake.NewSimpleClientset(), nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	if inClusterCalled {
		t.Errorf("显式指定 KubeconfigPath 时不应触发 In-Cluster 探测")
	}
	if mgr.IsInCluster() {
		t.Errorf("预期处于 Out-of-Cluster 模式")
	}
	if loadedPath != "/custom/kube/config" {
		t.Errorf("预期加载的配置文件路径为 /custom/kube/config，实际为 %q", loadedPath)
	}
	if mgr.CurrentContext() != "prod-cluster" {
		t.Errorf("预期 ContextName 覆盖生效为 prod-cluster，实际为 %q", mgr.CurrentContext())
	}
}

func TestManager_FallbackToKubeConfig(t *testing.T) {
	rawConfig := buildSampleKubeConfig()
	createdClients := make(map[string]kubernetes.Interface)

	// 模拟 In-Cluster 探测失败，自动降级至本地 Kubeconfig
	mgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return nil, errors.New("not in cluster")
		}),
		k8s.WithRawConfigLoader(func(path string) (*clientcmdapi.Config, error) {
			return rawConfig, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			fc := fake.NewSimpleClientset()
			createdClients[rc.Host] = fc
			return fc, nil
		}),
	)
	if err != nil {
		t.Fatalf("回退本地 Kubeconfig 初始化失败: %v", err)
	}

	if mgr.IsInCluster() {
		t.Errorf("预期非 In-Cluster 模式")
	}

	if mgr.CurrentContext() != "dev-cluster" {
		t.Errorf("预期默认 Context 为 %q, 实际得到 %q", "dev-cluster", mgr.CurrentContext())
	}

	contexts := mgr.ListContexts()
	if len(contexts) != 2 {
		t.Fatalf("预期返回 2 个 Context, 实际得到 %d 个: %v", len(contexts), contexts)
	}

	// 验证空 Context 参数自动路由到当前默认 Context
	clientDev1, err := mgr.GetClient("")
	if err != nil {
		t.Fatalf("获取默认 Client 失败: %v", err)
	}

	// 验证缓存命中（同一个 Context 重复获取应返回同一实例）
	clientDev2, err := mgr.GetClient("dev-cluster")
	if err != nil {
		t.Fatalf("重复获取 dev-cluster 失败: %v", err)
	}
	if clientDev1 != clientDev2 {
		t.Errorf("预期相同 Context 复用连接池缓存实例")
	}

	// 验证动态路由到不同的 Context (prod-cluster)
	clientProd, err := mgr.GetClient("prod-cluster")
	if err != nil {
		t.Fatalf("动态路由 prod-cluster 失败: %v", err)
	}
	if clientProd == clientDev1 {
		t.Errorf("预期不同 Context 对应不同 Client 实例")
	}
}

func TestManager_InvalidContextError(t *testing.T) {
	rawConfig := buildSampleKubeConfig()

	mgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return nil, errors.New("not in cluster")
		}),
		k8s.WithRawConfigLoader(func(path string) (*clientcmdapi.Config, error) {
			return rawConfig, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return fake.NewSimpleClientset(), nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	_, err = mgr.GetClient("non-existent-cluster")
	if err == nil {
		t.Fatalf("预期查询不存在的 context 报错，但未报错")
	}
}

func TestManager_FakeClientset_Interactions(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()

	mgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return fakeClient, nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	client, err := mgr.GetClient("")
	if err != nil {
		t.Fatalf("获取 client 失败: %v", err)
	}

	// 验证通过 Interface 可调用真实 K8s API (测试 Fake Clientset 端到端可用性)
	nsList, err := client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Namespace 查询失败: %v", err)
	}
	if nsList == nil {
		t.Fatalf("查询结果不应为 nil")
	}
}
