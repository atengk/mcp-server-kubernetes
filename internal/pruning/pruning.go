// Package pruning 提供面向 Kubernetes 资源的 Smart Pruning 智能降噪清洗管道与 Secret 敏感数据脱敏守卫。
//
// @author Ateng
// @since 2026-10-06
package pruning

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	// RedactedValue 声明机密数据脱敏后的统一掩码标识
	RedactedValue = "[REDACTED]"

	// LastAppliedConfigAnnotation 声明 kubectl 注入的超大历史配置注解键名
	LastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

	// ManagedFieldsKey 声明 Kubernetes 系统管理元数据字段键名
	ManagedFieldsKey = "managedFields"

	// MetadataKey 声明 Kubernetes 元数据根键名
	MetadataKey = "metadata"

	// AnnotationsKey 声明 Kubernetes 注解字典键名
	AnnotationsKey = "annotations"

	// KindKey 声明 Kubernetes 资源类型键名
	KindKey = "kind"

	// ItemsKey 声明 Kubernetes 资源列表子项键名
	ItemsKey = "items"

	// SecretKind 声明 Kubernetes 机密资源类型标识
	SecretKind = "Secret"
)

// Prune 对任意 Kubernetes 资源对象执行智能降噪清洗管道（剥离 managedFields、删除历史注解、脱敏 Secret、递归清理空节点）。
//
// @param obj 待清洗的资源对象（支持 map[string]any、[]any、*unstructured.Unstructured 或任意结构体）
// @return any 清洗后的精简对象
// @return error 处理过程中的序列化或转换异常
func Prune(obj any) (any, error) {
	if obj == nil {
		return make(map[string]any), nil
	}

	// 统一转为泛型 map/slice 结构以执行通用修剪
	var genericObj any
	switch v := obj.(type) {
	case *unstructured.Unstructured:
		if v == nil {
			return make(map[string]any), nil
		}
		genericObj = deepCopyMap(v.Object)
	case unstructured.Unstructured:
		genericObj = deepCopyMap(v.Object)
	case map[string]any:
		genericObj = deepCopyMap(v)
	case []any:
		genericObj = deepCopySlice(v)
	default:
		data, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("对象序列化失败: %w", err)
		}
		if err := json.Unmarshal(data, &genericObj); err != nil {
			return nil, fmt.Errorf("对象反序列化失败: %w", err)
		}
	}

	switch v := genericObj.(type) {
	case map[string]any:
		return pruneMap(v), nil
	case []any:
		return pruneSlice(v), nil
	default:
		return v, nil
	}
}

// PruneUnstructured 针对 Kubernetes Unstructured 动态对象执行智能降噪与脱敏。
//
// @param u 待清洗的 Unstructured 资源对象指针
// @return *unstructured.Unstructured 清洗后的 Unstructured 新对象
// @return error 转换处理异常
func PruneUnstructured(u *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if u == nil {
		return &unstructured.Unstructured{Object: make(map[string]any)}, nil
	}

	pruned, err := Prune(u.Object)
	if err != nil {
		return nil, err
	}

	prunedMap, ok := pruned.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("修剪结果非 map 结构: %T", pruned)
	}

	return &unstructured.Unstructured{Object: prunedMap}, nil
}

// PruneJSON 对 JSON 字节切片执行智能清洗，并重新序列化为清洗后的 JSON 数据。
//
// @param jsonData 原始 JSON 字节数组
// @return []byte 降噪后的 JSON 字节数组（空数据返回空切片）
// @return error JSON 解析或序列化失败时的错误
func PruneJSON(jsonData []byte) ([]byte, error) {
	if len(jsonData) == 0 {
		return []byte{}, nil
	}

	var obj any
	if err := json.Unmarshal(jsonData, &obj); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	pruned, err := Prune(obj)
	if err != nil {
		return nil, err
	}

	out, err := json.Marshal(pruned)
	if err != nil {
		return nil, fmt.Errorf("序列化清洗后的 JSON 失败: %w", err)
	}

	return out, nil
}

