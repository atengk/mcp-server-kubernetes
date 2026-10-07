// Package tools 封装面向 Kubernetes 核心排障诊断的 MCP 工具集与 Token Guard 截断防御。
//
// @author Ateng
// @since 2026-10-06
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/pruning"
	"github.com/atengk/mcp-server-kubernetes/internal/safety"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodLogOptions 封装获取 Pod 容器日志的查询参数选项。
type PodLogOptions struct {
	Namespace string
	Name      string
	Container string
	Tail      int
	Previous  bool
}

// PodLogGetterFunc 定义获取 Pod 容器日志的函数原型。
type PodLogGetterFunc func(ctx context.Context, client kubernetes.Interface, opts PodLogOptions) (string, error)

type toolOptions struct {
	podLogGetter PodLogGetterFunc
}

// Option 定义定制工具集特性的函数选项。
type Option func(*toolOptions)

// WithPodLogGetter 注入自定义 Pod 日志获取器（用于单元测试模拟日志输出流）。
//
// @param fn 日志获取函数
// @return Option 配置函数
func WithPodLogGetter(fn PodLogGetterFunc) Option {
	return func(o *toolOptions) {
		o.podLogGetter = fn
	}
}

// RegisterDiagnosticTools 向 MCP Server 注册五大核心只读诊断工具。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @param opts 可选配置函数
// @return error 注册异常
func RegisterDiagnosticTools(s *mcpserver.MCPServer, mgr *k8s.ClientManager, opts ...Option) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	opt := toolOptions{
		podLogGetter: defaultPodLogGetter,
	}
	for _, o := range opts {
		if o != nil {
			o(&opt)
		}
	}

	// 1. k8s_list_resources
	s.AddTool(
		mcp.NewTool("k8s_list_resources",
			mcp.WithDescription("列出指定类型的 Kubernetes 资源列表，支持命名空间与标签选择器过滤，内置 100 项安全截断保护"),
			mcp.WithString("kind", mcp.Required(), mcp.Description("资源种类 (如 pods, services, deployments, configmaps, secrets, nodes, namespaces, events)")),
			mcp.WithString("namespace", mcp.Description("命名空间 (未指定则查询所有命名空间)")),
			mcp.WithString("labelSelector", mcp.Description("Kubernetes 标签选择器 (如 app=nginx)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeListResourcesHandler(mgr),
	)

	// 2. k8s_get_resource
	s.AddTool(
		mcp.NewTool("k8s_get_resource",
			mcp.WithDescription("获取单个 Kubernetes 资源详情，自动执行 Smart Pruning 降噪与 Secret 脱敏"),
			mcp.WithString("kind", mcp.Required(), mcp.Description("资源种类 (如 pod, service, deployment, configmap, secret, node, namespace)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("资源名称")),
			mcp.WithString("namespace", mcp.Description("命名空间")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeGetResourceHandler(mgr),
	)

	// 3. k8s_describe_resource
	s.AddTool(
		mcp.NewTool("k8s_describe_resource",
			mcp.WithDescription("获取资源的聚合诊断视图（包含资源状态、关键条件与关联排障事件）"),
			mcp.WithString("kind", mcp.Required(), mcp.Description("资源种类 (如 pod, deployment, service)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("资源名称")),
			mcp.WithString("namespace", mcp.Description("命名空间")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeDescribeResourceHandler(mgr),
	)

	// 4. k8s_get_pod_logs
	s.AddTool(
		mcp.NewTool("k8s_get_pod_logs",
			mcp.WithDescription("获取 Pod 容器的标准输出与错误日志，内置 1000 行 Token Guard 硬性截断守卫"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Pod 名称")),
			mcp.WithString("namespace", mcp.Description("命名空间 (默认为 default)")),
			mcp.WithString("container", mcp.Description("容器名称 (多容器 Pod 建议指定)")),
			mcp.WithInteger("tail", mcp.Description("查看的日志末尾行数 (默认 100，上限 1000)")),
			mcp.WithBoolean("previous", mcp.Description("是否查看上一次崩溃重启前的历史日志")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeGetPodLogsHandler(mgr, opt.podLogGetter),
	)

	// 5. k8s_get_events
	s.AddTool(
		mcp.NewTool("k8s_get_events",
			mcp.WithDescription("检索集群事件，支持按命名空间、关联对象与事件级别过滤"),
			mcp.WithString("namespace", mcp.Description("命名空间")),
			mcp.WithString("involvedObjectName", mcp.Description("关联对象名称 (如 pod 名称)")),
			mcp.WithString("involvedObjectKind", mcp.Description("关联对象类型 (如 Pod, Node)")),
			mcp.WithString("type", mcp.Description("事件类型 (Warning 或 Normal)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeGetEventsHandler(mgr),
	)

	return nil
}

// makeListResourcesHandler 构建列出 Kubernetes 资源列表的 MCP Handler。
//
// @param mgr Kubernetes 客户端连接池管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeListResourcesHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		kind, err := request.RequireString("kind")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 kind"), nil
		}
		kind = strings.TrimSpace(kind)

		namespace := strings.TrimSpace(request.GetString("namespace", ""))
		labelSelector := strings.TrimSpace(request.GetString("labelSelector", ""))
		contextName := strings.TrimSpace(request.GetString("context", ""))

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		items, err := queryResourceList(ctx, client, kind, namespace, labelSelector)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("查询资源失败: %v", err)), nil
		}

		totalCount := len(items)
		truncatedItems, wasTruncated := safety.TruncateItems(items, safety.MaxListItems)

		pruned, err := pruning.Prune(truncatedItems)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗资源数据失败: %v", err)), nil
		}
		if pruned == nil {
			pruned = []any{}
		}

		jsonBytes, err := json.Marshal(pruned)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化结果失败: %v", err)), nil
		}

		outText := string(jsonBytes)
		if wasTruncated {
			outText += safety.FormatTruncationWarning(fmt.Sprintf("返回结果已截断至前 %d 项（共 %d 项），请使用 namespace 或 labelSelector 缩小查询范围", safety.MaxListItems, totalCount))
		}

		return mcp.NewToolResultText(outText), nil
	}
}

// makeGetResourceHandler 构建获取单个 Kubernetes 资源详情的 MCP Handler。
//
// @param mgr Kubernetes 客户端连接池管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeGetResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
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

		res, err := querySingleResource(ctx, client, kind, namespace, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取资源 %s/%s 失败: %v", kind, name, err)), nil
		}

		pruned, err := pruning.Prune(res)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗资源数据失败: %v", err)), nil
		}

		jsonBytes, err := json.Marshal(pruned)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}

// makeDescribeResourceHandler 构建获取 Kubernetes 资源聚合诊断视图的 MCP Handler。
//
// @param mgr Kubernetes 客户端连接池管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeDescribeResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
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

		res, err := querySingleResource(ctx, client, kind, namespace, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取资源失败: %v", err)), nil
		}

		// 关联查询匹配的集群排障事件（采用 canonicalKind 兼容资源别名）
		targetKind := canonicalKind(kind)
		relatedEvents := make([]corev1.Event, 0)
		eventsList, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err == nil && eventsList != nil {
			for _, ev := range eventsList.Items {
				if ev.InvolvedObject.Name == name && strings.EqualFold(ev.InvolvedObject.Kind, targetKind) {
					relatedEvents = append(relatedEvents, ev)
				}
			}
		}

		// 资源与关联事件分别清洗降噪，保障底层 Secret 脱敏与 managedFields 剔除有效生效
		prunedRes, err := pruning.Prune(res)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗资源数据失败: %v", err)), nil
		}
		prunedEvents, err := pruning.Prune(relatedEvents)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗关联事件失败: %v", err)), nil
		}
		if prunedEvents == nil {
			prunedEvents = []any{}
		}

		describeView := map[string]any{
			"resource": prunedRes,
			"events":   prunedEvents,
		}

		jsonBytes, err := json.Marshal(describeView)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化诊断数据失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}

// makeGetPodLogsHandler 构建获取 Pod 容器实时日志与历史日志的 MCP Handler。
//
// @param mgr Kubernetes 客户端连接池管理器
// @param logGetter 日志获取回调函数
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeGetPodLogsHandler(mgr *k8s.ClientManager, logGetter PodLogGetterFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 name"), nil
		}
		name = strings.TrimSpace(name)

		namespace := strings.TrimSpace(request.GetString("namespace", "default"))
		container := strings.TrimSpace(request.GetString("container", ""))
		tail := request.GetInt("tail", safety.DefaultLogTail)
		previous := request.GetBool("previous", false)
		contextName := strings.TrimSpace(request.GetString("context", ""))

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		wasTruncated := false
		if tail > safety.MaxLogTail {
			tail = safety.MaxLogTail
			wasTruncated = true
		}

		logOpts := PodLogOptions{
			Namespace: namespace,
			Name:      name,
			Container: container,
			Tail:      tail,
			Previous:  previous,
		}

		rawLogs, err := logGetter(ctx, client, logOpts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取日志失败: %v", err)), nil
		}

		// 二次防线：针对返回日志字符串实施物理文本行截断
		safeLogs, linesTruncated := safety.TruncateLogLines(rawLogs, safety.MaxLogTail)
		if wasTruncated || linesTruncated {
			safeLogs += safety.FormatTruncationWarning(fmt.Sprintf("请求行数已达 Token Guard 上限，强制截断为最近 %d 行", safety.MaxLogTail))
		}

		return mcp.NewToolResultText(safeLogs), nil
	}
}

// makeGetEventsHandler 构建检索过滤 Kubernetes 集群事件的 MCP Handler。
//
// @param mgr Kubernetes 客户端连接池管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeGetEventsHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := strings.TrimSpace(request.GetString("namespace", ""))
		objName := strings.TrimSpace(request.GetString("involvedObjectName", ""))
		objKind := strings.TrimSpace(request.GetString("involvedObjectKind", ""))
		eventType := strings.TrimSpace(request.GetString("type", ""))
		contextName := strings.TrimSpace(request.GetString("context", ""))

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		eventsList, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("检索事件失败: %v", err)), nil
		}

		filtered := make([]corev1.Event, 0)
		for _, ev := range eventsList.Items {
			if objName != "" && ev.InvolvedObject.Name != objName {
				continue
			}
			if objKind != "" && !strings.EqualFold(ev.InvolvedObject.Kind, canonicalKind(objKind)) {
				continue
			}
			if eventType != "" && !strings.EqualFold(ev.Type, eventType) {
				continue
			}
			filtered = append(filtered, ev)
		}

		totalCount := len(filtered)
		truncated, wasTruncated := safety.TruncateItems(filtered, safety.MaxListItems)

		pruned, err := pruning.Prune(truncated)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗事件数据失败: %v", err)), nil
		}
		if pruned == nil {
			pruned = []any{}
		}

		jsonBytes, err := json.Marshal(pruned)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化事件失败: %v", err)), nil
		}

		outText := string(jsonBytes)
		if wasTruncated {
			outText += safety.FormatTruncationWarning(fmt.Sprintf("事件列表已截断至前 %d 项（共 %d 项）", safety.MaxListItems, totalCount))
		}

		return mcp.NewToolResultText(outText), nil
	}
}

