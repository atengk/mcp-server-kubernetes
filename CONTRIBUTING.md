# 贡献指南 (Contributing Guide)

感谢你关注并愿意为本项目贡献力量！为了保持高效协作与高质量的代码维护，请在提交代码前阅读以下规范。

---

## 1. 协作与分支模型

本项目遵循标准的 **GitHub Flow** 工作流：

1. **Fork 本仓库** 到你个人的 GitHub 账号；
2. **基于 `main` 分支拉取新的特性分支**：
   ```bash
   git checkout -b feat/your-feature-name
   # 或者缺陷修复分支
   git checkout -b fix/issue-description
   ```
3. 在本地完成修改，确保自测通过并补充相应测试用例；
4. 提交更改并推送到你的远程分支：
   ```bash
   git push origin feat/your-feature-name
   ```
5. 在 GitHub 上向本仓库的 `main` 分支发起 **Pull Request**。

---

## 2. Commit 提交信息规范

本项目遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/) 规范，统一采用以下格式：

```text
<type>(<scope>): <subject>
```

### 常用类型说明

| 类型 | 说明 | 示例 |
| :--- | :--- | :--- |
| `feat` | 新增功能或特性 | `feat(auth): 支持 OAuth2 登录授权` |
| `fix` | 缺陷与 Bug 修复 | `fix(parser): 修复空字符串解析导致空指针的问题` |
| `docs` | 仅文档更新或修改 | `docs: 完善快速开始与环境配置文档` |
| `style` | 代码格式调整（空格、分号等，不影响逻辑） | `style: 优化代码排版与换行` |
| `refactor` | 代码重构（既非新增特性也非修复缺陷） | `refactor(core): 抽取公共工具类` |
| `perf` | 性能优化 | `perf(cache): 引入本地二级缓存提升吞吐量` |
| `test` | 增加或重构单元测试与集成测试 | `test: 补充用户服务边界条件测试用例` |
| `build` | 构建系统、外部依赖或脚手架调整 | `build: 升级依赖版本至最新稳定版` |
| `ci` | CI/CD 流水线与 GitHub Actions 脚本修改 | `ci: 优化 release 自动化发版流程` |
| `chore` | 其他琐碎杂项（不改动源码与测试） | `chore: 更新 .gitignore 忽略规则` |
| `revert` | 恢复或回滚此前的某次历史提交 | `revert: feat(auth): 回退登录授权变动` |

---

## 3. Pull Request 流程

