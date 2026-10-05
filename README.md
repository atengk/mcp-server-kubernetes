# MCP Server Kubernetes

<p align="center">
  <strong>Model Context Protocol (MCP) Server for Kubernetes</strong>
</p>

<p align="center">
  <a href="https://github.com/atengk/mcp-server-kubernetes/actions/workflows/ci.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/atengk/mcp-server-kubernetes/ci.yml?branch=main&label=CI&style=flat-square" alt="CI Status" />
  </a>
  <a href="https://github.com/atengk/mcp-server-kubernetes/releases">
    <img src="https://img.shields.io/github/v/release/atengk/mcp-server-kubernetes?style=flat-square" alt="Release" />
  </a>
  <a href="./LICENSE">
    <img src="https://img.shields.io/badge/License-Apache_2.0-blue.svg?style=flat-square" alt="License" />
  </a>
  <a href="./CONTRIBUTING.md">
    <img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=flat-square" alt="PRs Welcome" />
  </a>
</p>

---

## 📖 项目简介

`mcp-server-kubernetes` 是基于模型上下文协议（Model Context Protocol, MCP）构建的 Kubernetes 交互服务底座。

---

## 📂 仓库目录结构

```text
.
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md           # Bug 缺陷反馈模版
│   │   └── feature_request.md      # 新特性建议模版
│   ├── workflows/
│   │   ├── ci.yml                  # 自动化持续集成流水线
│   │   └── release.yml             # 自动化版本发版与分发流水线
│   └── PULL_REQUEST_TEMPLATE.md    # Pull Request 提交模版
├── .cliff.toml                     # git-cliff 变更日志提取与分类配置
├── .dockerignore                   # Docker 构建上下文忽略配置
├── .editorconfig                   # 跨编辑器编码规范
├── .gitattributes                  # 换行符与文件属性配置 (强制 LF)
├── .gitignore                      # 通用版本控制忽略配置
├── CONTRIBUTING.md                 # 贡献指南与 Commit 提交规范
├── LICENSE                         # 开源许可证 (Apache-2.0)
└── README.md                       # 项目主文档
```

---

## 🤝 参与贡献

欢迎参与贡献！请在提交代码前仔细阅读我们的 [贡献指南](./CONTRIBUTING.md)。

---

## 📄 开源许可证

本项目基于 [Apache License 2.0](./LICENSE) 协议开源。
