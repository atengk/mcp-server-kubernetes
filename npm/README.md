# @atengk/mcp-server-kubernetes

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![npm version](https://img.shields.io/npm/v/@atengk/mcp-server-kubernetes.svg)](https://www.npmjs.com/package/@atengk/mcp-server-kubernetes)

Kubernetes 原生 Model Context Protocol (MCP) 服务端 Node.js 跨平台分发包。

本项目为由 Go 语言编写的高性能原生二进制程序提供零依赖跨平台包装层，利用 npm 的 `optionalDependencies` 矩阵机制自动匹配当前操作系统平台与芯片架构并分发执行，无需用户在本地安装 Go 环境。

---

## 支持平台矩阵

本包装层自动识别并支持以下 5 大主流操作系统与芯片架构：

- `darwin-arm64` (macOS Apple Silicon M 系列芯片)
- `darwin-x64` (macOS Intel 芯片)
- `linux-x64` (Linux 64 位 x86_64)
- `linux-arm64` (Linux 64 位 ARM aarch64)
- `win32-x64` (Windows 64 位 x86_64)

---

## 快速上手

### 1. 使用 `npx` 即时启动

无需全局安装，直接通过 `npx` 启动 stdio 通信模式：

```bash
npx @atengk/mcp-server-kubernetes
```

### 2. 传递启动参数

所有传递给 CLI 的参数将原样透明转发至底层原生程序：

```bash
# 查看帮助文档
npx @atengk/mcp-server-kubernetes --help

# 以 SSE HTTP 服务模式运行在 8080 端口
npx @atengk/mcp-server-kubernetes --transport sse --port 8080

# 显式指定 kubeconfig 路径与激活 context
npx @atengk/mcp-server-kubernetes --kubeconfig ~/.kube/config --context dev-cluster

# 显式开启写操作变更与容器 Exec 门禁
npx @atengk/mcp-server-kubernetes --allow-write --allow-exec
```

---

## 原生支持的核心资源与常用别名

无需繁琐填写 GVR，即可直接使用全称或缩写别名进行 List / Get / Describe 排障查询：

- **工作负载**：`Deployment` (`deploy`), `StatefulSet` (`sts`), `DaemonSet` (`ds`)
- **批处理**：`Job` (`job`), `CronJob` (`cj`)
- **核心计算与网络**：`Pod` (`po`), `Service` (`svc`), `Ingress` (`ing`)
- **持久存储**：`PersistentVolumeClaim` (`pvc`), `PersistentVolume` (`pv`)
- **配置与安全**：`ConfigMap` (`cm`), `Secret` (强制掩码脱敏)
- **集群元数据**：`Node` (`no`), `Namespace` (`ns`), `Event` (`ev`)
- **第三方扩展**：所有自定义资源均可通过 `k8s_list_custom_resources` 动态反射支持。

---

## 通用客户端接入与多环境配置

各大主流 MCP 客户端（Claude Desktop、Cursor、VS Code Cline/Roo-Code、Windsurf 等）均遵从标准的 `mcpServers` JSON 配置规范：

### 1. 单环境通用配置模版

```json
{
  "mcpServers": {
    "kubernetes": {
      "command": "npx",
      "args": [
        "-y",
        "@atengk/mcp-server-kubernetes",
        "--kubeconfig", "/path/to/custom-kubeconfig.yaml",
        "--context", "my-cluster-context",
        "--allow-write"
      ]
    }
  }
}
```

### 2. 多环境物理隔离最佳实践 (开发可写 + 生产只读)

```json
{
  "mcpServers": {
    "k8s-dev": {
      "command": "npx",
      "args": [
        "-y",
        "@atengk/mcp-server-kubernetes",
        "--kubeconfig", "/path/to/dev.yaml",
        "--context", "dev-cluster",
        "--allow-write",
        "--allow-exec"
      ]
    },
    "k8s-prod": {
      "command": "npx",
      "args": [
        "-y",
        "@atengk/mcp-server-kubernetes",
        "--kubeconfig", "/path/to/prod.yaml",
        "--context", "prod-cluster"
      ]
    }
  }
}
```

---

## 环境变量覆盖

在特殊网络环境、离线环境或容器内部，若未能从 npm 下载对应平台的子包，可通过设置环境变量显式指定本地原生二进制文件绝对路径：

```bash
export MCP_SERVER_KUBERNETES_BINARY_PATH=/custom/path/to/mcp-server-kubernetes
npx @atengk/mcp-server-kubernetes
```

---

## 常见排障 (Troubleshooting)

- **提示未找到平台专属子包**：
  若安装时添加了 `--no-optional` 参数，npm 将跳过平台专有子包的安装。请移除该参数或检查 npm 源中是否存在对应子包；
- **权限问题**：
  在 Linux 或 macOS 系统下，确保当前用户对解压出的可执行文件具有执行权限（`chmod +x`）。

---

## 开源协议

本项目遵循 [Apache-2.0](../LICENSE) 开源协议。
