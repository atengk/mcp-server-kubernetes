// Package prompts 提供针对 Kubernetes 核心排障与集群运维的 MCP 预置 Prompts 工作流模板。
//
// @author Ateng
// @since 2026-10-06
package prompts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterPrompts 向 MCP Server 注册三大预置运维工作流 Prompts 模板。
//
// @param s MCP 服务端实例
// @return error 注册异常
func RegisterPrompts(s *mcpserver.MCPServer) error {
	if s == nil {
		return errors.New("mcp server 实例不可为空")
	}

	// 1. 注册 diagnose-pod-failure 模板
	s.AddPrompt(
		mcp.NewPrompt("diagnose-pod-failure",
			mcp.WithPromptDescription("针对故障 Pod 生成标准化多源排障编排指南，指导按部就班检索状态、日志与关联事件并定位根因"),
			mcp.WithArgument("namespace",
				mcp.RequiredArgument(),
				mcp.ArgumentDescription("目标 Pod 所在的命名空间"),
			),
			mcp.WithArgument("pod_name",
				mcp.RequiredArgument(),
				mcp.ArgumentDescription("故障 Pod 的名称"),
			),
			mcp.WithArgument("context",
				mcp.ArgumentDescription("目标集群上下文名称 (可选)"),
			),
		),
		handleDiagnosePodFailure,
	)

	// 2. 注册 cluster-health-check 模板
	s.AddPrompt(
		mcp.NewPrompt("cluster-health-check",
			mcp.WithPromptDescription("生成集群整体健康状况全面巡检提示词，涵盖节点状态、异常 Pod 扫描与告警事件评估"),
			mcp.WithArgument("context",
				mcp.ArgumentDescription("目标集群上下文名称 (可选)"),
			),
		),
		handleClusterHealthCheck,
	)

	// 3. 注册 workload-security-audit 模板
	s.AddPrompt(
		mcp.NewPrompt("workload-security-audit",
			mcp.WithPromptDescription("生成基于 Pod 安全标准 (PSS) 的工作负载合规审计提示词，检查特权模式、Root 运行与宿主机穿透"),
			mcp.WithArgument("namespace",
				mcp.ArgumentDescription("目标命名空间 (未指定则审计全集群范围)"),
			),
			mcp.WithArgument("context",
				mcp.ArgumentDescription("目标集群上下文名称 (可选)"),
			),
		),
		handleWorkloadSecurityAudit,
	)

	return nil
}

// handleDiagnosePodFailure 处理 diagnose-pod-failure 提示词生成请求。
func handleDiagnosePodFailure(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	namespace := req.Params.Arguments["namespace"]
	if strings.TrimSpace(namespace) == "" {
		return nil, errors.New("缺少必填参数 namespace")
	}

	podName := req.Params.Arguments["pod_name"]
	if strings.TrimSpace(podName) == "" {
		return nil, errors.New("缺少必填参数 pod_name")
	}

	contextHint := formatContextHint(req)

	guideText := fmt.Sprintf(`# Kubernetes Pod 故障排障诊断工作流 (Pod: %s, Namespace: %s%s)

请按以下标准化排障编排顺序调用 MCP 工具，系统化排查该 Pod 的故障根因：

1. **获取 Pod 状态详情与关联事件**：
   - 调用工具：`+"`"+`k8s_describe_resource(kind="Pod", namespace="%s", name="%s")`+"`"+`
   - 重点审查：Pod Phase、容器 State（如 CrashLoopBackOff、OOMKilled、ImagePullBackOff 等）、Exit Code 退出码以及最近关联 Events 告警。

2. **抓取容器运行日志与崩溃前日志**：
   - 若容器处于崩溃重启循环，优先调用：`+"`"+`k8s_get_pod_logs(namespace="%s", name="%s", previous=true)`+"`"+` 提取崩溃前的错误堆栈与致命异常；
   - 调用：`+"`"+`k8s_get_pod_logs(namespace="%s", name="%s")`+"`"+` 检索当前容器输出日志。

3. **核查资源限制与健康探针**：
   - 检查 CPU/Memory Limits 与 Requests，核实是否存在内存超限 (OOM) 或资源耗尽；
   - 检查 Liveness/Readiness/Startup Probes 探针端口、路径与超时配置，评估是否因探针失败触发重启。

4. **级联拓扑与环境诊断**：
   - 若疑为级联配置问题，调用 `+"`"+`k8s_get_resource_tree`+"`"+` 或 `+"`"+`k8s_get_events(namespace="%s")`+"`"+` 确认所属控制器及邻近服务状态。

5. **总结与修复方案**：
   - 明确故障根本原因 (Root Cause)；
   - 提供具体可行的修复配置或调整建议。
`, podName, namespace, contextHint, namespace, podName, namespace, podName, namespace, podName, namespace)

	msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(guideText))
	return mcp.NewGetPromptResult("Pod 故障多源排障指导流程", []mcp.PromptMessage{msg}), nil
}

