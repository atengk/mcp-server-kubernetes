# MCP Server Kubernetes

<p align="center">
  <strong>Model Context Protocol (MCP) Server for Kubernetes</strong><br>
  企业级云原生智能运维与排障服务底座（Go 内核 + 跨平台 npx 零配置即启）
</p>

<p align="center">
  <a href="https://github.com/atengk/mcp-server-kubernetes/actions/workflows/ci.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/atengk/mcp-server-kubernetes/ci.yml?branch=main&label=CI&style=flat-square" alt="CI Status" />
  </a>
  <a href="https://github.com/atengk/mcp-server-kubernetes/releases">
    <img src="https://img.shields.io/github/v/release/atengk/mcp-server-kubernetes?style=flat-square" alt="Release" />
  </a>
  <a href="https://www.npmjs.com/package/@atengk/mcp-server-kubernetes">
    <img src="https://img.shields.io/npm/v/@atengk/mcp-server-kubernetes?style=flat-square&color=cb3837&logo=npm" alt="npm version" />
  </a>
  <a href="https://github.com/atengk/mcp-server-kubernetes/pkgs/container/mcp-server-kubernetes">
    <img src="https://img.shields.io/badge/GHCR-Docker_Image-2496ed?style=flat-square&logo=docker&logoColor=white" alt="Docker Image" />
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

`mcp-server-kubernetes` 是基于模型上下文协议（Model Context Protocol, MCP）构建的 Kubernetes 集群交互与运维服务底座。

项目采用**双层混合分发架构**：核心引擎采用 **Go 语言（client-go + mcp-go）** 编译为原生机器码，保障卓越的并发性能与对 Kubernetes 生态的 100% 原生契约支持；外部通过 **Node.js Wrapper** 打包发布至 npm 注册表，使得 AI 开发者能够直接通过 `npx` 零依赖、零编译环境秒级启动。

---

## ✨ 核心特性

- ⚡ **原生高性能 & npx 零依赖分发**：Go 语言原生编译，通过 npm `optionalDependencies` 平台专属子包按需拉取，支持 macOS (Apple Silicon / Intel)、Linux (x64 / ARM64)、Windows (x64)。
- 🧩 **MCP 三大原语全支持**：
  - **Resources**：支持 `k8s://` 静态全景快照与参数化资源模板（`k8s://{namespace}/pods/{name}`），支持将工作负载实时状态即挂即用；
  - **Prompts**：预置专家级运维排障提示词（Pod 崩溃根因分析、集群健康巡检、安全合规审计）；
  - **Tools**：覆盖全生命周期排障诊断、动态 CRD 反射、RBAC 鉴权自检与受控拓扑下钻。
- 🛡️ **企业级安全守卫 (Safety Guard)**：
  - **默认强制只读**：变更类工具（Apply/Scale/Delete）必须显式传入 `--allow-write` 启动参数才可注册；
  - **Secret 敏感数据脱敏**：读取机密时保留键名（Key）与元数据，所有数值强制掩码为 `[REDACTED]`；
  - **受控容器命令探针**：容器内部 Exec 命令需显式开启 `--allow-exec`，内置 15 秒超时熔断与缓冲区硬截断。
- 🧠 **大模型上下文深度优化 (Smart Pruning & Token Guard)**：
  - **智能降噪 (Smart Pruning)**：自动剥离 `managedFields`、历史冗余配置与空节点，节约 70% 以上 Token 消耗；
  - **Token 截断守卫 (Token Guard)**：日志与列表输出设定默认安全阈值，超量输出自动附带截断警示。
- 🔄 **变更前预检闭环 (Dry-Run Preview)**：变更操作支持 Server-Side Dry-Run，自动生成 Unified Diff 供人类审核后再确认生效。
- 🌐 **双通道通信支持**：默认 Stdio 管道接入本地客户端；支持 `--transport sse --port 8080` 切换常驻服务以 Pod 部署。

---

## ⚡ 客户端通用接入与多环境配置 (Client Configuration)

