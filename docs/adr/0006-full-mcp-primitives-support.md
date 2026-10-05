# 0006. 全量支持 MCP 三大核心原语 (Tools, Resources, Prompts)

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

Model Context Protocol（MCP）规范由 Tools（主动工具）、Resources（只读上下文数据源）与 Prompts（场景提示词模版）三大核心原语组成。单纯实现 Tools 无法充分发挥 AI 客户端（如 Claude Desktop、Cursor）对静态上下文与预制工作流的协同编排能力。

## 架构决策

全面支持 MCP 三大核心原语：
1. **Resources**：建立 `k8s://` 统一资源定位体系：
   - `k8s://contexts`：暴露可用集群上下文列表；
   - `k8s://cluster/overview`：暴露集群版本、节点状态、总配额与核心指标只读快照；
   - `k8s://namespaces`：暴露命名空间及其活跃状态；
   - `k8s://api-resources`：暴露集群已安装的全部 API 组与 CRD 元数据。
2. **Prompts**：内置工业级智能运维与排障工作流模版：
   - `diagnose-pod-failure`：针对 CrashLoopBackOff、OOMKilled 等异常 Pod 聚合多源诊断；
   - `cluster-health-check`：对未就绪节点、驱逐事件、调度受阻资源进行全景健康巡检；
   - `workload-security-audit`：对工作负载的特权容器与安全配置进行审计评估。
3. **Tools**：承载主动执行类排障、探针、查询与（受控）变更动作。

## 架构后果

- **正面收益**：AI 客户端可将集群上下文直接作为系统知识注入，并将专家级排障经验一键转化为可复用的结构化工作流。
- **潜在代价**：需要同步维护 Resources 的实时性映射与 Prompts 的国际化/参数校验逻辑。
