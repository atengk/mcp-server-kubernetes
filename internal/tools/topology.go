// Package tools 提供 Kubernetes 级联拓扑树下钻与层级关联健康度分析工具。
//
// @author Ateng
// @since 2026-10-06
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/pruning"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// ResourceNode 表示 Kubernetes 级联拓扑树中的单一资源节点。
type ResourceNode struct {
	// Kind 资源对象类型（如 Deployment, ReplicaSet, Pod, Service, Endpoints）
	Kind string `json:"kind"`

	// Name 资源对象名称
	Name string `json:"name"`

	// Namespace 所在命名空间
	Namespace string `json:"namespace,omitempty"`

	// Status 聚合健康度与运行时状态（如 Running (Ready), Pending, Ready: 1/2）
	Status string `json:"status,omitempty"`

	// Info 附加的简明描述信息（如副本数、IP 端口等）
	Info string `json:"info,omitempty"`

	// Children 下属级联子资源节点列表（遵循空安全契约，无子项时保证为空切片而非 nil）
	Children []*ResourceNode `json:"children"`
}

// RegisterTopologyTool 向 MCP Server 注册级联资源拓扑树下钻工具。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @return error 注册异常
func RegisterTopologyTool(s *mcpserver.MCPServer, mgr *k8s.ClientManager) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	s.AddTool(
		mcp.NewTool("k8s_get_resource_tree",
			mcp.WithDescription("以指定资源为根节点递归构建其下属级联依赖拓扑树（支持 Deployment->RS->Pod、Service->Endpoints/Pods 等）并汇总各层级健康度"),
			mcp.WithString("kind", mcp.Required(), mcp.Description("根资源种类 (如 deployment, replicaset, service, pod)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("根资源名称")),
			mcp.WithString("namespace", mcp.Description("命名空间 (默认为 default)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeGetResourceTreeHandler(mgr),
	)

	return nil
}

// makeGetResourceTreeHandler 构建生成资源拓扑树的 MCP 工具处理器。
//
// @param mgr Kubernetes 客户端管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeGetResourceTreeHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		kind, err := request.RequireString("kind")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 kind"), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 name"), nil
		}
		kind = strings.TrimSpace(kind)
		name = strings.TrimSpace(name)

		namespace := strings.TrimSpace(request.GetString("namespace", "default"))
		contextName := strings.TrimSpace(request.GetString("context", ""))

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		treeNode, err := buildTopologyTree(ctx, client, canonicalKind(kind), namespace, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("构建拓扑树失败: %v", err)), nil
		}

		// 接入 Smart Pruning 剔除噪音节点与脱敏
		pruned, err := pruning.Prune(treeNode)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗拓扑树数据失败: %v", err)), nil
		}

		jsonBytes, err := json.Marshal(pruned)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化拓扑树失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}

// buildTopologyTree 根据传入的根资源类型与名称递归构建完整的拓扑树节点。
//
// @param ctx 上下文
// @param client Kubernetes 集群客户端接口
// @param kind 标准 Kind
// @param namespace 命名空间
// @param name 资源名称
// @return *ResourceNode 构建完成的树形根节点
// @return error 资源查询失败错误
func buildTopologyTree(ctx context.Context, client kubernetes.Interface, kind, namespace, name string) (*ResourceNode, error) {
	switch kind {
	case "Deployment":
		return buildDeploymentTree(ctx, client, namespace, name)
	case "ReplicaSet":
		return buildReplicaSetTree(ctx, client, namespace, name)
	case "Service":
		return buildServiceTree(ctx, client, namespace, name)
	case "Pod":
		return buildPodNode(ctx, client, namespace, name)
	default:
		return nil, fmt.Errorf("暂不支持以资源类型 %q 作为拓扑树根节点 (支持 Deployment, ReplicaSet, Service, Pod)", kind)
	}
}

// buildDeploymentTree 构建以 Deployment 为根的级联拓扑树 (Deployment -> ReplicaSets -> Pods)。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param namespace 命名空间
// @param name Deployment 名称
// @return *ResourceNode 构建完成的根节点
// @return error 查询失败错误
func buildDeploymentTree(ctx context.Context, client kubernetes.Interface, namespace, name string) (*ResourceNode, error) {
	deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 Deployment 失败: %w", err)
	}

	root := &ResourceNode{
		Kind:      "Deployment",
		Name:      deploy.Name,
		Namespace: deploy.Namespace,
		Status:    fmt.Sprintf("Ready: %d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas),
		Info:      fmt.Sprintf("Desired: %d, Updated: %d, Available: %d", deploy.Status.Replicas, deploy.Status.UpdatedReplicas, deploy.Status.AvailableReplicas),
		Children:  make([]*ResourceNode, 0),
	}

	// 获取下属关联的 ReplicaSets
	rsList, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("列出关联 ReplicaSets 失败: %w", err)
	}

	for _, rs := range rsList.Items {
		matched := false
		if len(rs.OwnerReferences) > 0 {
			matched = isOwnedBy(&rs.ObjectMeta, string(deploy.UID))
		} else {
			matched = matchSelector(rs.Labels, deploy.Spec.Selector)
		}
		if matched {
			rsNode, err := buildReplicaSetNodeWithRS(ctx, client, &rs)
			if err == nil && rsNode != nil {
				root.Children = append(root.Children, rsNode)
			}
		}
	}

	return root, nil
}

