// Package config 提供命令行参数与环境配置的解析、默认值设定及合法性校验。
//
// @author Ateng
// @since 2026-10-06
package config

import (
	"flag"
	"fmt"
	"io"
)

// TransportMode 定义支持的 MCP 协议通道传输类型枚举
type TransportMode string

const (
	// TransportStdio 表示基于标准输入输出的 MCP 传输协议通道
	TransportStdio TransportMode = "stdio"

	// TransportSSE 表示基于 HTTP Server-Sent Events 的流式传输协议通道
	TransportSSE TransportMode = "sse"

	// DefaultPort 表示 SSE 传输模式下的默认监听端口
	DefaultPort = 8080

	// MinPort 表示合法的最小 TCP 端口号
	MinPort = 1

	// MaxPort 表示合法的最大 TCP 端口号
	MaxPort = 65535
)

// Config 封装服务运行所需的基础配置与安全准入标志。
type Config struct {
	// Transport 指定 MCP 通信协议通道，支持 stdio 或 sse
	Transport TransportMode

	// Port 指定 SSE 模式下 HTTP 服务监听的 TCP 端口
	Port int

	// AllowWrite 标记是否激活具有修改副作用的 K8s 写操作工具（默认只读）
	AllowWrite bool

	// AllowExec 标记是否激活 Pod 容器命令执行排障探针
	AllowExec bool

	// Kubeconfig 指定显式配置的 Kubeconfig 文件路径
	Kubeconfig string

	// KubeContext 指定连接的目标集群上下文名称
	KubeContext string

	// ShowHelp 标记是否输出 CLI 帮助信息
	ShowHelp bool

	// ShowVersion 标记是否输出当前构建版本信息
	ShowVersion bool
}

// Parse 解析传入的命令行参数切片，并返回初始化且已验证的 Config 实例。
//
// @param args 命令行参数切片（不包含程序二进制名自身）
// @return *Config 解析生成的配置对象
// @return error 参数非法或校验失败时的错误信息
func Parse(args []string) (*Config, error) {
	cfg := &Config{
		Transport: TransportStdio,
		Port:      DefaultPort,
	}

	fs := flag.NewFlagSet("mcp-server-kubernetes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var transportStr string
	fs.StringVar(&transportStr, "transport", string(TransportStdio), "通信通道模式 (stdio|sse)")
	fs.IntVar(&cfg.Port, "port", DefaultPort, "SSE 模式监听端口")
	fs.BoolVar(&cfg.AllowWrite, "allow-write", false, "激活资源创建、变更与删除工具 (强制只读保护)")
	fs.BoolVar(&cfg.AllowExec, "allow-exec", false, "激活容器内部排障命令执行工具")
	fs.StringVar(&cfg.Kubeconfig, "kubeconfig", "", "显式指定 kubeconfig 配置文件路径")
	fs.StringVar(&cfg.KubeContext, "context", "", "指定默认连接的集群上下文名称")

	var helpLong, helpShort bool
	fs.BoolVar(&helpLong, "help", false, "显示帮助信息")
	fs.BoolVar(&helpShort, "h", false, "显示帮助信息 (简写)")

	var versionLong, versionShort bool
	fs.BoolVar(&versionLong, "version", false, "显示当前版本信息")
	fs.BoolVar(&versionShort, "v", false, "显示当前版本信息 (简写)")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("参数解析错误: %w", err)
	}

	cfg.Transport = TransportMode(transportStr)
	cfg.ShowHelp = helpLong || helpShort
	cfg.ShowVersion = versionLong || versionShort

	if cfg.ShowHelp || cfg.ShowVersion {
		return cfg, nil
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate 校验配置项是否在合法范围内。
//
// @return error 校验失败返回语义化错误，成功返回 nil
func (c *Config) Validate() error {
	if c.Transport != TransportStdio && c.Transport != TransportSSE {
		return fmt.Errorf("非法 transport 参数 %q: 仅支持 %q 或 %q", c.Transport, TransportStdio, TransportSSE)
	}

	if c.Port < MinPort || c.Port > MaxPort {
		return fmt.Errorf("非法 port 参数 %d: 必须在 %d 到 %d 之间", c.Port, MinPort, MaxPort)
	}

	return nil
}

// PrintUsage 输出格式化命令行帮助指南到指定的输出流。
//
// @param w 接收帮助信息的输出流
func PrintUsage(w io.Writer) {
	usage := `mcp-server-kubernetes - Model Context Protocol (MCP) Server for Kubernetes

使用方式:
  mcp-server-kubernetes [flags]

命令行参数 (Flags):
  --transport <stdio|sse>   通信通道模式 (默认: stdio)
  --port <port>             SSE 模式监听端口 (默认: 8080)
  --allow-write             激活资源创建、变更与删除工具 (默认: false, 强制只读)
  --allow-exec              激活容器内部排障命令执行工具 (默认: false)
  --kubeconfig <path>       显式指定 kubeconfig 配置文件路径
  --context <name>          指定默认连接的集群上下文名称
  -h, --help                显示帮助信息
  -v, --version             显示当前版本信息
`
	_, _ = fmt.Fprint(w, usage)
}
