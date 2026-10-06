// Package resources 提供 MCP 协议原生的只读资源端点与 RFC 6570 参数化资源模板实现。
//
// @author Ateng
// @since 2026-10-06
package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/pruning"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/yosida95/uritemplate/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// URIContexts 暴露可用集群上下文列表与激活状态资源 URI
	URIContexts = "k8s://contexts"

	// URIClusterOverview 暴露当前集群版本、节点及核心资源水位全景快照资源 URI
	URIClusterOverview = "k8s://cluster/overview"

	// URINamespaces 暴露集群所有命名空间及其状态资源 URI
	URINamespaces = "k8s://namespaces"

	// URIAPIResources 暴露集群已安装的全部 API 组与资源种类清单资源 URI
	URIAPIResources = "k8s://api-resources"

	// TemplatePods RFC 6570 参数化 Pod 实时上下文资源模板
	TemplatePods = "k8s://{namespace}/pods/{name}"

	// TemplateDeployments RFC 6570 参数化 Deployment 运行配置资源模板
	TemplateDeployments = "k8s://{namespace}/deployments/{name}"
)

var (
	podURITemplate    = uritemplate.MustNew(TemplatePods)
	deployURITemplate = uritemplate.MustNew(TemplateDeployments)
)

// ContextsPayload 强类型封装 k8s://contexts 资源数据契约
type ContextsPayload struct {
	CurrentContext string   `json:"currentContext"`
	Contexts       []string `json:"contexts"`
}

// ClusterOverviewPayload 强类型封装 k8s://cluster/overview 资源全景水位契约
type ClusterOverviewPayload struct {
	CurrentContext      string `json:"currentContext"`
	KubernetesVersion   string `json:"kubernetesVersion"`
	NodeCount           int    `json:"nodeCount"`
	ReadyNodeCount      int    `json:"readyNodeCount"`
	NamespaceCount      int    `json:"namespaceCount"`
	PodCount            int    `json:"podCount"`
	AllocatableCPUMilli int64  `json:"allocatableCpuMilli"`
	AllocatableMemBytes int64  `json:"allocatableMemBytes"`
}

// Register 向传入的 MCP Server 注册所有静态资源与动态参数化资源模板。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @return error 注册异常
func Register(s *mcpserver.MCPServer, mgr *k8s.ClientManager) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	// 1. 注册静态资源端点
	registerStaticResources(s, mgr)

	// 2. 注册参数化资源模板
	registerResourceTemplates(s, mgr)

	return nil
}

// registerStaticResources 注册 4 个全局只读静态资源
func registerStaticResources(s *mcpserver.MCPServer, mgr *k8s.ClientManager) {
	// 1. k8s://contexts
	s.AddResource(
		mcp.NewResource(
			URIContexts,
			"Kubernetes Contexts",
			mcp.WithResourceDescription("可用 Kubernetes 集群上下文列表与当前激活上下文"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			payload := ContextsPayload{
				CurrentContext: mgr.CurrentContext(),
				Contexts:       mgr.ListContexts(),
			}
			return renderPrunedResource(URIContexts, payload)
		},
	)

	// 2. k8s://cluster/overview
	s.AddResource(
		mcp.NewResource(
			URIClusterOverview,
			"Cluster Overview",
			mcp.WithResourceDescription("当前集群版本、节点总数与健康状况、核心资源水位快照"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			client, err := mgr.GetClient("")
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取 Kubernetes 客户端失败: %w", err)
			}

			// 获取集群版本
			versionStr := "unknown"
			serverVersion, err := client.Discovery().ServerVersion()
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取集群版本失败: %w", err)
			}
			if serverVersion != nil {
				versionStr = serverVersion.GitVersion
			}

			// 统计节点与可分配资源水位
			nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取集群节点列表失败: %w", err)
			}

			nodeCount := len(nodes.Items)
			readyNodeCount := 0
			var totalAllocatableCPU int64
			var totalAllocatableMem int64

			for _, n := range nodes.Items {
				for _, cond := range n.Status.Conditions {
					if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
						readyNodeCount++
						break
					}
				}
				totalAllocatableCPU += n.Status.Allocatable.Cpu().MilliValue()
				totalAllocatableMem += n.Status.Allocatable.Memory().Value()
			}

			// 统计命名空间
			namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取命名空间列表失败: %w", err)
			}

			// 统计 Pod 总数
			pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取集群 Pod 列表失败: %w", err)
			}

			overview := ClusterOverviewPayload{
				CurrentContext:      mgr.CurrentContext(),
				KubernetesVersion:   versionStr,
				NodeCount:           nodeCount,
				ReadyNodeCount:      readyNodeCount,
				NamespaceCount:      len(namespaces.Items),
				PodCount:            len(pods.Items),
				AllocatableCPUMilli: totalAllocatableCPU,
				AllocatableMemBytes: totalAllocatableMem,
			}

			return renderPrunedResource(URIClusterOverview, overview)
		},
	)

	// 3. k8s://namespaces
	s.AddResource(
		mcp.NewResource(
			URINamespaces,
			"Namespaces",
			mcp.WithResourceDescription("集群命名空间列表及活跃运行状态"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			client, err := mgr.GetClient("")
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取 Kubernetes 客户端失败: %w", err)
			}

			nsList, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("查询命名空间失败: %w", err)
			}

			return renderPrunedResource(URINamespaces, nsList)
		},
	)

	// 4. k8s://api-resources
	s.AddResource(
		mcp.NewResource(
			URIAPIResources,
			"API Resources",
			mcp.WithResourceDescription("集群已安装的全部 API 组与资源种类元数据清单"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			client, err := mgr.GetClient("")
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取 Kubernetes 客户端失败: %w", err)
			}

			resourceLists, err := client.Discovery().ServerPreferredResources()
			if err != nil && len(resourceLists) == 0 {
				return []mcp.ResourceContents{}, fmt.Errorf("查询 API Resources 失败: %w", err)
			}

			return renderPrunedResource(URIAPIResources, resourceLists)
		},
	)
}

