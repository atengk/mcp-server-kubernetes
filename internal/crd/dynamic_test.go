// Package crd_test 针对动态 CRD 反射工具执行单元测试与协议交互验证。
//
// @author Ateng
// @since 2026-10-06
package crd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/crd"
	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
)

func newSampleCertificate(namespace, name string, labels map[string]string) *unstructured.Unstructured {
	labelsMap := make(map[string]any, len(labels))
	for k, v := range labels {
		labelsMap[k] = v
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"labels":    labelsMap,
				"managedFields": []any{
					map[string]any{
						"manager":   "cert-manager",
						"operation": "Update",
					},
				},
				"annotations": map[string]any{
					"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"cert-manager.io/v1\"}",
				},
			},
			"spec": map[string]any{
				"dnsNames":   []any{"example.com", "api.example.com"},
				"secretName": "example-tls",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Ready",
						"status": "True",
						"reason": "CertIssued",
					},
				},
			},
		},
	}
	return obj
}

func setupTestCRDClient(t *testing.T, objects ...runtime.Object) (*client.Client, func()) {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		schema.GroupVersionResource{
			Group:    "cert-manager.io",
			Version:  "v1",
			Resource: "certificates",
		}: "CertificateList",
	}
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)

	mgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		}),
		k8s.WithDynamicClientFactory(func(rc *rest.Config) (dynamic.Interface, error) {
			return fakeDynamic, nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化 ClientManager 失败: %v", err)
	}

	server := mcpserver.NewMCPServer("test-server", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)
	if err := crd.RegisterCRDTools(server, mgr); err != nil {
		t.Fatalf("注册 CRD 工具失败: %v", err)
	}

	mcpClient, err := client.NewInProcessClient(server)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("启动 MCP 客户端失败: %v", err)
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("初始化握手失败: %v", err)
	}

	cleanup := func() {
		_ = mcpClient.Close()
	}

	return mcpClient, cleanup
}

func TestCRD_GetCustomResource(t *testing.T) {
	cert := newSampleCertificate("default", "my-cert", map[string]string{"app": "gateway"})
	mcpClient, cleanup := setupTestCRDClient(t, cert)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 成功获取并验证清洗与脱敏
	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_custom_resource",
			Arguments: map[string]any{
				"group":     "cert-manager.io",
				"version":   "v1",
				"resource":  "certificates",
				"namespace": "default",
				"name":      "my-cert",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_get_custom_resource 报错: %v", err)
	}
	if res.IsError {
		t.Fatalf("预期成功但返回错误: %v", res.Content)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "managedFields") {
		t.Errorf("Smart Pruning 未剔除 managedFields")
	}
	if strings.Contains(text, "last-applied-configuration") {
		t.Errorf("Smart Pruning 未剔除 last-applied-configuration")
	}
	if !strings.Contains(text, "my-cert") || !strings.Contains(text, "example-tls") {
		t.Errorf("响应缺少核心 CRD 属性: %s", text)
	}

	// 2. 查询不存在的资源
	resNotFound, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_custom_resource",
			Arguments: map[string]any{
				"group":     "cert-manager.io",
				"version":   "v1",
				"resource":  "certificates",
				"namespace": "default",
				"name":      "non-existent-cert",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用报错: %v", err)
	}
	if !resNotFound.IsError {
		t.Errorf("预期返回错误，但返回成功")
	}

	// 3. 缺少必填参数
	resMissing, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_custom_resource",
			Arguments: map[string]any{
				"version":  "v1",
				"resource": "certificates",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用报错: %v", err)
	}
	if !resMissing.IsError {
		t.Errorf("预期缺少参数返回错误，但返回成功")
	}
}

func TestCRD_ListCustomResources(t *testing.T) {
	var objects []runtime.Object
	for i := 1; i <= 105; i++ {
		cert := newSampleCertificate("prod", fmt.Sprintf("cert-%03d", i), map[string]string{
			"env": "production",
		})
		objects = append(objects, cert)
	}
	// 再加一个非 prod 标签的
	otherCert := newSampleCertificate("prod", "other-cert", map[string]string{
		"env": "staging",
	})
	objects = append(objects, otherCert)

	mcpClient, cleanup := setupTestCRDClient(t, objects...)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 带 labelSelector 过滤与截断验证
	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_list_custom_resources",
			Arguments: map[string]any{
				"group":         "cert-manager.io",
				"version":       "v1",
				"resource":      "certificates",
				"namespace":     "prod",
				"labelSelector": "env=production",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_list_custom_resources 报错: %v", err)
	}
	if res.IsError {
		t.Fatalf("预期成功但返回错误: %v", res.Content)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "[OUTPUT TRUNCATED") {
		t.Errorf("超过 100 项时预期包含截断警示，实际未发现: %s", text)
	}
	if strings.Contains(text, "other-cert") {
		t.Errorf("labelSelector 未能成功过滤掉 staging 资源")
	}

	// 2. 空列表返回安全契约（无匹配时返回空切片而非 nil）
	resEmpty, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_list_custom_resources",
			Arguments: map[string]any{
				"group":     "cert-manager.io",
				"version":   "v1",
				"resource":  "certificates",
				"namespace": "empty-ns",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用空列表报错: %v", err)
	}
	if resEmpty.IsError {
		t.Fatalf("查询空列表不应报错: %v", resEmpty.Content)
	}

	textEmpty := resEmpty.Content[0].(mcp.TextContent).Text
	var emptyMap map[string]any
	if err := json.Unmarshal([]byte(textEmpty), &emptyMap); err != nil {
		t.Fatalf("解析空列表 JSON 失败: %v", err)
	}
	items, ok := emptyMap["items"].([]any)
	if !ok || items == nil {
		t.Errorf("空查询预期 items 为 [] 且非 nil，实际为: %v", emptyMap["items"])
	}
	if len(items) != 0 {
		t.Errorf("预期 items 长度为 0，实际为 %d", len(items))
	}
}
