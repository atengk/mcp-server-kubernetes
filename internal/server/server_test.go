// Package server_test 基于内存管道与生命周期缝隙验证 MCP 服务端核心功能。
//
// @author Ateng
// @since 2026-10-06
package server_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/config"
	"github.com/atengk/mcp-server-kubernetes/internal/server"
	"github.com/atengk/mcp-server-kubernetes/internal/version"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestServer_Initialize_InMemoryIO 验证基于内存管道 (io.Pipe) 的 MCP 协议握手与元数据交换。
func TestServer_Initialize_InMemoryIO(t *testing.T) {
	cfg := &config.Config{
		Transport: config.TransportStdio,
		Port:      8080,
	}

	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("创建 Server 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 构建端到端双向内存流管道 (In-Memory Transport Seam)
	clientInReader, serverOutWriter := io.Pipe()
	serverInReader, clientOutWriter := io.Pipe()

	serverErrCh := make(chan error, 1)
	go func() {
		defer serverOutWriter.Close()
		serverErrCh <- srv.ServeStdio(ctx, serverInReader, serverOutWriter)
	}()

	// 2. 初始化 MCP 客户端并注入内存管道
	ioTransport := transport.NewIO(clientInReader, clientOutWriter, nil)
	mcpClient := client.NewClient(ioTransport)

	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("启动 MCP 客户端失败: %v", err)
	}
	defer func() {
		_ = mcpClient.Close()
		_ = clientOutWriter.Close()
	}()

	// 3. 执行 MCP Initialize 协议握手
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo: mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}

	initResult, err := mcpClient.Initialize(ctx, initReq)
	if err != nil {
		t.Fatalf("MCP 协议握手 Initialize 失败: %v", err)
	}

	// 4. 断言服务端元数据与协议支持
	if initResult.ServerInfo.Name != server.ServerName {
		t.Errorf("预期服务端名称为 %q, 实际得到 %q", server.ServerName, initResult.ServerInfo.Name)
	}
	if initResult.ServerInfo.Version != version.Version {
		t.Errorf("预期服务端版本为 %q, 实际得到 %q", version.Version, initResult.ServerInfo.Version)
	}

	// 5. 断言 Ping 连通性
	if err := mcpClient.Ping(ctx); err != nil {
		t.Errorf("MCP Ping 失败: %v", err)
	}

	// 6. 优雅取消上下文退出测试
	cancel()
	_ = clientOutWriter.Close()

	select {
	case err := <-serverErrCh:
		if err != nil && err != context.Canceled && err != io.EOF {
			t.Logf("服务端停机状态: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("服务端在上下文取消后未能在预期时间内退出")
	}
}

// TestServer_New_NilConfig 验证传入空配置时的防御拦截。
func TestServer_New_NilConfig(t *testing.T) {
	_, err := server.New(nil)
	if err == nil {
		t.Errorf("传入 nil 配置预期报错，但未报错")
	}
}

// TestServer_SSE_GracefulShutdown 验证 SSE 传输模式的启动与优雅停机能力。
func TestServer_SSE_GracefulShutdown(t *testing.T) {
	// 获取本地空闲可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("无法获取空闲端口: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cfg := &config.Config{
		Transport: config.TransportSSE,
		Port:      port,
	}

	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("创建 Server 失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serverErrCh := make(chan error, 1)

	go func() {
		serverErrCh <- srv.Serve(ctx)
	}()

	// 等待服务监听端口就绪
	time.Sleep(150 * time.Millisecond)

	// 触发优雅停机
	cancel()

	select {
	case err := <-serverErrCh:
		if err != nil && err != context.Canceled {
			t.Errorf("SSE 优雅停机发生非预期错误: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SSE 服务在上下文取消后未能按时停机")
	}
}
