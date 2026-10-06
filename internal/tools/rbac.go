// Package tools 提供 Kubernetes RBAC 鉴权自检探针与权限边界评估工具。
//
// @author Ateng
// @since 2026-10-06
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/pruning"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CanIResult 强类型封装 Kubernetes RBAC 鉴权评估结果契约。
type CanIResult struct {
	// Allowed 标记目标操作是否被当前或指定权限策略显式允许
	Allowed bool `json:"allowed"`

	// Denied 标记目标操作是否被显式拒绝
	Denied bool `json:"denied,omitempty"`

	// Reason 授权机制回传的允许或拒绝理由说明
	Reason string `json:"reason,omitempty"`

	// EvaluationError 鉴权评估过程中可能产生的异常信息
	EvaluationError string `json:"evaluationError,omitempty"`

	// Verb 评估的操作动词（如 get, list, create, delete）
	Verb string `json:"verb"`

	// Resource 评估的 Kubernetes 资源种类（如 pods, deployments）
	Resource string `json:"resource"`

	// Namespace 评估作用的命名空间范围（空表示集群全局或所有命名空间）
	Namespace string `json:"namespace,omitempty"`

	// Subresource 评估的特定子资源（如 log, status, portforward）
	Subresource string `json:"subresource,omitempty"`

	// Group 目标资源所属的 API Group（如 apps, batch）
	Group string `json:"group,omitempty"`

	// User 被评估的目标用户名（为空表示自检当前集群凭据）
	User string `json:"user,omitempty"`
}

// RegisterRBACTool 向 MCP Server 注册 RBAC 权限自检探针工具 (k8s_auth_can_i)。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @return error 注册异常
func RegisterRBACTool(s *mcpserver.MCPServer, mgr *k8s.ClientManager) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	s.AddTool(
		mcp.NewTool("k8s_auth_can_i",
			mcp.WithDescription("基于 Kubernetes AccessReview API 评估当前凭据或指定用户在特定命名空间下的权限边界 (等效于 kubectl auth can-i)"),
			mcp.WithString("verb", mcp.Required(), mcp.Description("操作动词 (如 get, list, create, delete, update, patch, watch)")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("资源类型 (如 pods, services, deployments, secrets, namespaces)")),
			mcp.WithString("namespace", mcp.Description("目标命名空间 (未指定则评估集群级别权限)")),
			mcp.WithString("subresource", mcp.Description("子资源名称 (可选，如 log, status, exec)")),
			mcp.WithString("group", mcp.Description("API Group (可选，如 apps, batch, rbac.authorization.k8s.io)")),
			mcp.WithString("user", mcp.Description("指定被评估的目标用户名 (可选，未指定则评估当前连接凭据)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		makeAuthCanIHandler(mgr),
	)

	return nil
}

// makeAuthCanIHandler 构建执行 RBAC 鉴权自检的 MCP 处理器。
//
// @param mgr Kubernetes 客户端连接池管理器
// @return mcpserver.ToolHandlerFunc 工具执行回调函数
func makeAuthCanIHandler(mgr *k8s.ClientManager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		verb, err := request.RequireString("verb")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 verb"), nil
		}
		resource, err := request.RequireString("resource")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 resource"), nil
		}

		namespace := request.GetString("namespace", "")
		subresource := request.GetString("subresource", "")
		group := request.GetString("group", "")
		user := request.GetString("user", "")
		contextName := request.GetString("context", "")

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		checkResult, err := evaluateAccessReview(ctx, client, accessReviewOptions{
			Verb:        verb,
			Resource:    resource,
			Namespace:   namespace,
			Subresource: subresource,
			Group:       group,
			User:        user,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("权限自检执行失败: %v", err)), nil
		}

		// 接入 Smart Pruning 清洗降噪
		pruned, err := pruning.Prune(checkResult)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("清洗鉴权结果失败: %v", err)), nil
		}

		jsonBytes, err := json.Marshal(pruned)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("序列化鉴权结果失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}

// accessReviewOptions 封装权限评估所需的详细上下文选项。
type accessReviewOptions struct {
	// Verb 操作动词
	Verb string
	// Resource 目标资源种类
	Resource string
	// Namespace 作用域命名空间
	Namespace string
	// Subresource 子资源名称
	Subresource string
	// Group 资源所属 API 组
	Group string
	// User 被评估的目标用户名
	User string
}

// evaluateAccessReview 调用 Kubernetes AccessReview API 评估权限。
//
// @param ctx 请求上下文
// @param client Kubernetes 集群客户端接口
// @param opts 鉴权参数选项
// @return *CanIResult 评估结果结构
// @return error API 调用错误
func evaluateAccessReview(ctx context.Context, client kubernetes.Interface, opts accessReviewOptions) (*CanIResult, error) {
	result := &CanIResult{
		Verb:        opts.Verb,
		Resource:    opts.Resource,
		Namespace:   opts.Namespace,
		Subresource: opts.Subresource,
		Group:       opts.Group,
		User:        opts.User,
	}

	attrs := &authorizationv1.ResourceAttributes{
		Namespace:   opts.Namespace,
		Verb:        opts.Verb,
		Group:       opts.Group,
		Resource:    opts.Resource,
		Subresource: opts.Subresource,
	}

	// 1. 若未指定具体用户，基于当前客户端凭据发起 SelfSubjectAccessReview
	if opts.User == "" {
		selfSAR := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: attrs,
			},
		}
		resp, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, selfSAR, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("创建 SelfSubjectAccessReview 失败: %w", err)
		}
		populateResultFromStatus(result, resp.Status)
		return result, nil
	}

	// 2. 若显式指定目标用户，基于 SubjectAccessReview 模拟评估目标用户的权限边界
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:               opts.User,
			ResourceAttributes: attrs,
		},
	}
	resp, err := client.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("创建 SubjectAccessReview 失败: %w", err)
	}
	populateResultFromStatus(result, resp.Status)
	return result, nil
}

// populateResultFromStatus 填充 AccessReview 返回的权限状态信息。
func populateResultFromStatus(result *CanIResult, status authorizationv1.SubjectAccessReviewStatus) {
	result.Allowed = status.Allowed
	result.Denied = status.Denied
	result.Reason = status.Reason
	result.EvaluationError = status.EvaluationError
}