// canonicalKind 将用户输入的资源种类别名与缩写统一归一化为标准的 Kubernetes PascalCase Kind。
//
// @param kind 用户传入的资源类型名称（如 pods, po, deploy, svc, sts, ds, cj, ing, pvc, pv）
// @return string 标准的 Kubernetes Kind（如 Pod, Deployment, Service, StatefulSet 等）
func canonicalKind(kind string) string {
	trimmed := strings.TrimSpace(kind)
	switch strings.ToLower(trimmed) {
	case "pods", "pod", "po":
		return "Pod"
	case "services", "service", "svc":
		return "Service"
	case "deployments", "deployment", "deploy":
		return "Deployment"
	case "statefulsets", "statefulset", "sts":
		return "StatefulSet"
	case "daemonsets", "daemonset", "ds":
		return "DaemonSet"
	case "jobs", "job":
		return "Job"
	case "cronjobs", "cronjob", "cj":
		return "CronJob"
	case "ingresses", "ingress", "ing":
		return "Ingress"
	case "persistentvolumeclaims", "persistentvolumeclaim", "pvc":
		return "PersistentVolumeClaim"
	case "persistentvolumes", "persistentvolume", "pv":
		return "PersistentVolume"
	case "configmaps", "configmap", "cm":
		return "ConfigMap"
	case "secrets", "secret":
		return "Secret"
	case "namespaces", "namespace", "ns":
		return "Namespace"
	case "nodes", "node", "no":
		return "Node"
	case "events", "event", "ev":
		return "Event"
	default:
		return trimmed
	}
}

