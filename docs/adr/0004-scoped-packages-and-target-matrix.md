# 0004. 组织作用域包命名与五大主流平台编译矩阵

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

为了在 npm 注册表中明确所有权归属并规避包名抢注冲突，同时保障跨不同操作系统与硬件架构的 AI 开发者均可直接运行。

## 架构决策

1. **组织作用域命名空间**：
   - 主包（CLI 入口）：`@atengk/mcp-server-kubernetes`
   - 平台子包：`@atengk/mcp-server-kubernetes-<os>-<arch>`
2. **五大主流平台编译矩阵**：
   - `darwin-arm64`（Apple Silicon Mac / M1-M4）
   - `darwin-x64`（Intel Mac）
   - `linux-x64`（主流 Linux / WSL）
   - `linux-arm64`（ARM64 Linux 服务器 / 云主机）
   - `win32-x64`（Windows 64 位）

## 架构后果

- **正面收益**：npm 命名归属清晰，天然免疫恶意抢注；一次性覆盖 99% 以上桌面开发与云原生服务器环境。
- **潜在代价**：CI 发布流水线固定为 5 个目标编译并分发。
