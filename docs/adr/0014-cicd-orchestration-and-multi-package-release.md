# 0014. CI/CD 流水线编排与全自动化多包发版

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

本项目涉及 Go 原生多架构交叉编译、npm 平台子包与主包的有序发布、GitHub Releases 附件分发以及容器化 Docker 镜像推送。发版流程若缺乏严密的编排，容易产生包版本不一致、部分子包漏发或版本号维护冲突。

## 架构决策

1. **双重保障 CI 流水线 (`ci.yml`)**：
   - PR 语义化标题检查（Conventional Commits）；
   - Go 单元测试与代码质检（`go vet`、`go test`）；
   - 五大平台（macOS arm64/x64、Linux amd64/arm64、Windows x64）交叉编译演练（Dry-Run）；
   - Node Wrapper 语法合规性检查。
2. **单 Runner 强一致发版编排 (`release.yml`)**：
   - **Tag SSOT**：以 Git Tag（如 `v1.0.0`）作为单一真实信源，通过 ldflags（`-X main.Version=1.0.0`）动态注入 Go 二进制，动态生成 npm `package.json` 版本；
   - **顺序发布**：同一 Runner 内完成 5 大平台编译与打包校验和 -> 并行发布 5 个平台子包（`@atengk/mcp-server-kubernetes-<os>-<arch>`）-> 发布主包（`@atengk/mcp-server-kubernetes`）；
   - **GHCR 镜像分发**：同步构建多架构（`linux/amd64`, `linux/arm64`）Docker 镜像并推送到 `ghcr.io/atengk/mcp-server-kubernetes`。

## 架构后果

- **正面收益**：零本地手动版本号维护负担；一次打 Tag 即可自动化完成全渠道分发（GitHub Releases + npm 跨平台矩阵 + Docker 容器镜像）；强一致性杜绝子包裂化。
- **潜在代价**：需妥善管理 `NPM_TOKEN` 并在仓库 Settings 授予 Actions 对 Packages 和 Releases 的写入权限。
