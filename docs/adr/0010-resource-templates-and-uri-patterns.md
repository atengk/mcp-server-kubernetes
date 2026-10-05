# 0010. 动态参数化资源模板 (Resource Templates) 与 URI 映射

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

静态资源 URI（如 `k8s://namespaces`）仅能提供枚举级元数据。AI 客户端用户通常希望在发起对话时直接引用特定的特定 Pod 或 Deployment 上下文，而无需触发额外工具查询。

## 架构决策

基于 RFC 6570 规范实现 MCP 参数化资源模板（Resource Templates）：
- `k8s://{namespace}/pods/{name}`：动态挂载指定命名空间下 Pod 的实时运行状态；
- `k8s://{namespace}/deployments/{name}`：动态挂载 Deployment 配置与副本就绪状态；
- `k8s://{namespace}/logs/{pod}`：挂载容器尾部最新排障日志切片。

## 架构后果

- **正面收益**：用户可直接在 IDE 对话框通过 `@k8s://default/pods/nginx-xxx` 原生挂载目标资源作为对话提示词背景，减少多余调用往返。
- **潜在代价**：需在服务端维护 URI 模式路由匹配器与参数合法性校验层。
