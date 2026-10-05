# 0007. 基于 Dynamic Client 的自定义资源 (CRD) 与通用反射支持

- **状态**: accepted
- **日期**: 2026-10-06

## 背景与上下文

现代云原生集群严重依赖各种自定义资源定义（CRD，例如 PrometheusRule、VirtualService、Application 等）。固定静态编译的强类型结构体无法覆盖用户安装的第三方未知扩展资源。

## 架构决策

采用“核心内置强类型 + Dynamic Client 动态反射兜底”的双模方案：
1. **核心内置资源**：通过 `client-go` 的静态 ClientSet 解析 Pod、Deployment、Service 等核心对象，保证极速序列化与关键字段提取；
2. **CRD 动态反射**：集成 `k8s.io/client-go/dynamic` 与 `k8s.io/client-go/discovery`，暴露 `k8s_get_custom_resource` 与 `k8s_list_custom_resources` 工具，接收 Group/Version/Resource/Kind 参数，实现对任意 CRD 的免代码改动即时查询。

## 架构后果

- **正面收益**：对 Istio、ArgoCD、Cert-Manager、Kubeflow 等全生态 CRD 具备通用适配能力。
- **潜在代价**：非结构化对象（`unstructured.Unstructured`）输出需要自行清洗与序列化，避免冗余空字段消耗 Token。
