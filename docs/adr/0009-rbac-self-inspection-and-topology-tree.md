# 0009. RBAC 权限自检探针与级联资源拓扑树下钻

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

在 Kubernetes 运维中，权限拒绝（403 Forbidden）与多层级资源级联故障（如 Deployment -> ReplicaSet -> Pod 调度失败，或 Ingress -> Service -> Endpoints 断流）最为常见。单体资源查询迫使 AI 反复发起大量独立请求。

## 架构决策

1. **RBAC 鉴权自检工具 (`k8s_auth_can_i`)**：
   - 基于 Kubernetes 原生 `SelfSubjectAccessReview` 与 `SubjectAccessReview` API；
   - 允许 AI 快速验证自身或指定 ServiceAccount 是否具备针对特定命名空间/资源的指定 Verb 权限。
2. **资源所有权拓扑树 (`k8s_get_resource_tree`)**：
   - 基于 `metadata.ownerReferences` 与 `spec.selector` 算法；
   - 传入根资源（如 Deployment / Service），服务端自动级联抓取下属所有关联对象及其健康就绪状态，聚合为单棵树状数据结构输出。

## 架构后果

- **正面收益**：将多轮往返查询压缩为单次高效下钻，显著减少 Token 损耗并加快故障定界速度。
- **潜在代价**：拓扑树解析在大型集群中需要执行并发请求聚合，需合理使用并发池防抖。
