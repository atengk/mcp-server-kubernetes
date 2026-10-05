# Agent 协作与工程规范

本项目已集成工程化技能体系（Engineering Skills）并确立了严格的架构与安全设计准则。所有面向本代码库的自动化 AI Coding Agent 必须严格遵守以下规范。

---

## 1. 架构设计军规 (Architecture Invariants)

- **双栈工程分工边界**：
  - Go 核心工程严格收敛在根目录、`cmd/`、`internal/`；所有与 Kubernetes API 交互、MCP 协议处理的逻辑均在 Go 侧实现；
  - Node.js 包装层严格收敛在 `npm/` 目录下；该层仅负责通过 `process.platform` 与 `process.arch` 解析原生可执行文件路径并以子进程启动，严禁在该层编写业务逻辑。
- **领域模型与术语严守**：
  - 编码、注释、Issue、PR 标题及文档中的术语必须 100% 遵循 [CONTEXT.md](./CONTEXT.md)；严禁使用词汇表已明确 `_Avoid_` 的同义词或生造词。
- **架构决策变更流程**：
  - 任何涉及协议调整、分发机制变更或安全模型重构的工作，必须首先查阅并遵循 [docs/adr/](./docs/adr/) 中的既定决策；如有不可避免的冲突或修正，必须先行增补或修订 ADR。
- **版本单一真实信源 (Tag SSOT)**：
  - 代码库与所有配置文件中严禁硬编码静态版本号；版本的唯一信源为 Git Tag，由 CI/CD 发版流水线在构建期动态注入二进制与 `package.json`。

---

## 2. 编码与安全开发红线 (Coding & Security Guardrails)

- **只读默认与写权限门禁**：
  - 任何具有副作用的工具（`apply`、`scale`、`delete`）必须被封装在写权限校验之后，只有配置了 `--allow-write` 启动参数才允许向客户端暴露注册；
  - 容器内命令执行（`exec`）必须受 `--allow-exec` 独立开关门禁管辖，并强制配置 15 秒超时熔断与 64KB 缓冲区截断。
- **Smart Pruning 强制降噪**：
  - 所有由 Kubernetes API 返回并向客户端暴露的资源数据，必须前置通过清洗管道剔除 `metadata.managedFields`、`last-applied-configuration` 注解及空节点；
  - Secret 资源必须在输出前强制执行脱敏：保留键名（Key）与元数据，所有数据值掩码为 `[REDACTED]`。
- **上下文防爆与空指针防御**：
  - 所有 K8s API 访问必须显式传递 `context.Context`，杜绝悬挂请求；
  - 查询列表无匹配对象时，必须返回空切片（`[]`）而严禁返回 `nil`。
  - 列表与日志工具必须遵从 Token Guard 规范，超量输出必须追加截断告警。

---

## 3. Agent 技能契约 (Agent Skills)

### Issue tracker

使用 GitHub Issues 跟踪问题与需求任务（基于 `gh` CLI 操作）。详见 [issue-tracker.md](./docs/agents/issue-tracker.md)。

### Triage labels

采用标准的 5 种分类标签体系（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`）。详见 [triage-labels.md](./docs/agents/triage-labels.md)。

### Domain docs

单上下文架构（Single-context），根目录包含 [CONTEXT.md](./CONTEXT.md)，架构决策记录位于 [docs/adr/](./docs/adr/)。详见 [domain.md](./docs/agents/domain.md)。