// PruneYAML 对 YAML 字节切片执行智能清洗，并重新序列化为清洗后的 YAML 数据。
//
// @param yamlData 原始 YAML 字节数组
// @return []byte 降噪后的 YAML 字节数组（空数据返回空切片）
// @return error YAML 解析或序列化失败时的错误
func PruneYAML(yamlData []byte) ([]byte, error) {
	if len(yamlData) == 0 {
		return []byte{}, nil
	}

	var obj any
	if err := yaml.Unmarshal(yamlData, &obj); err != nil {
		return nil, fmt.Errorf("解析 YAML 失败: %w", err)
	}

	pruned, err := Prune(obj)
	if err != nil {
		return nil, err
	}

	out, err := yaml.Marshal(pruned)
	if err != nil {
		return nil, fmt.Errorf("序列化清洗后的 YAML 失败: %w", err)
	}

	return out, nil
}

// pruneMap 针对单个资源 map 执行脱敏与降噪流程。
func pruneMap(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}

	// 1. 若当前资源为 Secret，对数据项强制执行掩码脱敏
	redactSecretIfNeeded(m)

	// 2. 剥离 metadata 下的噪音字段（managedFields 与 last-applied-configuration）
	stripMetadataNoise(m)

	// 3. 针对列表类资源（如 SecretList、PodList），深入穿透清洗 items 列表中的每个子资源
	if items, ok := m[ItemsKey].([]any); ok {
		for i, item := range items {
			if itemMap, ok := item.(map[string]any); ok {
				items[i] = pruneMap(itemMap)
			}
		}
	}

	// 4. 递归清洗所有 null、空列表与空字典
	cleaned := cleanMap(m)
	if cleaned == nil {
		return make(map[string]any)
	}
	return cleaned
}

// pruneSlice 对切片内的每个对象元素递归执行降噪。
func pruneSlice(slice []any) []any {
	if slice == nil {
		return []any{}
	}

	result := make([]any, 0, len(slice))
	for _, item := range slice {
		switch v := item.(type) {
		case map[string]any:
			cleaned := pruneMap(v)
			if len(cleaned) > 0 {
				result = append(result, cleaned)
			}
		default:
			cleaned := cleanValue(item)
			if cleaned != nil {
				result = append(result, cleaned)
			}
		}
	}
	return result
}

// redactSecretIfNeeded 针对 kind: Secret 资源执行字段值替换。
func redactSecretIfNeeded(m map[string]any) {
	kind, ok := m[KindKey].(string)
	if !ok || !strings.EqualFold(kind, SecretKind) {
		return
	}

	redactField(m, "data")
	redactField(m, "stringData")
}

// redactField 掩码替换指定键名的字典数据值，兼顾 map[string]any 与 map[string]string。
func redactField(m map[string]any, field string) {
	val, ok := m[field]
	if !ok || val == nil {
		return
	}

	switch v := val.(type) {
	case map[string]any:
		for key := range v {
			v[key] = RedactedValue
		}
	case map[string]string:
		redacted := make(map[string]any, len(v))
		for key := range v {
			redacted[key] = RedactedValue
		}
		m[field] = redacted
	}
}

// stripMetadataNoise 剔除 metadata 中的 managedFields 与历史注解。
func stripMetadataNoise(m map[string]any) {
	metadata, ok := m[MetadataKey].(map[string]any)
	if !ok || metadata == nil {
		return
	}

	delete(metadata, ManagedFieldsKey)

	if annotations, ok := metadata[AnnotationsKey].(map[string]any); ok && annotations != nil {
		delete(annotations, LastAppliedConfigAnnotation)
	}
}

// cleanMap 递归清洗 map 中的 null、空切片与空字典。
func cleanMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	res := make(map[string]any)
	for k, v := range m {
		cleaned := cleanValue(v)
		if cleaned != nil {
			res[k] = cleaned
		}
	}

	if len(res) == 0 {
		return nil
	}
	return res
}

// cleanValue 递归对任意数据值执行判空并剔除空结构。
func cleanValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		res := cleanMap(val)
		if len(res) == 0 {
			return nil
		}
		return res
	case []any:
		var res []any
		for _, item := range val {
			cleaned := cleanValue(item)
			if cleaned != nil {
				res = append(res, cleaned)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	default:
		return val
	}
}

// deepCopyMap 深度拷贝 map 数据，遵循空安全返回空字典。
func deepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return make(map[string]any)
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

// deepCopySlice 深度拷贝切片数据，遵循空安全返回空切片。
func deepCopySlice(src []any) []any {
	if src == nil {
		return []any{}
	}
	dst := make([]any, len(src))
	for i, v := range src {
		dst[i] = deepCopyValue(v)
	}
	return dst
}

// deepCopyValue 深度拷贝标量或复合值。
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		return deepCopySlice(val)
	default:
		return val
	}
}
