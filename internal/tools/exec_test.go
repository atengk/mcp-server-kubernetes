// Package tools_test 针对容器探针 Exec 命令执行与安全门禁开展单元测试。
//
// @author Ateng
// @since 2026-10-06
package tools_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/tools"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func setupTestExecServer(t *testing.T, allowExec bool, runner tools.ExecRunnerFunc) (*client.Client, func()) {
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
		t.Fatalf("初始化 ClientManager 失败: %v", err)
	}

	server := mcpserver.NewMCPServer("test-server", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	var opts []tools.ExecOption
	if runner != nil {
		opts = append(opts, tools.WithExecRunner(runner))
	}

	if err := tools.RegisterExecTools(server, mgr, allowExec, opts...); err != nil {
		t.Fatalf("注册 Exec 工具失败: %v", err)
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
		t.Fatalf("握手失败: %v", err)
	}

	cleanup := func() {
		_ = mcpClient.Close()
	}

	return mcpClient, cleanup
}

func TestExec_DisabledByDefaultForSafety(t *testing.T) {
	// 默认未开启 --allow-exec，工具应完全被隐藏
	mcpClient, cleanup := setupTestExecServer(t, false, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listRes, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("列出工具失败: %v", err)
	}

	for _, tool := range listRes.Tools {
		if tool.Name == "k8s_exec_command" {
			t.Errorf("默认只读安全模式下，k8s_exec_command 工具应被完全隐藏，但实际暴露了")
		}
	}

	// 主动调用未暴露工具应被协议层拦截
	callRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_exec_command",
			Arguments: map[string]any{
				"pod_name":  "test-pod",
				"namespace": "default",
				"command":   []string{"ls"},
			},
		},
	})
	if err == nil && (callRes == nil || !callRes.IsError) {
		t.Errorf("未授权调用应返回错误或被拦截")
	}
}

func TestExec_Enabled_NormalExecution(t *testing.T) {
	runner := func(ctx context.Context, client kubernetes.Interface, rc *rest.Config, opts tools.ExecOptions) (string, string, int, error) {
		if opts.PodName != "my-pod" || opts.Namespace != "default" {
			return "", "not found", 1, errors.New("pod not found")
		}
		return "hello world from container\n", "", 0, nil
	}

	mcpClient, cleanup := setupTestExecServer(t, true, runner)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_exec_command",
			Arguments: map[string]any{
				"pod_name":  "my-pod",
				"namespace": "default",
				"command":   []any{"echo", "hello"},
			},
		},
	})
	if err != nil {
		t.Fatalf("调用报错: %v", err)
	}
	if res.IsError {
		t.Fatalf("预期成功但返回错误: %v", res.Content)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "hello world from container") {
		t.Errorf("未能正确获取容器输出: %s", text)
	}
}

func TestExec_BufferTruncationAt64KB(t *testing.T) {
	runner := func(ctx context.Context, client kubernetes.Interface, rc *rest.Config, opts tools.ExecOptions) (string, string, int, error) {
		// 生成 100KB 大输出
		largeOutput := strings.Repeat("a", 100*1024)
		return largeOutput, "", 0, nil
	}

	mcpClient, cleanup := setupTestExecServer(t, true, runner)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_exec_command",
			Arguments: map[string]any{
				"pod_name":  "large-pod",
				"namespace": "default",
				"command":   []any{"cat", "/large/file"},
			},
		},
	})
	if err != nil {
		t.Fatalf("调用报错: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "[OUTPUT TRUNCATED: 超过 64KB 缓冲区上限]") {
		t.Errorf("预期输出包含 64KB 截断警告，实际为: %s", text)
	}
	// 验证实际返回体积不超过 65KB (截断后 64KB 加上提示文本)
	if len(text) > 66*1024 {
		t.Errorf("返回内容长度 %d 超过了截断限制", len(text))
	}
}

func TestExec_TimeoutFuseProtection(t *testing.T) {
	runner := func(ctx context.Context, client kubernetes.Interface, rc *rest.Config, opts tools.ExecOptions) (string, string, int, error) {
		select {
		case <-ctx.Done():
			return "", "", 0, ctx.Err()
		case <-time.After(20 * time.Second):
			return "done", "", 0, nil
		}
	}

	mcpClient, cleanup := setupTestExecServer(t, true, runner)
	defer cleanup()

	// 传入已超时的 Context 验证熔断
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_exec_command",
			Arguments: map[string]any{
				"pod_name":  "timeout-pod",
				"namespace": "default",
				"command":   []any{"sleep", "30"},
			},
		},
	})
	if err != nil {
		// 客户端层超时亦属预期
		return
	}
	if !res.IsError {
		t.Errorf("超时任务预期返回错误")
	}
}
