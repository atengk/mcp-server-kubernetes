// Package tools 封装 Kubernetes 受控写操作变更、Dry-Run Diff 差异演练与安全门禁。
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
	"github.com/atengk/mcp-server-kubernetes/internal/safety"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// RegisterMutationTools 根据安全开关决定是否向 MCP Server 注册写变更工具集与预检 Diff 工具。
// 遵循 ADR 0003 与 ADR 0012 架构军规：未开启 allowWrite 时完全隐藏并拒绝注册写操作工具。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @param allowWrite 是否激活写权限门禁
// @return error 注册异常
func RegisterMutationTools(s *mcpserver.MCPServer, mgr *k8s.ClientManager, allowWrite bool) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	// 1. 始终注册只读预检差异对比工具 k8s_diff_resource
	s.AddTool(
		mcp.NewTool("k8s_diff_resource",
			mcp.WithDescription("变更预演对比工具：基于 Server-Side 逻辑比对拟提交配置与集群当前状态，输出标准 Unified Diff (只读无副作用)"),
			mcp.WithString("manifest", mcp.Required(), mcp.Description("拟提交应用的 YAML 资源清单配置")),
			mcp.WithString("namespace", mcp.Description("目标命名空间 (可选，默认优先使用 manifest 元数据)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeDiffResourceHandler(mgr),
	)

	// 2. 安全门禁拦截：未开启 allowWrite 时完全隐藏写工具
	if !allowWrite {
		return nil
	}

	// 3. 开启 allowWrite 时激活写操作工具集
	s.AddTool(
		mcp.NewTool("k8s_apply_resource",
			mcp.WithDescription("向 Kubernetes 集群应用或更新资源配置清单 (受 --allow-write 门禁保护)"),
			mcp.WithString("manifest", mcp.Required(), mcp.Description("待应用的 YAML 或 JSON 资源清单配置")),
			mcp.WithString("namespace", mcp.Description("目标命名空间 (可选)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
		),
		makeApplyResourceHandler(mgr),
	)

	s.AddTool(
		mcp.NewTool("k8s_scale_resource",
			mcp.WithDescription("水平伸缩工作负载副本数 (支持 Deployment, ReplicaSet, StatefulSet，受 --allow-write 门禁保护)"),
			mcp.WithString("kind", mcp.Required(), mcp.Description("工作负载类型 (如 Deployment, ReplicaSet, StatefulSet)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("工作负载名称")),
			mcp.WithNumber("replicas", mcp.Required(), mcp.Description("期望的目标副本数 (必须 >= 0)")),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("所属命名空间")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
		),
		makeScaleResourceHandler(mgr),
	)

	s.AddTool(
		mcp.NewTool("k8s_delete_resource",
			mcp.WithDescription("从 Kubernetes 集群安全删除指定资源 (受 --allow-write 门禁保护)"),
			mcp.WithString("kind", mcp.Required(), mcp.Description("资源种类 (如 Pod, Service, Deployment, ConfigMap 等)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("待删除的资源名称")),
			mcp.WithString("namespace", mcp.Description("命名空间 (集群作用域资源请留空)")),
			mcp.WithNumber("grace_period_seconds", mcp.Description("优雅终止宽限期秒数 (可选)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
		),
		makeDeleteResourceHandler(mgr),
	)

	return nil
}

// makeDiffResourceHandler 构建差异对比预检工具处理器。
func makeDiffResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		manifestStr, err := request.RequireString("manifest")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 manifest"), nil
		}

		u, err := parseManifestToUnstructured(manifestStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("解析 manifest 失败: %v", err)), nil
		}

		namespace := request.GetString("namespace", "")
		if namespace == "" {
			namespace = u.GetNamespace()
		}
		if namespace == "" {
			namespace = "default"
		}
		u.SetNamespace(namespace)

		name := u.GetName()
		if name == "" {
			return mcp.NewToolResultError("manifest 元数据缺少 metadata.name"), nil
		}

		contextName := request.GetString("context", "")
		dynClient, err := mgr.GetDynamicClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取 Dynamic 客户端失败: %v", err)), nil
		}

		gvr := inferResourceGVR(u.GroupVersionKind())
		currentYAML := ""

		// 检索现有集群资源状态
		existing, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil && existing != nil {
			prunedExisting, pErr := pruning.PruneUnstructured(existing)
			if pErr == nil {
				if yBytes, yErr := yaml.Marshal(prunedExisting.Object); yErr == nil {
					currentYAML = string(yBytes)
				}
			}
		}

		// 容错降级：若动态客户端未命中，尝试通过 Typed Client 获取现有资源
		if currentYAML == "" {
			if typedClient, cErr := mgr.GetClient(contextName); cErr == nil {
				if obj, qErr := querySingleResource(ctx, typedClient, u.GetKind(), namespace, name); qErr == nil && obj != nil {
					if pruned, pErr := pruning.Prune(obj); pErr == nil {
						if yBytes, yErr := yaml.Marshal(pruned); yErr == nil {
							currentYAML = string(yBytes)
						}
					}
				}
			}
		}

		// 执行 Server-Side Dry-Run 服务端校验与默认值演练 (ADR 0012)
		proposedObj := u
		if existing != nil {
			toUpdate := u.DeepCopy()
			toUpdate.SetResourceVersion(existing.GetResourceVersion())
			if dryRunResp, drErr := dynClient.Resource(gvr).Namespace(namespace).Update(ctx, toUpdate, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}); drErr == nil && dryRunResp != nil {
				proposedObj = dryRunResp
			}
		} else {
			if dryRunResp, drErr := dynClient.Resource(gvr).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}); drErr == nil && dryRunResp != nil {
				proposedObj = dryRunResp
			}
		}

		// 清洗拟提交或干跑生成的配置并序列化
		prunedProposed, err := pruning.PruneUnstructured(proposedObj)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗拟提交资源失败: %v", err)), nil
		}
		proposedYAMLBytes, err := yaml.Marshal(prunedProposed.Object)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化拟提交资源失败: %v", err)), nil
		}
		proposedYAML := string(proposedYAMLBytes)

		diff := safety.GenerateUnifiedDiff(currentYAML, proposedYAML, "current", "proposed")
		if diff == "" {
			return mcp.NewToolResultText("未检测到任何配置差异 (拟提交配置与集群当前状态完全一致)"), nil
		}

		return mcp.NewToolResultText(diff), nil
	}
}

