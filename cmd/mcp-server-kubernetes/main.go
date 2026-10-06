// Package main 是 mcp-server-kubernetes 应用程序的装配入口，负责解析启动参数、监听操作系统信号并调度核心服务生命周期。
//
// @author Ateng
// @since 2026-10-06
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/atengk/mcp-server-kubernetes/internal/config"
	"github.com/atengk/mcp-server-kubernetes/internal/server"
	"github.com/atengk/mcp-server-kubernetes/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("服务运行异常退出", "error", err)
		os.Exit(1)
	}
}

// run 承载主进程装配与执行逻辑，便于测试注入与错误隔离。
//
// @param args 命令行入参（不含程序名）
// @return error 运行期产生的异常信息
func run(args []string) error {
	// 1. 解析与校验命令行参数
	cfg, err := config.Parse(args)
	if err != nil {
		config.PrintUsage(os.Stderr)
		return fmt.Errorf("参数解析失败: %w", err)
	}

	// 2. 拦截与响应帮助与版本等即时指令
	if cfg.ShowHelp {
		config.PrintUsage(os.Stdout)
		return nil
	}

	if cfg.ShowVersion {
		fmt.Println(version.GetInfo().String())
		return nil
	}

	// 3. 构建可响应系统信号的生命周期上下文
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. 装配核心服务实例并启动
	srv, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("初始化 MCP 服务失败: %w", err)
	}

	if cfg.Transport == config.TransportSSE {
		slog.Info("正在启动 MCP SSE 服务", "port", cfg.Port, "version", version.Version)
	}

	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("服务监听异常: %w", err)
	}

	return nil
}
