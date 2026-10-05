# Agent 协作与工程规范

本项目已集成工程化技能体系（Engineering Skills），以下为针对各类自动化 Agent 的规范说明。

## Agent skills

### Issue tracker

使用 GitHub Issues 跟踪问题与需求任务（基于 `gh` CLI 操作）。详见 [issue-tracker.md](./docs/agents/issue-tracker.md)。

### Triage labels

采用标准的 5 种分类标签体系（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`）。详见 [triage-labels.md](./docs/agents/triage-labels.md)。

### Domain docs

单上下文架构（Single-context），根目录包含 `CONTEXT.md`，架构决策记录位于 `docs/adr/`。详见 [domain.md](./docs/agents/domain.md)。