无需安装 Go 环境，确保宿主机已安装 Node.js (>= 18)，各大主流 AI 客户端（Claude Desktop、Cursor、VS Code Cline/Roo-Code、Windsurf、Zed 等）底层均采用标准的 `mcpServers` JSON 格式协议。

### 1. 通用客户端配置文件路径速查

| 客户端 / AI Agent | 配置文件路径 (macOS / Linux) | 配置文件路径 (Windows) |
| :--- | :--- | :--- |
| **Claude Desktop** | `~/Library/Application Support/Claude/claude_desktop_config.json` | `%APPDATA%\Claude\claude_desktop_config.json` |
| **Cursor** | 项目根目录 `.cursor/mcp.json` 或 **Settings -> Features -> MCP Servers** | 同左 |
| **VS Code (Cline / Roo-Code)** | `~/Library/Application Support/Code/User/globalStorage/.../mcp_settings.json` | `%APPDATA%\Code\User\globalStorage\...\mcp_settings.json` |
| **Windsurf** | `~/.codeium/windsurf/mcp_config.json` | `%USERPROFILE%\.codeium\windsurf\mcp_config.json` |

---

### 2. 标准通用配置模版 (单环境)

直接在上述配置文件中的 `mcpServers` 对象内添加以下内容。支持通过 `--kubeconfig` 指定自定义配置文件路径，通过 `--context` 绑定默认集群上下文：

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

> 💡 **参数与环境变量说明**：
> - 未指定 `--kubeconfig` 时，默认按顺位探测：集群内 In-Cluster ServiceAccount -> 环境变量 `KUBECONFIG` -> 宿主机 `~/.kube/config`；
> - 亦可通过环境变量传递：在 Server 对象中追加 `"env": { "KUBECONFIG": "/path/to/custom-kubeconfig.yaml" }`；
> - 写操作门禁：默认强制只读，按需追加 `--allow-write`（开启增删改）与 `--allow-exec`（开启容器 Exec 探针）。

---

### 3. 多环境 / 多集群配置最佳实践

针对多环境需求（如开发测试与生产集群），根据安全隔离要求提供两种典型拓扑范式：

#### 方案 A：多实例物理隔离与权限分级 (推荐最佳实践)

在配置中声明多个独立的服务实例，分别绑定各自的配置文件或 Context，并针对不同环境设置**差异化安全防线**（开发环境允许写操作与容器探针，生产环境强制保持只读以防 AI 误操作）：

```json
{
  "mcpServers": {
    "k8s-dev": {
      "command": "npx",
      "args": [
        "-y",
        "@atengk/mcp-server-kubernetes",
        "--kubeconfig", "/path/to/dev-kubeconfig.yaml",
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
        "--kubeconfig", "/path/to/prod-kubeconfig.yaml",
        "--context", "prod-cluster"
      ]
    }
  }
}
```

#### 方案 B：单实例动态 Context 路由 (轻量模式)

若各集群的访问凭据已合并于同一个 `kubeconfig`（或默认配置包含多个上下文）：
只需配置一个通用 Server，AI Agent 可通过内置资源 `k8s://contexts` 感知所有可用集群，并在调用任意工具时动态传入 `context: "prod-cluster"` 参数，底层连接池会自动并发安全路由调度：

```json
{
  "mcpServers": {
    "kubernetes": {
      "command": "npx",
      "args": [
        "-y",
        "@atengk/mcp-server-kubernetes",
        "--kubeconfig", "/path/to/merged-kubeconfig.yaml"
      ]
    }
  }
}
```

---

### 4. Kubernetes 集群内常驻运行 (SSE 模式 / 生产级部署)

#### 方式 A：一键部署到 Kubernetes 集群 (推荐)

仓库已提供包含最小权限 ServiceAccount、只读 RBAC 与健康检测的开箱即用清单：

```bash
kubectl apply -f deploy/kubernetes-sse.yaml
```

服务将在 `mcp-system` 命名空间下启动，并通过 `mcp-server-kubernetes.mcp-system.svc:8080` 向集群内的其他服务提供 SSE 协议端点。

#### 方式 B：使用 Docker 单机启动 (多架构支持 linux/amd64, linux/arm64)

