# 0001. Go 原生二进制内核与 Node.js npm 包装层混合分发架构

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

开发 Kubernetes MCP 服务端需要深度调用 K8s API，并要求极其广泛便捷的用户分发体验。在技术选型中，Go 拥有原生的官方 `client-go` 与出色的静态编译性能；而 AI 工具链（如 Cursor、Claude Desktop 等）普遍优先推荐通过 `npx` 零依赖启动 MCP Server。

## 架构决策

我们决定采用**双层混合架构**：
1. **核心服务端**：采用 Go 语言（搭配 `client-go` 与 `mark3labs/mcp-go`）编写核心逻辑，编译为各平台原生跨平台二进制文件（Native Binary）；
2. **分发包装层**：采用轻量 Node.js 编写可执行 Wrapper，发布至 npm 注册表，在运行时自动探测操作系统与芯片架构，调度执行对应的原生二进制进程。

## 备选方案权衡

- **全量 Node.js / TypeScript 实现**：通过 `@kubernetes/client-node` 实现。未采纳原因在于社区维护度与对 CRD/最新 K8s 特性的支持深度不及 Go 原生生态的 `client-go`。
- **纯 Go 分发（仅通过 GitHub Releases / Homebrew / go install）**：未采纳原因在于增加了 Node/AI 开发者在配置 Claude Desktop / Cursor 时的环境依赖与安装摩擦力，无法实现 `npx` 零配置一键唤起。

## 架构后果

- **正面收益**：兼具 Go 语言与 K8s 官方生态的最高契合度、高并发性能，同时获得 npm 生态的极致分发生态体验。
- **潜在代价**：CI/CD 流水线需要维护双重构建与发布机制（GoReleaser 跨平台交叉编译 + npm 发布流水线）。
