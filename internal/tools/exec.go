// Package tools 封装 Kubernetes 容器命令执行探针与安全防御门禁。
//
// @author Ateng
// @since 2026-10-06
package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/safety"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	// DefaultExecTimeout 声明容器内命令执行的最大超时熔断时间（15 秒）
	DefaultExecTimeout = 15 * time.Second

	// MaxExecBufferBytes 声明输出缓冲区的硬上限（64KB）
	MaxExecBufferBytes = 64 * 1024
)

// ExecOptions 封装容器命令执行的入参选项。
type ExecOptions struct {
	// PodName 目标 Pod 名称
	PodName string
	// Namespace 目标命名空间
	Namespace string
	// Container 目标容器名称（可选）
	Container string
	// Command 待执行的命令及其参数切片
	Command []string
}

// ExecRunnerFunc 定义由底层 SPDY/Websocket 执行容器命令的原型函数。
//
// @param ctx 控制执行生命周期与超时的上下文
// @param client Kubernetes 集群客户端接口
// @param rc 集群底层 REST 配置对象
// @param opts 容器命令执行参数配置
// @return stdout 标准输出文本
// @return stderr 标准错误输出文本
// @return exitCode 命令执行退出码
// @return err 执行异常错误
type ExecRunnerFunc func(ctx context.Context, client kubernetes.Interface, rc *rest.Config, opts ExecOptions) (stdout, stderr string, exitCode int, err error)

type execConfig struct {
	runner ExecRunnerFunc
}

// ExecOption 定义配置 Exec 工具行为的函数选项。
//
// @param c 目标配置结构指针
type ExecOption func(*execConfig)

// WithExecRunner 注入自定义 Exec 执行器（主要用于单元测试与模拟环境）。
//
// @param fn 自定义执行回调
// @return ExecOption 配置函数
func WithExecRunner(fn ExecRunnerFunc) ExecOption {
	return func(c *execConfig) {
		c.runner = fn
	}
}

// RegisterExecTools 根据安全开关决定是否向 MCP Server 注册容器探针命令执行工具。
// 遵循 ADR 0008 军规：未显式开启 allowExec 时完全隐藏并拒绝注册。
//
// @param s MCP 服务端实例
// @param mgr Kubernetes 客户端管理器
// @param allowExec 是否激活 Exec 执行门禁
// @param opts 自定义函数选项
// @return error 注册异常
func RegisterExecTools(s *mcpserver.MCPServer, mgr *k8s.ClientManager, allowExec bool, opts ...ExecOption) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}
	if mgr == nil {
		return errors.New("k8s client manager 实例不可为空")
	}

	// 1. 安全门禁拦截：未显式开启时完全隐藏不注册
	if !allowExec {
		return nil
	}

	cfg := execConfig{
		runner: defaultSPDYExecRunner,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	// 2. 激活注册 k8s_exec_command
	s.AddTool(
		mcp.NewTool("k8s_exec_command",
			mcp.WithDescription("在指定 Pod 容器内部执行非交互式单次诊断命令 (受 15 秒超时熔断与 64KB 缓冲区截断管辖)"),
			mcp.WithString("pod_name", mcp.Required(), mcp.Description("目标 Pod 名称")),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Pod 所在的命名空间")),
			mcp.WithString("command", mcp.Required(), mcp.Description("待执行的命令或参数列表")),
			mcp.WithString("container", mcp.Description("目标容器名称 (未指定则默认为 Pod 内首个容器)")),
			mcp.WithString("context", mcp.Description("目标集群上下文名称 (可选)")),
		),
		makeExecCommandHandler(mgr, cfg.runner),
	)

	return nil
}