// registerResourceTemplates 注册参数化资源模板
func registerResourceTemplates(s *mcpserver.MCPServer, mgr *k8s.ClientManager) {
	// 1. k8s://{namespace}/pods/{name}
	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			TemplatePods,
			"Pod Details",
			mcp.WithTemplateDescription("动态挂载指定命名空间下 Pod 的实时运行状态与诊断数据"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			namespace, name, err := matchResourceTemplate(podURITemplate, request.Params.URI, TemplatePods)
			if err != nil {
				return []mcp.ResourceContents{}, err
			}

			client, err := mgr.GetClient("")
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取 Kubernetes 客户端失败: %w", err)
			}

			pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("读取 Pod %s/%s 失败: %w", namespace, name, err)
			}

			return renderPrunedResource(request.Params.URI, pod)
		},
	)

	// 2. k8s://{namespace}/deployments/{name}
	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			TemplateDeployments,
			"Deployment Details",
			mcp.WithTemplateDescription("动态挂载指定命名空间下 Deployment 的配置规格与副本就绪状态"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			namespace, name, err := matchResourceTemplate(deployURITemplate, request.Params.URI, TemplateDeployments)
			if err != nil {
				return []mcp.ResourceContents{}, err
			}

			client, err := mgr.GetClient("")
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("获取 Kubernetes 客户端失败: %w", err)
			}

			deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return []mcp.ResourceContents{}, fmt.Errorf("读取 Deployment %s/%s 失败: %w", namespace, name, err)
			}

			return renderPrunedResource(request.Params.URI, deploy)
		},
	)
}

// matchResourceTemplate 解析 URI 中的 namespace 与 name 命名变量
func matchResourceTemplate(tmpl *uritemplate.Template, uri, templateStr string) (string, string, error) {
	matches := tmpl.Match(uri)
	if matches == nil {
		return "", "", fmt.Errorf("URI %q 不符合模板格式 %s", uri, templateStr)
	}

	namespace := matches.Get("namespace").String()
	name := matches.Get("name").String()
	if namespace == "" || name == "" {
		return "", "", fmt.Errorf("解析 URI %q 参数缺失", uri)
	}

	return namespace, name, nil
}

// renderPrunedResource 统一将资源对象经由 Smart Pruning 清洗并打包为 MCP TextResourceContents
func renderPrunedResource(uri string, obj any) ([]mcp.ResourceContents, error) {
	pruned, err := pruning.Prune(obj)
	if err != nil {
		return []mcp.ResourceContents{}, fmt.Errorf("清洗资源 %s 数据失败: %w", uri, err)
	}

	data, err := json.Marshal(pruned)
	if err != nil {
		return []mcp.ResourceContents{}, fmt.Errorf("序列化资源 %s 失败: %w", uri, err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
