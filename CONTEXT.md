# MCP Server Kubernetes

基于模型上下文协议（Model Context Protocol, MCP）构建的 Kubernetes 集群交互与智能运维底座。

## 核心概念与领域术语

**MCP Server**:
实现 MCP 规范的服务进程，向 AI Agent 暴露 Kubernetes 相关的 Tools、Prompts 与 Resources。
_Avoid_: Agent Plugin, K8s Copilot

**Native Binary**:
由 Go 语言直接编译生成的各平台目标机器码原生可执行文件（ELF / Mach-O / PE）。
_Avoid_: Go Script, Bytecode

**Node Wrapper**:
基于 Node.js 实现的轻量可执行分发包装层，用于发布至 npm 仓库并支持用户通过 `npx` 零依赖直接拉起对应平台的 Native Binary。
_Avoid_: Node SDK, NPM Service

**Platform Sub-packages**:
按平台架构划分的 npm 专属子包（`@atengk/mcp-server-kubernetes-<os>-<arch>`），承载具体平台的 Native Binary，由主包通过 `optionalDependencies` 矩阵按需拉取。
_Avoid_: Fat Package, Dynamic Downloader

**Transport Layer**:
MCP Server 与客户端之间的通信协议通道，包含标准输入输出（Stdio）与服务端推送流（SSE）。
_Avoid_: Network Protocol, RPC Client

**Safety Guard**:
运行时的安全准入拦截与防御机制，默认强制只读模式，并对 Secret 等敏感资产的数据内容执行脱敏。
_Avoid_: FireWall, Privilege Check

**Redacted Secret**:
经过安全脱敏处理后的 Secret 视图，保留其配置键名（Keys）与元数据，但彻底遮蔽具体内容数值。
_Avoid_: Masked Config, Secret Dump

**Context Routing**:
动态路由机制，支持在单次请求中依据上下文参数动态调度目标 Kubernetes 集群凭据。
_Avoid_: Cluster Switcher, Multi-Tenancy

**Token Guard**:
服务端对超长日志与海量列表输出的安全截断与约束机制，防止单次响应撑爆 AI 模型的上下文窗口。
_Avoid_: Rate Limiter, Response Trimmer

**Resource URI**:
以 `k8s://` 为统一 Scheme 的 MCP 只读上下文资源端点，供 AI Agent 直接挂载集群元数据与运行时概览。
_Avoid_: Webhook, REST URL

**Resource Template**:
符合 RFC 6570 规范的参数化资源 URI 模式（如 `k8s://{namespace}/pods/{name}`），支持动态挂载具体工作负载上下文。
_Avoid_: URL Pattern, Dynamic Route

**Smart Pruning**:
服务端对 Kubernetes 原始响应结构的主动修剪降噪管道，剔除 `managedFields` 等噪音，节约 70% 以上 Token 消耗。
_Avoid_: Data Filter, JSON Minifier

**Dry-Run Preview**:
基于 Kubernetes Server-Side Dry-Run 机制生成的增量 Unified Diff，为 AI 变更操作提供人类可读的预检缓冲垫。
_Avoid_: Virtual Exec, Mock Update

**Progress Notification**:
基于 MCP 原生协议的流式进度通知机制，在执行长耗时复合操作时持续向客户端推送阶段进度与状态。
_Avoid_: Polling Status, Async Callback

**Prompt Template**:
预置在 MCP Server 内的高频智能运维交互模版（如 `diagnose-pod-failure`、`cluster-health-check` 等），引导 AI 分步完成复合排障。
_Avoid_: System Instruction, Chat Memory

**Dynamic Resource**:
基于 `client-go/dynamic` 动态反射与发现机制解析的非内置自定义资源（CRD）。
_Avoid_: Generic Object, Raw JSON

**Diagnostic Exec**:
在受控安全门禁（`--allow-exec`）保护下向目标容器注入的单次非交互式命令探测。
_Avoid_: SSH Shell, Interactive Terminal

**Resource Tree**:
以 OwnerReferences 和 LabelSelectors 自动级联抓取的多层级 Kubernetes 资源依赖与关联拓扑树。
_Avoid_: Graph Database, Object Hierarchy

**RBAC Can-I**:
基于 Kubernetes 原生 AccessReview API 的鉴权自检探针，用于评估指定主体在特定作用域下的操作权限。
_Avoid_: IAM Policy, Role Matrix

**Tag SSOT**:
以 Git 语义化标签作为项目版本的单一真实信源（Single Source of Truth），发版流水线动态注入版本信息，消除仓库内的冗余版本配置文件。
_Avoid_: Manual Bump, Hardcoded Version
