// Package prompts_test 针对预置排障与运维 Prompts 模板开展单元测试与协议验证。
//
// @author Ateng
// @since 2026-10-06
package prompts_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/prompts"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func setupTestPromptsClient(t *testing.T) (*client.Client, func()) {
	server := mcpserver.NewMCPServer("test-server", "1.0.0",
		mcpserver.WithPromptCapabilities(true),
	)
	if err := prompts.RegisterPrompts(server); err != nil {
		t.Fatalf("注册 Prompts 失败: %v", err)
	}

	mcpClient, err := client.NewInProcessClient(server)
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

func TestPrompts_ListPrompts(t *testing.T) {
	mcpClient, cleanup := setupTestPromptsClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listRes, err := mcpClient.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		t.Fatalf("列出 Prompts 失败: %v", err)
	}

	expectedPrompts := map[string]bool{
		"diagnose-pod-failure":    false,
		"cluster-health-check":    false,
		"workload-security-audit": false,
	}

	for _, p := range listRes.Prompts {
		if _, ok := expectedPrompts[p.Name]; ok {
			expectedPrompts[p.Name] = true
		}
	}

	for name, found := range expectedPrompts {
		if !found {
			t.Errorf("未在 Prompt 列表中发现预置模板: %s", name)
		}
	}
}

func TestPrompts_DiagnosePodFailure(t *testing.T) {
	mcpClient, cleanup := setupTestPromptsClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 正常入参渲染
	res, err := mcpClient.GetPrompt(ctx, mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "diagnose-pod-failure",
			Arguments: map[string]string{
				"namespace": "production",
				"pod_name":  "api-gateway-7f9b8c6",
			},
		},
	})
	if err != nil {
		t.Fatalf("获取 diagnose-pod-failure 报错: %v", err)
	}
	if len(res.Messages) == 0 {
		t.Fatalf("预期返回至少一条指导 Message")
	}

	textContent, ok := res.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("预期 Content 为 TextContent 类型")
	}
	text := textContent.Text

	if !strings.Contains(text, "api-gateway-7f9b8c6") || !strings.Contains(text, "production") {
		t.Errorf("生成的提示词中缺少目标 Pod 或 Namespace: %s", text)
	}
	if !strings.Contains(text, "k8s_describe_resource") || !strings.Contains(text, "k8s_get_pod_logs") {
		t.Errorf("提示词应包含推荐的排障诊断工具指导")
	}

	// 2. 缺少必填参数
	_, errMissing := mcpClient.GetPrompt(ctx, mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "diagnose-pod-failure",
			Arguments: map[string]string{
				"namespace": "production",
			},
		},
	})
	if errMissing == nil {
		t.Errorf("缺少 pod_name 预期返回错误")
	}
}

func TestPrompts_ClusterHealthCheck(t *testing.T) {
	mcpClient, cleanup := setupTestPromptsClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.GetPrompt(ctx, mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "cluster-health-check",
		},
	})
	if err != nil {
		t.Fatalf("获取 cluster-health-check 报错: %v", err)
	}
	if len(res.Messages) == 0 {
		t.Fatalf("预期返回至少一条指导 Message")
	}

	textContent, ok := res.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("预期 Content 为 TextContent 类型")
	}
	text := textContent.Text

	if !strings.Contains(text, "Node") || !strings.Contains(text, "Pending") || !strings.Contains(text, "Warning") {
		t.Errorf("巡检提示词未包含节点、异常 Pod 或告警事件检查指引: %s", text)
	}
}

func TestPrompts_WorkloadSecurityAudit(t *testing.T) {
	mcpClient, cleanup := setupTestPromptsClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.GetPrompt(ctx, mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "workload-security-audit",
			Arguments: map[string]string{
				"namespace": "payment",
			},
		},
	})
	if err != nil {
		t.Fatalf("获取 workload-security-audit 报错: %v", err)
	}
	if len(res.Messages) == 0 {
		t.Fatalf("预期返回至少一条指导 Message")
	}

	textContent, ok := res.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("预期 Content 为 TextContent 类型")
	}
	text := textContent.Text

	if !strings.Contains(text, "payment") {
		t.Errorf("审计提示词缺少指定的目标命名空间: %s", text)
	}
	if !strings.Contains(text, "privileged") || !strings.Contains(text, "runAsNonRoot") {
		t.Errorf("审计提示词缺少特权或运行身份合规检查指引: %s", text)
	}
}
