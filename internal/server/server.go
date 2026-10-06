// Package server 封装 MCP 服务的生命周期管理、协议通道适配与优雅停机机制。
//
// @author Ateng
// @since 2026-10-06
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/config"
	"github.com/atengk/mcp-server-kubernetes/internal/version"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	// ServerName 声明当前 MCP 服务对外暴露的统一名称标识（遵循领域术语）
	ServerName = "mcp-server-kubernetes"

	// defaultShutdownTimeout 优雅停机的最长等待超时时间（消除魔法值）
	defaultShutdownTimeout = 5 * time.Second
)

// Server 聚合 MCP 核心实例与底层传输适配器。
type Server struct {
	cfg         *config.Config
	mcpServer   *mcpserver.MCPServer
	stdioServer *mcpserver.StdioServer
	sseServer   *mcpserver.SSEServer
}

// New 创建并初始化 MCP 服务实例。
//
// @param cfg 运行配置对象
// @return *Server 初始化完成的服务实例
// @return error 配置无效或初始化失败时返回错误
func New(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("配置对象不可为空")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}

	baseServer := mcpserver.NewMCPServer(
		ServerName,
		version.Version,
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithPromptCapabilities(true),
		mcpserver.WithToolCapabilities(true),
	)

	s := &Server{
		cfg:         cfg,
		mcpServer:   baseServer,
		stdioServer: mcpserver.NewStdioServer(baseServer),
		sseServer:   mcpserver.NewSSEServer(baseServer),
	}

	return s, nil
}

// MCPServer 返回底层 mark3labs/mcp-go 原生服务实例，供后续工单注册 Tools、Resources 与 Prompts。
//
// @return *mcpserver.MCPServer 底层 MCP 服务实例
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
}

// Serve 根据配置的 Transport 模式启动服务并阻塞监听，响应上下文取消信号以实现优雅停机。
//
// @param ctx 控制服务生命周期的上下文
// @return error 服务异常退出时返回错误，正常停机返回 nil
func (s *Server) Serve(ctx context.Context) error {
	switch s.cfg.Transport {
	case config.TransportStdio:
		return s.ServeStdio(ctx, os.Stdin, os.Stdout)
	case config.TransportSSE:
		return s.ServeSSE(ctx)
	default:
		return fmt.Errorf("未知的 transport 模式: %s", s.cfg.Transport)
	}
}

// ServeStdio 在指定的输入与输出流上运行 Stdio MCP 传输通道。
//
// @param ctx 生命周期上下文
// @param stdin 输入流（通常为 os.Stdin 或测试内存管道）
// @param stdout 输出流（通常为 os.Stdout 或测试内存管道）
// @return error 运行时错误，客户端正常断开 (io.EOF) 或主动取消返回 nil
func (s *Server) ServeStdio(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	err := s.stdioServer.Listen(ctx, stdin, stdout)
	if err != nil && (errors.Is(err, io.EOF) || errors.Is(err, context.Canceled)) {
		return nil
	}
	return err
}

// ServeSSE 启动 HTTP SSE 服务通道，并在上下文取消时执行优雅停机。
//
// @param ctx 控制生命周期的上下文
// @return error 运行或关闭过程中的错误，正常停机返回 nil
func (s *Server) ServeSSE(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	serverErrCh := make(chan error, 1)

	go func() {
		err := s.sseServer.Start(addr)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()
		if shutdownErr := s.sseServer.Shutdown(shutdownCtx); shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
			return fmt.Errorf("SSE 优雅停机失败: %w", shutdownErr)
		}
		return nil
	case err := <-serverErrCh:
		return err
	}
}