// handleClusterHealthCheck 处理 cluster-health-check 提示词生成请求。
func handleClusterHealthCheck(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	contextHint := formatContextHint(req)

	guideText := fmt.Sprintf(`# Kubernetes 集群健康状况巡检工作流%s

请按以下标准化步骤调用 MCP 工具，全面巡检并评估集群核心健康度：

1. **集群版本与节点就绪水位检查**：
   - 访问集群概览资源：`+"`"+`k8s://cluster/overview`+"`"+`
   - 调用工具：`+"`"+`k8s_list_resources(kind="Node")`+"`"+`
   - 检查要点：所有节点的 Ready 状态，是否存在 MemoryPressure、DiskPressure、PIDPressure 等调度污点与异常 Condition。

2. **全集群异常 Pod 状态扫描**：
   - 调用工具：`+"`"+`k8s_list_resources(kind="Pod")`+"`"+`
   - 检查要点：排查处于 Pending、CrashLoopBackOff、Error、ImagePullBackOff 等非 Running/Succeeded 状态的工作负载，统计故障分布。

3. **全集群关键警告事件检索**：
   - 调用工具：`+"`"+`k8s_get_events(type="Warning")`+"`"+`
   - 检查要点：检索近期 Warning 级别告警事件（FailedScheduling、FailedMount、BackOff 等），识别底层调度与存储瓶颈。

4. **核心系统命名空间自检**：
   - 重点关注 `+"`"+`kube-system`+"`"+` 中的 DNS、CNI 与 Ingress 控制面组件运行状态。

5. **生成巡检汇总报告**：
   - 给出集群当前健康评级 (Healthy / Warning / Critical)；
   - 整理潜在隐患清单与治理建议。
`, contextHint)

	msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(guideText))
	return mcp.NewGetPromptResult("集群健康巡检指导流程", []mcp.PromptMessage{msg}), nil
}

// handleWorkloadSecurityAudit 处理 workload-security-audit 提示词生成请求。
func handleWorkloadSecurityAudit(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	namespace := req.Params.Arguments["namespace"]
	targetScope := "全集群命名空间"
	if namespace != "" {
		targetScope = fmt.Sprintf("命名空间 %s", namespace)
	}

	contextHint := formatContextHint(req)

	guideText := fmt.Sprintf(`# Kubernetes 工作负载安全合规审计工作流 (范围: %s%s)

请依据 Kubernetes Pod 安全标准 (Baseline / Restricted)，按以下步骤调用 MCP 工具展开合规审计：

1. **抓取目标作用域的工作负载实例**：
   - 调用工具：`+"`"+`k8s_list_resources(kind="Pod"%s)`+"`"+`，获取待审计的 Pod 资源清单。

2. **特权容器与权限提升审计**：
   - 检查是否配置了 `+"`"+`securityContext.privileged: true`+"`"+` 特权模式；
   - 检查是否禁用了提权（`+"`"+`allowPrivilegeEscalation: false`+"`"+`）。

3. **运行身份与 Root 用户审计**：
   - 检查是否配置了 `+"`"+`runAsNonRoot: true`+"`"+`；
   - 检查是否避免了以 root 用户 (UID 0) 启动容器。

4. **宿主机穿透与隔离边界审计**：
   - 检查是否存在危险的宿主机共享：`+"`"+`hostNetwork: true`+"`"+`、`+"`"+`hostPID: true`+"`"+`、`+"`"+`hostIPC: true`+"`"+`。

5. **Linux Capabilities 权能审计**：
   - 检查是否按最小权限原则丢弃权能：`+"`"+`capabilities.drop: ["ALL"]`+"`"+`；
   - 检查是否违规添加了高危权能（如 CAP_SYS_ADMIN）。

6. **只读文件系统与卷挂载审计**：
   - 检查是否启用 `+"`"+`readOnlyRootFilesystem: true`+"`"+`；
   - 检查是否存在高危 hostPath 挂载（如 /var/run/docker.sock、/etc 等）。

7. **生成安全审计与加固报告**：
   - 汇总不合规 Pod 列表及严重等级；
   - 提供合规加固的 YAML 补丁建议。
`, targetScope, contextHint, func() string {
		if namespace != "" {
			return fmt.Sprintf(`, namespace="%s"`, namespace)
		}
		return ""
	}())

	msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(guideText))
	return mcp.NewGetPromptResult("工作负载安全合规审计指导流程", []mcp.PromptMessage{msg}), nil
}

// formatContextHint 从 Prompt 请求中提取集群上下文参数并格式化显示提示。
//
// @param req MCP Prompt 获取请求对象
// @return string 格式化的上下文提示信息（无上下文时返回空字符串）
func formatContextHint(req mcp.GetPromptRequest) string {
	if req.Params.Arguments == nil {
		return ""
	}
	clusterContext := strings.TrimSpace(req.Params.Arguments["context"])
	if clusterContext != "" {
		return fmt.Sprintf(" (集群上下文: %s)", clusterContext)
	}
	return ""
}
