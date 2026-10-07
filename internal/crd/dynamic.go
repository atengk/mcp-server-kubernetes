// Package crd 封装面向 Kubernetes 自定义资源 (CRD) 的通用动态反射发现与查询工具。
//
// @author Ateng
// @since 2026-10-06
package crd

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	// defaultListLimit 默认查询列表返回条目上限
	defaultListLimit = 100

	// maxListLimit 列表返回硬上限（防爆保护）
	maxListLimit = 100
)

// RegisterCRDTools 向 MCP Server 注册通用自定义资源 (CRD) 动态查询工具。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @return error 注册异常
func RegisterCRDTools(s *mcpserver.MCPServer, mgr *k8s.ClientManager) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	// 1. 注册 k8s_get_custom_resource
	s.AddTool(
		mcp.NewTool("k8s_get_custom_resource",
			mcp.WithDescription("基于 Dynamic Client 查询指定自定义资源 (CRD) 实例详情，经 Smart Pruning 清洗降噪与 Secret 脱敏后返回"),
			mcp.WithString("group", mcp.Required(), mcp.Description("API Group (如 cert-manager.io、istio.io，核心组传空字符串)")),
			mcp.WithString("version", mcp.Required(), mcp.Description("API Version (如 v1、v1beta1)")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("资源复数名称 (如 certificates、virtualservices)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("CRD 资源名称")),
			mcp.WithString("namespace", mcp.Description("命名空间 (集群作用域资源请留空)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeGetCustomResourceHandler(mgr),
	)

	// 2. 注册 k8s_list_custom_resources
	s.AddTool(
		mcp.NewTool("k8s_list_custom_resources",
			mcp.WithDescription("基于 Dynamic Client 批量列出指定自定义资源 (CRD) 实例，支持标签过滤与 Token Guard 截断守卫"),
			mcp.WithString("group", mcp.Required(), mcp.Description("API Group (如 cert-manager.io、monitoring.coreos.com)")),
			mcp.WithString("version", mcp.Required(), mcp.Description("API Version (如 v1、v1alpha1)")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("资源复数名称 (如 certificates、prometheuses)")),
			mcp.WithString("namespace", mcp.Description("命名空间 (未指定则列出所有命名空间或集群范围资源)")),
			mcp.WithString("labelSelector", mcp.Description("标签选择器 (如 app=frontend,env=prod)")),
			mcp.WithNumber("limit", mcp.Description("返回资源数量上限 (默认 100，硬上限 100)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeListCustomResourcesHandler(mgr),
	)

	return nil
}

// makeGetCustomResourceHandler 构建获取单个 CRD 实例的工具处理器。
//
// @param mgr Kubernetes 客户端管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调
func makeGetCustomResourceHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gvr, err := extractGVR(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 name"), nil
		}
		name = strings.TrimSpace(name)

		namespace := strings.TrimSpace(request.GetString("namespace", ""))
		contextName := strings.TrimSpace(request.GetString("context", ""))

		dynClient, err := mgr.GetDynamicClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取 Dynamic 客户端失败: %v", err)), nil
		}

		obj, err := getCRDInstance(ctx, dynClient, gvr, namespace, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("查询自定义资源 %s/%s 失败: %v", gvr.Resource, name, err)), nil
		}

		// 接入 Smart Pruning 管道降噪与脱敏
		pruned, err := pruning.PruneUnstructured(obj)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗自定义资源失败: %v", err)), nil
		}

		data, err := json.Marshal(pruned.Object)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化自定义资源失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

// makeListCustomResourcesHandler 构建批量列出 CRD 实例的工具处理器。
//
// @param mgr Kubernetes 客户端管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调
func makeListCustomResourcesHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gvr, err := extractGVR(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		namespace := strings.TrimSpace(request.GetString("namespace", ""))
		labelSelector := strings.TrimSpace(request.GetString("labelSelector", ""))
		contextName := strings.TrimSpace(request.GetString("context", ""))

		limit := request.GetInt("limit", defaultListLimit)
		if limit <= 0 || limit > maxListLimit {
			limit = maxListLimit
		}

		dynClient, err := mgr.GetDynamicClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取 Dynamic 客户端失败: %v", err)), nil
		}

		list, err := listCRDInstances(ctx, dynClient, gvr, namespace, labelSelector)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("列出自定义资源 %s 失败: %v", gvr.Resource, err)), nil
		}

		var rawItems []unstructured.Unstructured
		if list != nil && list.Items != nil {
			rawItems = list.Items
		} else {
			rawItems = make([]unstructured.Unstructured, 0)
		}

		// Token Guard 守卫截断
		truncatedItems, isTruncated := safety.TruncateItems(rawItems, limit)
		var warningMsg string
		if isTruncated {
			warningMsg = safety.FormatTruncationWarning(fmt.Sprintf("资源总数 %d 超过单次上限 %d，已截断", len(rawItems), limit))
		}

		// Smart Pruning 批量清洗
		prunedItems := make([]map[string]any, 0, len(truncatedItems))
		for i := range truncatedItems {
			pruned, pErr := pruning.PruneUnstructured(&truncatedItems[i])
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("清洗资源条目失败: %v", pErr)), nil
			}
			prunedItems = append(prunedItems, pruned.Object)
		}

		resultMap := map[string]any{
			"apiVersion": gvr.GroupVersion().String(),
			"kind":       resolveListKind(list, gvr.Resource),
			"items":      prunedItems,
		}
		if isTruncated {
			resultMap["_warning"] = warningMsg
		}

		jsonData, err := json.Marshal(resultMap)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化资源列表失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(jsonData)), nil
	}
}

// extractGVR 从 MCP 请求中提取并校验通用的 GroupVersionResource 核心三元组。
func extractGVR(request mcp.CallToolRequest) (schema.GroupVersionResource, error) {
	group, err := request.RequireString("group")
	if err != nil {
		return schema.GroupVersionResource{}, errors.New("缺少必填参数 group")
	}
	version, err := request.RequireString("version")
	if err != nil {
		return schema.GroupVersionResource{}, errors.New("缺少必填参数 version")
	}
	resource, err := request.RequireString("resource")
	if err != nil {
		return schema.GroupVersionResource{}, errors.New("缺少必填参数 resource")
	}

	group = strings.TrimSpace(group)
	version = strings.TrimSpace(version)
	resource = strings.TrimSpace(resource)

	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}

// resolveListKind 推导并返回资源列表对应的准确 Kind 名称。
func resolveListKind(list *unstructured.UnstructuredList, resourceName string) string {
	if list != nil && list.GetKind() != "" {
		return list.GetKind()
	}
	if len(resourceName) == 0 {
		return "CustomResourceList"
	}
	return strings.ToUpper(resourceName[:1]) + resourceName[1:] + "List"
}

// getCRDInstance 获取单个 CRD 资源实例。
func getCRDInstance(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace != "" {
		return client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	return client.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
}

// listCRDInstances 批量列出 CRD 资源实例。
func listCRDInstances(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, namespace, labelSelector string) (*unstructured.UnstructuredList, error) {
	listOpts := metav1.ListOptions{
		LabelSelector: labelSelector,
	}
	if namespace != "" {
		return client.Resource(gvr).Namespace(namespace).List(ctx, listOpts)
	}
	return client.Resource(gvr).List(ctx, listOpts)
}