// buildReplicaSetTree 构建以指定 ReplicaSet 为根的拓扑树。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param namespace 命名空间
// @param name ReplicaSet 名称
// @return *ResourceNode 拓扑树根节点
// @return error 查询错误
func buildReplicaSetTree(ctx context.Context, client kubernetes.Interface, namespace, name string) (*ResourceNode, error) {
	rs, err := client.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 ReplicaSet 失败: %w", err)
	}
	return buildReplicaSetNodeWithRS(ctx, client, rs)
}

// buildReplicaSetNodeWithRS 根据已获取的 ReplicaSet 构建其自身及下属 Pods。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param rs ReplicaSet 原生对象
// @return *ResourceNode 节点对象
// @return error Pod 查询错误
func buildReplicaSetNodeWithRS(ctx context.Context, client kubernetes.Interface, rs *appsv1.ReplicaSet) (*ResourceNode, error) {
	node := &ResourceNode{
		Kind:      "ReplicaSet",
		Name:      rs.Name,
		Namespace: rs.Namespace,
		Status:    fmt.Sprintf("Ready: %d/%d", rs.Status.ReadyReplicas, rs.Status.Replicas),
		Info:      fmt.Sprintf("Replicas: %d", rs.Status.Replicas),
		Children:  make([]*ResourceNode, 0),
	}

	// 检索归属于该 ReplicaSet 的所有 Pods
	listOpts := metav1.ListOptions{}
	if rs.Spec.Selector != nil {
		if sel, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector); err == nil && !sel.Empty() {
			listOpts.LabelSelector = sel.String()
		}
	}
	podList, err := client.CoreV1().Pods(rs.Namespace).List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("列出关联 Pods 失败: %w", err)
	}

	for _, p := range podList.Items {
		matched := false
		if len(p.OwnerReferences) > 0 {
			matched = isOwnedBy(&p.ObjectMeta, string(rs.UID))
		} else {
			matched = matchSelector(p.Labels, rs.Spec.Selector)
		}
		if matched {
			podNode := formatPodNode(&p)
			node.Children = append(node.Children, podNode)
		}
	}

	return node, nil
}

// buildServiceTree 构建以 Service 为根的级联拓扑树 (Service -> Endpoints / Pods)。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param namespace 命名空间
// @param name Service 名称
// @return *ResourceNode 拓扑树根节点
// @return error 查询错误
func buildServiceTree(ctx context.Context, client kubernetes.Interface, namespace, name string) (*ResourceNode, error) {
	svc, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 Service 失败: %w", err)
	}

	root := &ResourceNode{
		Kind:      "Service",
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Status:    string(svc.Spec.Type),
		Info:      fmt.Sprintf("ClusterIP: %s", svc.Spec.ClusterIP),
		Children:  make([]*ResourceNode, 0),
	}

	// 1. 关联检索 Endpoints
	ep, err := client.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil && ep != nil {
		readyCount := 0
		for _, s := range ep.Subsets {
			readyCount += len(s.Addresses)
		}
		epNode := &ResourceNode{
			Kind:      "Endpoints",
			Name:      ep.Name,
			Namespace: ep.Namespace,
			Status:    fmt.Sprintf("%d endpoints ready", readyCount),
			Children:  make([]*ResourceNode, 0),
		}
		root.Children = append(root.Children, epNode)
	}

	// 2. 根据 Service Spec.Selector 查询后端关联 Pods
	if len(svc.Spec.Selector) > 0 {
		selector := labels.SelectorFromSet(svc.Spec.Selector).String()
		podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err == nil && podList != nil {
			for _, p := range podList.Items {
				root.Children = append(root.Children, formatPodNode(&p))
			}
		}
	}

	return root, nil
}

// buildPodNode 单个 Pod 节点构建入口。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param namespace 命名空间
// @param name Pod 名称
// @return *ResourceNode Pod 节点对象
// @return error 获取错误
func buildPodNode(ctx context.Context, client kubernetes.Interface, namespace, name string) (*ResourceNode, error) {
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 Pod 失败: %w", err)
	}
	return formatPodNode(pod), nil
}

// formatPodNode 解析并格式化 Pod 的健康度及运行状态节点。
//
// @param pod Kubernetes 原生 Pod 对象
// @return *ResourceNode 格式化的树节点
func formatPodNode(pod *corev1.Pod) *ResourceNode {
	status := string(pod.Status.Phase)
	isReady := false
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			isReady = true
			break
		}
	}

	if pod.Status.Phase == corev1.PodRunning {
		if isReady {
			status = "Running (Ready)"
		} else {
			status = "Running (NotReady)"
		}
	}

	return &ResourceNode{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    status,
		Info:      fmt.Sprintf("IP: %s, Node: %s", pod.Status.PodIP, pod.Spec.NodeName),
		Children:  make([]*ResourceNode, 0),
	}
}

// isOwnedBy 判断资源元数据中是否包含指定 UID 的控制器所有者。
//
// @param meta 待检查的资源元数据
// @param targetUID 控制器属主的 UID
// @return bool 为 true 表示受该 UID 控制管辖
func isOwnedBy(meta *metav1.ObjectMeta, targetUID string) bool {
	if meta == nil || targetUID == "" {
		return false
	}
	for _, owner := range meta.OwnerReferences {
		if string(owner.UID) == targetUID {
			return true
		}
	}
	return false
}

// matchSelector 判定标签集是否与 LabelSelector 匹配（支持 MatchLabels 与 MatchExpressions）。
//
// @param targetLabels 目标资源现有标签字典
// @param selector 待匹配的标签选择器
// @return bool 为 true 表示标签符合匹配条件
func matchSelector(targetLabels map[string]string, selector *metav1.LabelSelector) bool {
	if selector == nil {
		return false
	}
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil || sel.Empty() {
		return false
	}
	return sel.Matches(labels.Set(targetLabels))
}