// makeApplyResourceHandler 构建资源部署与更新工具处理器。
func makeApplyResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		manifestStr, err := request.RequireString("manifest")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 manifest"), nil
		}

		u, err := parseManifestToUnstructured(manifestStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("解析 manifest 失败: %v", err)), nil
		}

		namespace := request.GetString("namespace", "")
		if namespace == "" {
			namespace = u.GetNamespace()
		}
		if namespace == "" {
			namespace = "default"
		}
		u.SetNamespace(namespace)

		name := u.GetName()
		if name == "" {
			return mcp.NewToolResultError("manifest 缺少 metadata.name"), nil
		}

		contextName := request.GetString("context", "")
		dynClient, err := mgr.GetDynamicClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取 Dynamic 客户端失败: %v", err)), nil
		}

		gvr := inferResourceGVR(u.GroupVersionKind())

		var applied *unstructured.Unstructured
		existing, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			// 新建资源
			applied, err = dynClient.Resource(gvr).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("创建资源 %s/%s 失败: %v", gvr.Resource, name, err)), nil
			}
		} else if err != nil {
			// 若由于 fake 或特殊原因报错，尝试直接创建
			applied, err = dynClient.Resource(gvr).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("创建资源 %s/%s 失败: %v", gvr.Resource, name, err)), nil
			}
		} else {
			// 更新现有资源
			u.SetResourceVersion(existing.GetResourceVersion())
			applied, err = dynClient.Resource(gvr).Namespace(namespace).Update(ctx, u, metav1.UpdateOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("更新资源 %s/%s 失败: %v", gvr.Resource, name, err)), nil
			}
		}

		pruned, _ := pruning.PruneUnstructured(applied)
		jsonBytes, _ := json.Marshal(pruned.Object)

		return mcp.NewToolResultText(fmt.Sprintf("成功应用资源 %s/%s:\n%s", gvr.Resource, name, string(jsonBytes))), nil
	}
}

// makeScaleResourceHandler 构建工作负载副本伸缩工具处理器。
func makeScaleResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		kind, err := request.RequireString("kind")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 kind"), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 name"), nil
		}
		namespace, err := request.RequireString("namespace")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 namespace"), nil
		}
		replicasInt, err := request.RequireInt("replicas")
		if err != nil || replicasInt < 0 {
			return mcp.NewToolResultError("缺少有效的 replicas 副本数 (必须为非负整数)"), nil
		}

		replicas := int32(replicasInt)
		contextName := request.GetString("context", "")

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		switch strings.ToLower(kind) {
		case "deployment", "deployments", "deploy":
			deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("获取 Deployment 失败: %v", err)), nil
			}
			deploy.Spec.Replicas = &replicas
			_, err = client.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("更新 Deployment 副本数失败: %v", err)), nil
			}
		case "replicaset", "replicasets", "rs":
			rs, err := client.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("获取 ReplicaSet 失败: %v", err)), nil
			}
			rs.Spec.Replicas = &replicas
			_, err = client.AppsV1().ReplicaSets(namespace).Update(ctx, rs, metav1.UpdateOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("更新 ReplicaSet 副本数失败: %v", err)), nil
			}
		case "statefulset", "statefulsets", "sts":
			sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("获取 StatefulSet 失败: %v", err)), nil
			}
			sts.Spec.Replicas = &replicas
			_, err = client.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("更新 StatefulSet 副本数失败: %v", err)), nil
			}
		default:
			return mcp.NewToolResultError(fmt.Sprintf("不支持伸缩的工作负载种类: %s", kind)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("成功将 %s/%s (命名空间: %s) 副本数调整为 %d", kind, name, namespace, replicas)), nil
	}
}

