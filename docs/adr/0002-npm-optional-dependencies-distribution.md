# 0002. 基于 npm optionalDependencies 矩阵分发原生多架构二进制

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

为了让 AI 开发者通过 `npx` 零依赖唤起由 Go 编写的 Native Binary，需要确定二进制在 npm 生态中的打包与分发机制。

## 架构决策

我们决定采纳 **npm optionalDependencies 平台专属子包矩阵方案**（对齐现代 `esbuild`、`@biomejs/biome` 的工业级实践）：
1. 主包（CLI 入口包）仅包含轻量 JS 调度胶水代码，声明依赖各平台的 `optionalDependencies`；
2. 针对各主流平台分别打包发布专有二进制子包（如 `darwin-arm64`、`linux-x64`、`win32-x64` 等）；
3. 运行时调度脚本通过 `process.platform` 与 `process.arch` 解析对应的原生二进制路径，以子进程直接启动。

## 备选方案权衡

- **运行时动态从 GitHub Release 下载**：在无外网代理或公司私有内网环境下极易超时或受限，且破坏了 npm 离线缓存机制。
- **单包内嵌全部平台二进制（胖包）**：每次安装均强制下载无关平台的冗余体积（40MB+）。
- **optionalDependencies 平台子包**：npm 原生在安装期仅拉取适配当前宿主机平台的单个子包（仅约 10MB），天然兼容公司私有 npm 源镜像，零运行时网络开销。

## 架构后果

- **正面收益**：用户体验最佳，安装速度最快，完全免疫运行期网络波动。
- **潜在代价**：CI/CD 发版流水线需要通过矩阵自动化并行发布多个子包与主包。