```bash
docker run -d --name mcp-k8s \
  -p 8080:8080 \
  -v ~/.kube/config:/root/.kube/config:ro \
  ghcr.io/atengk/mcp-server-kubernetes:latest \
  --transport sse --port 8080
```

---

## 🛠️ MCP 核心能力全景图表

### 1. 只读资源端点 (Resources & Resource Templates)

| 资源 URI / 模板 | 类型 | 说明与应用场景 |
| :--- | :--- | :--- |
| `k8s://contexts` | 静态资源 | 查看当前已配置的集群上下文列表与激活集群 |
| `k8s://cluster/overview` | 静态资源 | 集群版本、节点状态、总配额与核心指标快照 |
| `k8s://namespaces` | 静态资源 | 活跃命名空间及其运行状态 |
| `k8s://api-resources` | 静态资源 | 当前集群已注册的核心 API 与 CRD 清单 |
| `k8s://{namespace}/pods/{name}` | 资源模板 | 直接挂载指定 Pod 的运行状态作为会话背景 |
| `k8s://{namespace}/deployments/{name}` | 资源模板 | 直接挂载指定 Deployment 的配置与副本就绪度 |

### 2. 预置运维工作流模版 (Prompts)

- `diagnose-pod-failure`：传入 `namespace` 与 `pod_name`，自动分步调度 Describe、Events 与 Logs 生成根本原因诊断报告；
- `cluster-health-check`：全景巡检集群内未就绪节点、Pending 待调度 Pod、CrashLoop 容器与高危事件；
- `workload-security-audit`：巡检工作负载的特权容器配置、root 用户运行与资源限制配额风险。

### 3. 核心工具集 (Tools)

#### 基础诊断与查询（默认开启）
- `k8s_list_resources`：按命名空间或全集群查询资源列表（支持标签选择器过滤，内置 Token Guard 截断防护）；
- `k8s_get_resource`：获取指定资源详情（输出自动剥离 managedFields，Secret 自动脱敏）；
- `k8s_describe_resource`：获取类似 `kubectl describe` 的易读聚合诊断详情（包含关联排障事件）；
- `k8s_get_pod_logs`：获取容器日志（默认 tail 100 行，上限 1000 行，支持 `--previous`）；
- `k8s_get_events`：检索指定命名空间或关联对象的集群事件。

##### 原生支持核心资源与常用别名对照表 (免配置 GVR 直查)

| 资源类别 (Category) | 标准资源类型 (Kind) | 常用缩写与别名 (Aliases) | 作用域 (Scope) |
| :--- | :--- | :--- | :--- |
| **工作负载 (apps)** | `Deployment`, `StatefulSet`, `DaemonSet` | `deploy`, `sts`, `ds` | 命名空间 |
| **批处理任务 (batch)** | `Job`, `CronJob` | `job`, `cj` | 命名空间 |
| **计算与服务 (core)** | `Pod`, `Service` | `po`, `svc` | 命名空间 |
| **网络路由 (networking)**| `Ingress` | `ing` | 命名空间 |
| **持久存储 (core)** | `PersistentVolumeClaim`, `PersistentVolume` | `pvc`, `pv` | 命名空间 / 集群级 |
| **配置与凭据 (core)** | `ConfigMap`, `Secret` (强制脱敏) | `cm`, `secret` | 命名空间 |
| **集群元数据与事件** | `Node`, `Namespace`, `Event` | `no`, `ns`, `ev` | 集群级 / 命名空间 |

> 💡 **提示**：上述 15 类高频标准资源支持直接通过全称或短别名进行 List/Get/Describe 查询并自动补齐 TypeMeta；对于集群内注册的第三方自定义资源（CRD），请使用下方的 `k8s_list_custom_resources` / `k8s_get_custom_resource` 动态反射工具。

#### 高级排障与拓扑下钻（默认开启）
- `k8s_get_resource_tree`：基于所有权与选择器一键下钻 Deployment/Service 级联下属 Pod/RS 拓扑树；
- `k8s_auth_can_i`：RBAC 权限边界自检，评估当前主体或指定用户在命名空间内的执行权限；
- `k8s_get_custom_resource` / `k8s_list_custom_resources`：通用动态反射，支持查询任意第三方 CRD。

