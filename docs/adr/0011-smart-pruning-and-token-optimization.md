# 0011. Kubernetes 原始数据智能降噪 (Smart Pruning) 与 Token 优化

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

Kubernetes API 原生响应包含海量系统管理元数据（如 `metadata.managedFields`、`kubectl.kubernetes.io/last-applied-configuration` 注解、复杂的时间戳转换矩阵等）。一个 Pod 的原始 JSON 往往高达 300~500 行，其中 70% 以上是对 AI 诊断排障无价值的噪音数据，会迅速消耗上下文窗口并大幅增加模型推理延迟与费用。

## 架构决策

在服务端向客户端序列化输出前，默认强制开启智能降噪修剪管道（Smart Pruning Pipeline）：
1. **剥离所有权元数据**：剔除整个 `metadata.managedFields` 字段；
2. **清洗超大历史注解**：剔除 `kubectl.kubernetes.io/last-applied-configuration`；
3. **清洗空数据节点**：递归清洗全 `null`、空列表 `[]` 与空对象 `{}` 字段；
4. **保留诊断核心**：完整保留 `status.phase`、`status.conditions`、`status.containerStatuses`、事件原因与退出码。

## 架构后果

- **正面收益**：单次资源输出体积压缩 70%~80%，显著降低 Token 消耗，模型对关键状态指标的注意力聚集度大幅提升。
- **潜在代价**：若特定用户需要调试服务端应用控制器字段所有权（Server-Side Apply 冲突），需通过 `--raw` 显式参数绕过修剪。
