# 0016. 企业级高频核心资源原生路由与全量防御性入参净化

- **状态**: accepted
- **日期**: 2026-10-07

## 背景与上下文

在 Kubernetes 生产排障场景中，Agent 的诊断需求远不止于 Pod 与 Deployment：
1. **常用资源受限**：先前 `k8s_list_resources` 与 `k8s_get_resource` 仅内置支持 8 种基础资源。当排查有状态应用（`StatefulSet`）、守护进程（`DaemonSet`）、一次性或定时任务（`Job`、`CronJob`）、路由网关（`Ingress`）及存储卷声明（`PVC`、`PV`）时，工具直接报错不支持，迫使 Agent 退回调用必须明确填写 GVR 三元组的 `k8s_list_custom_resources` 动态反射工具，增加了交互轮次与思考负担；
2. **LLM 传参格式抖动**：大语言模型在生成 JSON 工具入参时，受 Token 分词与格式生成概率波动影响，偶发附带前后空格或换行符（如 `"default "` 或 `"pod\n"`），若直接透传至 API 会导致偶发性 404 NotFound 校验失败；
3. **连接池设计取舍**：在常驻 SSE 运行模式下，需要权衡多集群 Context 连接池是否需要引入复杂的 LRU 淘汰机制。

## 架构决策

1. **核心资源原生路由扩充 (Core Resource Dispatch)**：
   - 在 `canonicalKind`、`queryResourceList` 与 `querySingleResource` 中将原生强类型支持扩充至 **15 类常用核心资源**：
     - 工作负载：`Deployment`, `StatefulSet`, `DaemonSet`
     - 批处理：`Job`, `CronJob`
     - 计算与服务：`Pod`, `Service`
     - 网络路由：`Ingress`
     - 存储卷：`PersistentVolumeClaim`, `PersistentVolume`
     - 配置与安全：`ConfigMap`, `Secret`（强制掩码脱敏）
     - 集群元数据与事件：`Node`, `Namespace`, `Event`
   - 支持主流缩写别名（如 `sts`, `ds`, `cj`, `ing`, `pvc`, `pv` 等），并在返回时自动注入标准的 `Kind` 与 `APIVersion` TypeMeta；
   - `k8s_describe_resource` 针对上述工作负载自动关联排障事件视图。
2. **全量 Tool Handler 输入参数防御性清洗 (Input Sanitization)**：
   - 在所有工具入口（诊断、拓扑树、RBAC 探针、变更操作、容器 Exec 及动态 CRD）对关键字符串参数统一前置执行 `strings.TrimSpace` 净化，消除首尾空白与换行隐患；
   - 遵循稳健原则（Postel's Law），提升对 LLM 偶发抖动的容错防御能力。
3. **连接池保持克制与无状态设计**：
   - 多 Context 客户端连接池保留轻量、无界且并发安全的 `sync.RWMutex` + `map[string]...` 实现；
   - 鉴于本地与集群 kubeconfig 上下文数量通常有限（< 20），杜绝引入外部 LRU 淘汰与频繁连接重构的过度设计。
4. **集群内部署清单权限完全对齐**：
   - 同步在 `deploy/kubernetes-sse.yaml` 的 `ClusterRole` 规则中增补 `networking.k8s.io` 下 `ingresses` 资源的 `get`、`list`、`watch` 权限，保障开箱即用。

## 架构后果

- **正面收益**：
  - 核心排障体验更加平滑自然，常用资源无需繁琐查询 GVR 即可直查；
  - 杜绝因格式空格引发的假性报错；
  - 集群清单与代码能力严格保持一致。
- **潜在代价**：
  - 核心资源列表与别名映射需由代码集中维护（对于非通用标准资源与第三方 CRD 仍需使用动态客户端反射）。
