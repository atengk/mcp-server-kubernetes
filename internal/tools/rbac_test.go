// Package tools_test 针对 Kubernetes RBAC 鉴权自检探针 (k8s_auth_can_i) 开展单元与集成测试。
//
// @author Ateng
// @since 2026-10-06
package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/tools"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

// setupRBACTestServer 初始化 RBAC 测试客户端
func setupRBACTestServer(t *testing.T, fakeClient kubernetes.Interface) (*client.Client, func()) {
	k8sMgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return &rest.Config{Host: "https://k8s.local"}, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return fakeClient, nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化 ClientManager 失败: %v", err)
	}

	mcpSrv := mcpserver.NewMCPServer("test-rbac-server", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	if err := tools.RegisterRBACTool(mcpSrv, k8sMgr); err != nil {
		t.Fatalf("注册 RBAC 工具失败: %v", err)
	}

	mcpClient, err := client.NewInProcessClient(mcpSrv)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo:      mcp.Implementation{Name: "rbac-test-client", Version: "1.0.0"},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("握手失败: %v", err)
	}

	return mcpClient, func() {
		_ = mcpClient.Close()
	}
}

func TestTools_AuthCanI_SelfSubjectAccessReview_Allowed(t *testing.T) {
	// 构造真实 Role 与 RoleBinding 规则对象
	podReaderRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-reader", Namespace: "default"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"pods"}},
		},
	}
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "read-pods", Namespace: "default"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "current-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "pod-reader"},
	}

	fakeClient := fake.NewSimpleClientset(podReaderRole, roleBinding)
	// 配置 SelfSubjectAccessReviews 反映为允许
	fakeClient.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		sar := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
		sar.Status.Allowed = true
		sar.Status.Reason = "RBAC: allowed by pod-reader role"
		return true, sar, nil
	})

	mcpClient, cleanup := setupRBACTestServer(t, fakeClient)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_auth_can_i",
			Arguments: map[string]any{
				"verb":      "get",
				"resource":  "pods",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_auth_can_i 失败: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	var check tools.CanIResult
	if err := json.Unmarshal([]byte(text), &check); err != nil {
		t.Fatalf("反序列化 CanI 响应失败: %v (原文: %s)", err, text)
	}

	if !check.Allowed {
		t.Errorf("预期鉴权结果为 true，实际为 false: %+v", check)
	}
	if check.Verb != "get" || check.Resource != "pods" || check.Namespace != "default" {
		t.Errorf("鉴权属性回显异常: %+v", check)
	}
}

func TestTools_AuthCanI_SelfSubjectAccessReview_Denied(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	// 配置 SelfSubjectAccessReviews 反映为拒绝
	fakeClient.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		sar := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
		sar.Status.Allowed = false
		sar.Status.Denied = true
		sar.Status.Reason = "RBAC: User cannot delete namespaces"
		return true, sar, nil
	})

	mcpClient, cleanup := setupRBACTestServer(t, fakeClient)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_auth_can_i",
			Arguments: map[string]any{
				"verb":     "delete",
				"resource": "namespaces",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_auth_can_i 失败: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	var check tools.CanIResult
	if err := json.Unmarshal([]byte(text), &check); err != nil {
		t.Fatalf("反序列化 CanI 响应失败: %v (原文: %s)", err, text)
	}

	if check.Allowed {
		t.Errorf("预期鉴权结果为 false，实际为 true: %+v", check)
	}
	if check.Reason != "RBAC: User cannot delete namespaces" {
		t.Errorf("拒绝理由未正确透传: %+v", check)
	}
}

func TestTools_AuthCanI_SubjectAccessReview_SpecificUser(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	// 配置 SubjectAccessReviews 反映
	fakeClient.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
		if sar.Spec.User == "developer-bob" {
			sar.Status.Allowed = true
		} else {
			sar.Status.Allowed = false
		}
		return true, sar, nil
	})

	mcpClient, cleanup := setupRBACTestServer(t, fakeClient)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 针对 developer-bob 检查
	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_auth_can_i",
			Arguments: map[string]any{
				"verb":      "create",
				"resource":  "deployments",
				"namespace": "production",
				"user":      "developer-bob",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_auth_can_i 失败: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	var check tools.CanIResult
	if err := json.Unmarshal([]byte(text), &check); err != nil {
		t.Fatalf("反序列化 CanI 响应失败: %v", err)
	}

	if !check.Allowed {
		t.Errorf("预期 developer-bob 具备创建权限: %+v", check)
	}
	if check.User != "developer-bob" {
		t.Errorf("User 字段未正确透传: %+v", check)
	}
}