- **PR 标题规范**：PR 标题必须同样遵循 [Conventional Commits](#2-commit-提交信息规范) 格式（如 `feat: 新增能力` 或 `fix: 修复缺陷`），CI 会对其进行自动化合规校验；
- **模版填写**：发起 PR 时，请按模版完整填写变更背景、解决的问题以及关联的 Issue（如 `close #12`）；
- **CI 绿灯**：确保 CI 流水线测试全部处于通过状态；
- **审查与合并**：代码审查（Code Review）提出修改意见后，在原分支继续提交即可自动同步至 PR；合并后特性分支将被删除。

---

## 4. 版本发版机制与发布说明

本项目通过 GitHub Actions 实现了自动化发版体系。正式发版标准流程如下：

### 1. 日常提交与日志归纳
平时向 `main` 分支提交代码或合并 PR 时，规范的提交记录（Conventional Commits）会被 `git-cliff` 自动追踪，并在发版时聚合生成更新日志。

### 2. 同步项目版本号（前置可选）
发版前若涉及项目自身版本号递增，**严禁使用文本正则全局替换**（极易误伤同名第三方依赖版本）。推荐采用各生态官方原生命令或单一真实信源（SSOT）架构：

| 技术栈 / 生态 | 推荐方式 / 原生命令 | 说明 |
| :--- | :--- | :--- |
| **Java (Maven)** | `mvn versions:set -DnewVersion=1.2.0 -DgenerateBackupPoms=false` | 递归同步父工程与所有子模块 `pom.xml`，零误伤第三方依赖 |
| **Java (Maven 推荐)** | 父 POM 配置 `<properties><revision>1.2.0</revision></properties>` | 官方 CI-Friendly 规范，全工程仅需在父 POM 维护这 1 行 |
| **Java (Gradle)** | 在 `gradle.properties` 中维护单一变量 `version=1.2.0` | 单一真实信源，多子模块自动继承 |
| **Node.js (npm / pnpm)** | `npm version 1.2.0` 或 `pnpm version 1.2.0` | 自动同步 `package.json` 与对应 lockfile（默认自动建 Tag） |
| **前端通用 (跨包管理器)** | `npx bumpp` | 自动探测 npm/yarn/pnpm，交互式选择版本并同步 lockfile |
| **Python** | `poetry version patch` (或使用 `bump-my-version`) | 自动安全递增 `pyproject.toml` 中的版本号 |
| **Go 语言** | 无需操作 | Go 模块原生完全基于 Git Tag，源码零版本配置文件 |
| **Rust** | `cargo set-version 1.2.0` (或使用 `cargo-release`) | 官方子命令，安全递增 `Cargo.toml` 及其工作区版本 |

> 💡 *若使用原生命令（非 `npm version` / `bumpp`）修改了版本文件，请先在本地提交：`git commit -am "chore(release): v1.2.0"`*

### 3. 打标签并推送到远端（触发发版）
当本地代码与版本准备就绪后，推送标签至 GitHub 即可触发发版流水线：

```bash
# 步骤 A：确保本地最新代码已推送到 main 分支
git push origin main

# 步骤 B：打版本标签并推送到 GitHub (支持 v1.0.0, v1.0.0-beta.1 等)
git tag v1.0.0
git push origin v1.0.0

# 或使用联合命令一键推送分支与标签
# git push origin main --tags
```

### 4. 自动化流水线运行
标签推送后，GitHub Actions 将会自动执行 [`.github/workflows/release.yml`](./.github/workflows/release.yml)：
- 自动提取自上一版本以来的全部提交与 PR，由 `git-cliff` 格式化为发布日志；
- 自动创建 GitHub Release 并挂载发布内容；
- 将打包产物与 `checksums.txt` 挂载至附件（若配置了构建步骤）；
- 分发至官方中心仓库或平台（若配置了对应发布 Job）。

> 💡 **安全校验和 (SHA-256 Checksums) 验证指引**：
> 下游用户或测试者下载产物与 `checksums.txt` 后，可在终端通过原生命令一键验证文件完整性：
> - **Linux**：`sha256sum -c checksums.txt --ignore-missing`
> - **macOS**：`shasum -a 256 -c checksums.txt`
> - **Windows (PowerShell)**：`Get-FileHash .\your-asset-file.tar.gz -Algorithm SHA256`

### 中心仓库发布凭据 (Secrets) 参考

如需发布至官方包管理仓库，请在仓库的 **Settings -> Secrets and variables -> Actions** 中配置对应凭证：

- **npm**: 推荐配置 [npm Trusted Publishing (OIDC)](https://docs.npmjs.com/trusted-publishers)，或配置 `NPM_TOKEN`；
- **Maven Central**: 配置 `OSSRH_USERNAME`、`OSSRH_TOKEN`、`MAVEN_GPG_PRIVATE_KEY`、`MAVEN_GPG_PASSPHRASE`；
- **PyPI**: 推荐使用官方 [PyPI Trusted Publisher (OIDC)](https://docs.pypi.org/trusted-publishers/)，免配静态密钥；
- **Docker 镜像**:
  - **GHCR (推荐)**: 默认直接使用系统内置 `GITHUB_TOKEN`，零 Secret 配置；
  - **Docker Hub**: 配置 `DOCKERHUB_USERNAME` 与 `DOCKERHUB_TOKEN`；
- **Go 二进制工具 (GoReleaser)**: 默认直接使用系统内置 `GITHUB_TOKEN` 上传附件至 Release，零 Secret 配置；
- **GitHub Pages (前端/文档)**: 默认使用官方工作流部署，仅需在仓库 **Settings -> Pages -> Build and deployment -> Source** 切换为 **GitHub Actions** 即可。