#### 容器探针与变更工具（需显式授权）
- `k8s_exec_command`：在指定容器中执行非交互式排障命令（**需 `--allow-exec`**，内置 15 秒超时与 64KB 缓冲区截断）；
- `k8s_diff_resource`：基于 Server-Side Dry-Run 计算变更增量 Unified Diff；
- `k8s_apply_manifest`：创建或声明式应用资源 YAML（**需 `--allow-write`**）；
- `k8s_scale_resource`：扩缩容工作负载副本数（**需 `--allow-write`**）；
- `k8s_delete_resource`：安全删除指定资源（**需 `--allow-write`**）。

---

## ⚙️ 命令行参数全集 (CLI Flags)

```text
--transport <stdio|sse>   通信通道模式 (默认: stdio)
--port <port>             SSE 模式监听端口 (默认: 8080)
--allow-write             激活资源创建、变更与删除工具 (默认: false, 强制只读)
--allow-exec              激活容器内部排障命令执行工具 (默认: false)
--kubeconfig <path>       显式指定 kubeconfig 配置文件路径
--context <name>          指定默认连接的集群上下文名称
--help, -h                显示帮助信息
--version, -v             显示当前版本信息
```

---

## 📂 仓库目录结构

```text
.
├── cmd/
│   └── mcp-server-kubernetes/      # Go 应用程序入口 (main.go)
├── internal/
│   ├── config/                     # 命令行参数与环境配置解析
│   ├── crd/                        # dynamic 客户端与 CRD 动态反射
│   ├── k8s/                        # client-go 连接池与多 Context 路由
│   ├── prompts/                    # 专家级运维诊断提示词原语
│   ├── pruning/                    # Smart Pruning 智能降噪清洗管道与 Secret 脱敏
│   ├── resources/                  # k8s:// 静态资源与 RFC 6570 参数化模板原语
│   ├── safety/                     # Safety Guard 门禁、Token 截断与 Dry-Run Diff
│   ├── server/                     # mark3labs/mcp-go 双通道生命周期管理
│   ├── tools/                      # 核心诊断、拓扑下钻、RBAC 自检、变更与 Exec 工具
│   └── version/                    # 构建期动态注入的版本元数据 (Tag SSOT)
├── npm/
│   ├── bin/                        # npx CLI 调度脚本 (cli.js)
│   ├── lib/                        # 跨平台架构探测解析器与子包组装器
│   ├── scripts/                    # 平台子包构建与动态版本同步脚本
│   └── package.json                # npm 主包配置 (@atengk/mcp-server-kubernetes)
├── deploy/
│   └── kubernetes-sse.yaml         # 企业级集群内常驻运行声明式清单 (RBAC + Deployment)
├── scripts/
│   └── build-cross-platforms.sh    # 5 大主流平台架构交叉编译脚本
├── docs/
│   ├── adr/                        # 架构决策记录 (0001 ~ 0016)
│   └── agents/                     # Agent 协作规范 (issue-tracker, triage, domain)
├── Dockerfile                      # 生产级多阶段非 root 容器镜像定义
├── .cliff.toml                     # 自动化版本日志提取规则
├── .github/workflows/              # GitHub Actions CI 与全自动 Release 流水线
├── AGENTS.md                       # AI Agent 协作与工程规范
├── CONTEXT.md                      # 领域核心模型与词汇表
├── CONTRIBUTING.md                 # 贡献指南与 Commit 规范
├── LICENSE                         # 开源许可证 (Apache-2.0)
└── README.md                       # 本文档
```

---

## 🤝 参与贡献

欢迎任何形式的贡献与建议！请在提交代码前仔细阅读我们的 [贡献指南](./CONTRIBUTING.md) 与 [架构决策记录](./docs/adr/)。

---

## 📄 开源许可证

本项目基于 [Apache License 2.0](./LICENSE) 协议开源。