// queryResourceList 根据资源类型与过滤条件获取资源列表，并自动补充 Kind 与 APIVersion 元数据。
//
// @param ctx 上下文
// @param client Kubernetes 集群客户端接口
// @param kind 目标资源种类
// @param namespace 命名空间（空字符串表示跨命名空间查询）
// @param labelSelector 标签选择器
// @return []any 资源对象切片（无匹配时严格返回空切片，绝不返回 nil）
// @return error 查询失败或不支持的资源类型错误
func queryResourceList(ctx context.Context, client kubernetes.Interface, kind, namespace, labelSelector string) ([]any, error) {
	opts := metav1.ListOptions{LabelSelector: labelSelector}
	res := make([]any, 0)

	switch canonicalKind(kind) {
	case "Pod":
		list, err := client.CoreV1().Pods(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Pod"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "Service":
		list, err := client.CoreV1().Services(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Service"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "Deployment":
		list, err := client.AppsV1().Deployments(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Deployment"
			item.APIVersion = "apps/v1"
			res = append(res, item)
		}
	case "StatefulSet":
		list, err := client.AppsV1().StatefulSets(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "StatefulSet"
			item.APIVersion = "apps/v1"
			res = append(res, item)
		}
	case "DaemonSet":
		list, err := client.AppsV1().DaemonSets(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "DaemonSet"
			item.APIVersion = "apps/v1"
			res = append(res, item)
		}
	case "Job":
		list, err := client.BatchV1().Jobs(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Job"
			item.APIVersion = "batch/v1"
			res = append(res, item)
		}
	case "CronJob":
		list, err := client.BatchV1().CronJobs(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "CronJob"
			item.APIVersion = "batch/v1"
			res = append(res, item)
		}
	case "Ingress":
		list, err := client.NetworkingV1().Ingresses(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Ingress"
			item.APIVersion = "networking.k8s.io/v1"
			res = append(res, item)
		}
	case "PersistentVolumeClaim":
		list, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "PersistentVolumeClaim"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "PersistentVolume":
		list, err := client.CoreV1().PersistentVolumes().List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "PersistentVolume"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "ConfigMap":
		list, err := client.CoreV1().ConfigMaps(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "ConfigMap"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "Secret":
		list, err := client.CoreV1().Secrets(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Secret"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "Namespace":
		list, err := client.CoreV1().Namespaces().List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Namespace"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "Node":
		list, err := client.CoreV1().Nodes().List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Node"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	case "Event":
		list, err := client.CoreV1().Events(namespace).List(ctx, opts)
		if err != nil {
			return []any{}, err
		}
		for _, item := range list.Items {
			item.Kind = "Event"
			item.APIVersion = "v1"
			res = append(res, item)
		}
	default:
		return []any{}, fmt.Errorf("不支持的资源类型: %q (支持 pods, services, deployments, statefulsets, daemonsets, jobs, cronjobs, ingresses, persistentvolumeclaims, persistentvolumes, configmaps, secrets, nodes, namespaces, events)", kind)
	}

	if len(res) == 0 {
		return []any{}, nil
	}
	return res, nil
}

// querySingleResource 获取单个特定名称的 Kubernetes 资源对象并补全 TypeMeta。
//
// @param ctx 上下文
// @param client Kubernetes 集群客户端接口
// @param kind 目标资源种类
// @param namespace 命名空间
// @param name 资源名称
// @return any 资源强类型对象
// @return error 查询失败或不支持的资源类型错误
func querySingleResource(ctx context.Context, client kubernetes.Interface, kind, namespace, name string) (any, error) {
	opts := metav1.GetOptions{}

	switch canonicalKind(kind) {
	case "Pod":
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		pod.Kind = "Pod"
		pod.APIVersion = "v1"
		return pod, nil
	case "Service":
		svc, err := client.CoreV1().Services(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		svc.Kind = "Service"
		svc.APIVersion = "v1"
		return svc, nil
	case "Deployment":
		deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		deploy.Kind = "Deployment"
		deploy.APIVersion = "apps/v1"
		return deploy, nil
	case "StatefulSet":
		sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		sts.Kind = "StatefulSet"
		sts.APIVersion = "apps/v1"
		return sts, nil
	case "DaemonSet":
		ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		ds.Kind = "DaemonSet"
		ds.APIVersion = "apps/v1"
		return ds, nil
	case "Job":
		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		job.Kind = "Job"
		job.APIVersion = "batch/v1"
		return job, nil
	case "CronJob":
		cj, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		cj.Kind = "CronJob"
		cj.APIVersion = "batch/v1"
		return cj, nil
	case "Ingress":
		ing, err := client.NetworkingV1().Ingresses(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		ing.Kind = "Ingress"
		ing.APIVersion = "networking.k8s.io/v1"
		return ing, nil
	case "PersistentVolumeClaim":
		pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		pvc.Kind = "PersistentVolumeClaim"
		pvc.APIVersion = "v1"
		return pvc, nil
	case "PersistentVolume":
		pv, err := client.CoreV1().PersistentVolumes().Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		pv.Kind = "PersistentVolume"
		pv.APIVersion = "v1"
		return pv, nil
	case "ConfigMap":
		cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		cm.Kind = "ConfigMap"
		cm.APIVersion = "v1"
		return cm, nil
	case "Secret":
		sec, err := client.CoreV1().Secrets(namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		sec.Kind = "Secret"
		sec.APIVersion = "v1"
		return sec, nil
	case "Namespace":
		ns, err := client.CoreV1().Namespaces().Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		ns.Kind = "Namespace"
		ns.APIVersion = "v1"
		return ns, nil
	case "Node":
		node, err := client.CoreV1().Nodes().Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		node.Kind = "Node"
		node.APIVersion = "v1"
		return node, nil
	default:
		return nil, fmt.Errorf("不支持的资源种类: %q", kind)
	}
}

// defaultPodLogGetter 生产环境下由 ClientSet 获取 Pod 实时日志流的默认实现。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param opts 日志查询选项对象（避免 Data Clumps 散装传参）
// @return string 读取出的原始日志文本
// @return error 读取或流异常
func defaultPodLogGetter(ctx context.Context, client kubernetes.Interface, opts PodLogOptions) (string, error) {
	podLogOpts := &corev1.PodLogOptions{
		Container: opts.Container,
		TailLines: func() *int64 { t := int64(opts.Tail); return &t }(),
		Previous:  opts.Previous,
	}

	req := client.CoreV1().Pods(opts.Namespace).GetLogs(opts.Name, podLogOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("打开日志流失败: %w", err)
	}
	defer stream.Close()

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, stream); err != nil {
		return "", fmt.Errorf("读取日志流失败: %w", err)
	}

	return buf.String(), nil
}