// makeExecCommandHandler 构建容器内执行命令的 MCP 处理器。
func makeExecCommandHandler(mgr *k8s.ClientManager, runner ExecRunnerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		podName, err := request.RequireString("pod_name")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 pod_name"), nil
		}
		namespace, err := request.RequireString("namespace")
		if err != nil {
			return mcp.NewToolResultError("缺少必填参数 namespace"), nil
		}

		commandSlice, err := extractCommandSlice(request)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("命令参数解析失败: %v", err)), nil
		}
		if len(commandSlice) == 0 {
			return mcp.NewToolResultError("待执行的命令切片不可为空"), nil
		}

		container := request.GetString("container", "")
		contextName := request.GetString("context", "")

		client, err := mgr.GetClient(contextName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取集群客户端失败: %v", err)), nil
		}

		rc, _ := mgr.GetRESTConfig(contextName)

		// 强制应用 15 秒超时熔断机制
		execCtx, cancel := context.WithTimeout(ctx, DefaultExecTimeout)
		defer cancel()

		opts := ExecOptions{
			PodName:   podName,
			Namespace: namespace,
			Container: container,
			Command:   commandSlice,
		}

		stdout, stderr, exitCode, err := runner(execCtx, client, rc, opts)
		if err != nil {
			if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
				return mcp.NewToolResultError("容器命令执行超时 (超出 15 秒安全熔断上限)"), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("容器命令执行失败: %v", err)), nil
		}

		combinedOutput := stdout
		if stderr != "" {
			if combinedOutput != "" {
				combinedOutput += "\n[STDERR]\n" + stderr
			} else {
				combinedOutput = "[STDERR]\n" + stderr
			}
		}

		// 64KB 缓冲区硬上限截断
		if len(combinedOutput) > MaxExecBufferBytes {
			combinedOutput = combinedOutput[:MaxExecBufferBytes]
			combinedOutput += safety.FormatTruncationWarning("超过 64KB 缓冲区上限")
		}

		if exitCode != 0 {
			combinedOutput = fmt.Sprintf("命令执行完成 (退出码: %d):\n%s", exitCode, combinedOutput)
		}

		return mcp.NewToolResultText(combinedOutput), nil
	}
}

// extractCommandSlice 从参数中提取命令切片（支持字符串切片、任意元素切片或单个字符串）。
func extractCommandSlice(request mcp.CallToolRequest) ([]string, error) {
	if slice, err := request.RequireStringSlice("command"); err == nil && len(slice) > 0 {
		return slice, nil
	}
	if str, err := request.RequireString("command"); err == nil && len(str) > 0 {
		return []string{"sh", "-c", str}, nil
	}
	args := request.GetArguments()
	if rawVal, ok := args["command"]; ok {
		switch v := rawVal.(type) {
		case []any:
			res := make([]string, 0, len(v))
			for _, item := range v {
				res = append(res, fmt.Sprintf("%v", item))
			}
			return res, nil
		case []string:
			if v == nil {
				return []string{}, nil
			}
			return v, nil
		}
	}
	return []string{}, errors.New("缺少 command 参数或类型不匹配")
}

// defaultSPDYExecRunner 原生 SPDY Executor 实现。
func defaultSPDYExecRunner(ctx context.Context, client kubernetes.Interface, rc *rest.Config, opts ExecOptions) (string, string, int, error) {
	if rc == nil {
		return "", "", 1, errors.New("集群 REST 配置不可为空")
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(opts.PodName).
		Namespace(opts.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.Container,
			Command:   opts.Command,
			Stdout:    true,
			Stderr:    true,
			Stdin:     false,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(rc, "POST", req.URL())
	if err != nil {
		return "", "", 1, fmt.Errorf("创建 SPDY 执行器失败: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &limitWriter{w: &stdoutBuf, max: MaxExecBufferBytes},
		Stderr: &limitWriter{w: &stderrBuf, max: MaxExecBufferBytes},
		Tty:    false,
	})

	return stdoutBuf.String(), stderrBuf.String(), 0, err
}

// limitWriter 防止内存无节制膨胀的受限写入流。
type limitWriter struct {
	w       io.Writer
	max     int
	written int
}

func (l *limitWriter) Write(p []byte) (n int, err error) {
	if l.written >= l.max {
		return len(p), nil
	}
	remaining := l.max - l.written
	if len(p) > remaining {
		n, err = l.w.Write(p[:remaining])
		l.written += n
		return len(p), err
	}
	n, err = l.w.Write(p)
	l.written += n
	return n, err
}