// makeDeleteResourceHandler 构建资源删除工具处理器。
func makeDeleteResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		kind, err := request.RequireString("kind")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 kind"), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 name"), nil
		}

		namespace := request.GetString("namespace", "")
		contextName := request.GetString("context", "")

		var gracePeriod *int64
		if gp := request.GetInt("grace_period_seconds", -1); gp >= 0 {
			v := int64(gp)
			gracePeriod = &v
		}

		deleteOpts := metav1.DeleteOptions{
			GracePeriodSeconds: gracePeriod,
		}

		gvr := inferResourceGVR(schema.GroupVersionKind{Kind: kind})

		// 优先使用 Dynamic 客户端执行通用资源删除 (解耦特定类型硬编码)
		dynClient, dynErr := mgr.GetDynamicClient(contextName)
		if dynErr == nil {
			var delErr error
			if namespace != "" {
				delErr = dynClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, deleteOpts)
			} else {
				delErr = dynClient.Resource(gvr).Delete(ctx, name, deleteOpts)
			}
			if delErr == nil {
				return mcp.NewToolResultText(fmt.Sprintf("成功删除资源 %s/%s (命名空间: %s)", kind, name, namespace)), nil
			}
		}

		// 容错降级：尝试使用 Typed Client 核心接口删除
		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		var delErr error
		switch strings.ToLower(kind) {
		case "pod", "pods", "po":
			delErr = client.CoreV1().Pods(namespace).Delete(ctx, name, deleteOpts)
		case "deployment", "deployments", "deploy":
			delErr = client.AppsV1().Deployments(namespace).Delete(ctx, name, deleteOpts)
		case "replicaset", "replicasets", "rs":
			delErr = client.AppsV1().ReplicaSets(namespace).Delete(ctx, name, deleteOpts)
		case "service", "services", "svc":
			delErr = client.CoreV1().Services(namespace).Delete(ctx, name, deleteOpts)
		case "configmap", "configmaps", "cm":
			delErr = client.CoreV1().ConfigMaps(namespace).Delete(ctx, name, deleteOpts)
		case "secret", "secrets":
			delErr = client.CoreV1().Secrets(namespace).Delete(ctx, name, deleteOpts)
		case "namespace", "namespaces", "ns":
			delErr = client.CoreV1().Namespaces().Delete(ctx, name, deleteOpts)
		default:
			delErr = fmt.Errorf("未知的资源类型: %s", kind)
		}

		if delErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("删除资源 %s/%s 失败: %v", kind, name, delErr)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("成功删除资源 %s/%s (命名空间: %s)", kind, name, namespace)), nil
	}
}

// parseManifestToUnstructured 将 YAML 或 JSON 文本反序列化为 Unstructured 结构。
func parseManifestToUnstructured(manifest string) (*unstructured.Unstructured, error) {
	var obj map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &obj); err != nil {
		return nil, fmt.Errorf("反序列化 YAML 配置失败: %w", err)
	}
	if obj == nil {
		return nil, errors.New("配置内容为空")
	}
	return &unstructured.Unstructured{Object: obj}, nil
}

// irregularPlurals 记录 Kubernetes 核心与常用资源的不规则复数映射。
var irregularPlurals = map[string]string{
	"ingress":                  "ingresses",
	"networkpolicy":            "networkpolicies",
	"endpoints":                "endpoints",
	"customresourcedefinition": "customresourcedefinitions",
	"priorityclass":            "priorityclasses",
	"storageclass":             "storageclasses",
	"horizontalpodautoscaler":  "horizontalpodautoscalers",
}

// inferResourceGVR 根据 GVK 推导对应的 GroupVersionResource。
func inferResourceGVR(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	group := gvk.Group
	version := gvk.Version
	if version == "" {
		version = "v1"
	}

	lowerKind := strings.ToLower(gvk.Kind)
	if plural, ok := irregularPlurals[lowerKind]; ok {
		return schema.GroupVersionResource{Group: group, Version: version, Resource: plural}
	}

	// 通用英文复数推导
	res := lowerKind
	if strings.HasSuffix(res, "s") || strings.HasSuffix(res, "x") || strings.HasSuffix(res, "z") || strings.HasSuffix(res, "ch") || strings.HasSuffix(res, "sh") {
		res += "es"
	} else if strings.HasSuffix(res, "y") && len(res) > 1 && !isVowel(res[len(res)-2]) {
		res = res[:len(res)-1] + "ies"
	} else {
		res += "s"
	}

	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: res,
	}
}

// isVowel 判断字符是否为元音字母。
func isVowel(b byte) bool {
	return b == 'a' || b == 'e' || b == 'i' || b == 'o' || b == 'u'
}
