// Package pruning_test 针对 Smart Pruning 智能降噪管道与 Secret 脱敏守卫开展单测。
//
// @author Ateng
// @since 2026-10-06
package pruning_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/atengk/mcp-server-kubernetes/internal/pruning"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestPrune_ManagedFieldsAndLastApplied 验证剔除 managedFields 与 last-applied-configuration 注解。
func TestPrune_ManagedFieldsAndLastApplied(t *testing.T) {
	input := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "nginx-pod",
			"namespace": "default",
			"managedFields": []any{
				map[string]any{
					"manager":    "kube-controller-manager",
					"operation":  "Update",
					"apiVersion": "v1",
					"time":       "2026-10-06T08:00:00Z",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]any{
						"f:metadata": map[string]any{
							"f:labels": map[string]any{
								"f:app": map[string]any{},
							},
						},
					},
				},
			},
			"annotations": map[string]any{
				pruning.LastAppliedConfigAnnotation: `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"nginx-pod"}}`,
				"prometheus.io/scrape":               "true",
			},
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "nginx",
					"image": "nginx:1.27",
				},
			},
		},
	}

	result, err := pruning.Prune(input)
	if err != nil {
		t.Fatalf("Prune 失败: %v", err)
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("预期返回 map[string]any，实际为 %T", result)
	}

	metadata, ok := resMap["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 metadata 字段")
	}

	if _, exists := metadata["managedFields"]; exists {
		t.Errorf("managedFields 未被剔除")
	}

	annotations, ok := metadata["annotations"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 annotations 字段")
	}

	if _, exists := annotations[pruning.LastAppliedConfigAnnotation]; exists {
		t.Errorf("%s 注解未被剔除", pruning.LastAppliedConfigAnnotation)
	}

	if annotations["prometheus.io/scrape"] != "true" {
		t.Errorf("保留的有效注解丢失")
	}
}

// TestPrune_RecursiveEmptyCleanup 验证递归清理 null、空列表与空对象。
func TestPrune_RecursiveEmptyCleanup(t *testing.T) {
	input := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":        "my-config",
			"emptyMap":    map[string]any{},
			"emptySlice":  []any{},
			"nilField":    nil,
			"annotations": map[string]any{},
		},
		"data": map[string]any{
			"key1": "value1",
			"nested": map[string]any{
				"emptyChild": map[string]any{},
				"nilChild":   nil,
			},
		},
		"emptyOuter": map[string]any{
			"inner": map[string]any{
				"deepEmpty": []any{},
			},
		},
	}

	result, err := pruning.Prune(input)
	if err != nil {
		t.Fatalf("Prune 失败: %v", err)
	}

	resMap := result.(map[string]any)
	metadata := resMap["metadata"].(map[string]any)

	if _, exists := metadata["emptyMap"]; exists {
		t.Errorf("emptyMap 未被清理")
	}
	if _, exists := metadata["emptySlice"]; exists {
		t.Errorf("emptySlice 未被清理")
	}
	if _, exists := metadata["nilField"]; exists {
		t.Errorf("nilField 未被清理")
	}
	if _, exists := metadata["annotations"]; exists {
		t.Errorf("变空的 annotations 字典未被清理")
	}
	if _, exists := resMap["emptyOuter"]; exists {
		t.Errorf("级联全空的 emptyOuter 字典未被清理")
	}

	data := resMap["data"].(map[string]any)
	if data["key1"] != "value1" {
		t.Errorf("有效数据 key1 丢失")
	}
	if _, exists := data["nested"]; exists {
		t.Errorf("全部为空的 nested 字典未被清理")
	}
}

// TestPrune_SecretRedaction 验证对 kind: Secret 的强制脱敏。
func TestPrune_SecretRedaction(t *testing.T) {
	sensitiveData := "super-secret-password-12345"
	sensitiveToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

	input := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      "database-secret",
			"namespace": "production",
		},
		"type": "Opaque",
		"data": map[string]any{
			"password": sensitiveData,
			"token":    sensitiveToken,
		},
		"stringData": map[string]string{
			"rawPassword": "plaintext-password",
		},
	}

	result, err := pruning.Prune(input)
	if err != nil {
		t.Fatalf("Prune 失败: %v", err)
	}

	outBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	outStr := string(outBytes)

	// 断言绝不泄露敏感明文
	if strings.Contains(outStr, sensitiveData) {
		t.Fatalf("安全违规: 敏感密码 %q 依然存在于输出中", sensitiveData)
	}
	if strings.Contains(outStr, sensitiveToken) {
		t.Fatalf("安全违规: 敏感令牌 %q 依然存在于输出中", sensitiveToken)
	}
	if strings.Contains(outStr, "plaintext-password") {
		t.Fatalf("安全违规: stringData 敏感内容明文泄漏")
	}

	resMap := result.(map[string]any)
	data := resMap["data"].(map[string]any)
	if data["password"] != pruning.RedactedValue {
		t.Errorf("password 脱敏值预期为 %q，实际为 %v", pruning.RedactedValue, data["password"])
	}
	if data["token"] != pruning.RedactedValue {
		t.Errorf("token 脱敏值预期为 %q，实际为 %v", pruning.RedactedValue, data["token"])
	}

	stringData := resMap["stringData"].(map[string]any)
	if stringData["rawPassword"] != pruning.RedactedValue {
		t.Errorf("rawPassword 脱敏值预期为 %q，实际为 %v", pruning.RedactedValue, stringData["rawPassword"])
	}
}

// TestPrune_SecretList_Penetration 验证列表类型资源 (SecretList) 中子资源的穿透脱敏与降噪。
func TestPrune_SecretList_Penetration(t *testing.T) {
	secretList := map[string]any{
		"apiVersion": "v1",
		"kind":       "SecretList",
		"metadata": map[string]any{
			"resourceVersion": "12345",
		},
		"items": []any{
			map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]any{
					"name": "secret-1",
					"managedFields": []any{
						map[string]any{"manager": "kubectl"},
					},
				},
				"data": map[string]any{
					"api-key": "secret-token-abcdef",
				},
			},
			map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]any{
					"name": "secret-2",
				},
				"stringData": map[string]any{
					"db-pass": "topsecret123",
				},
			},
		},
	}

	result, err := pruning.Prune(secretList)
	if err != nil {
		t.Fatalf("Prune 失败: %v", err)
	}

	outBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	outStr := string(outBytes)

	if strings.Contains(outStr, "secret-token-abcdef") || strings.Contains(outStr, "topsecret123") {
		t.Fatalf("安全违规: SecretList items 子项未被穿透脱敏！输出: %s", outStr)
	}
	if strings.Contains(outStr, "managedFields") {
		t.Fatalf("规范违规: SecretList items 子项 managedFields 未被剥离！")
	}
}

// TestPruneUnstructured 验证对 Kubernetes Unstructured 动态对象类型的适配。
func TestPruneUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name": "web-deploy",
				"managedFields": []any{
					map[string]any{"manager": "helm"},
				},
			},
			"spec": map[string]any{
				"replicas": 3,
			},
		},
	}

	prunedU, err := pruning.PruneUnstructured(u)
	if err != nil {
		t.Fatalf("PruneUnstructured 失败: %v", err)
	}

	meta := prunedU.Object["metadata"].(map[string]any)
	if _, exists := meta["managedFields"]; exists {
		t.Errorf("Unstructured 对象的 managedFields 未被剔除")
	}
	if meta["name"] != "web-deploy" {
		t.Errorf("Unstructured 对象 name 字段丢失")
	}
}

// TestPrune_RealPodReductionOver70Percent 对比包含大篇幅 managedFields 与历史注解的真实 Pod 降噪与 Token 优化效果。
func TestPrune_RealPodReductionOver70Percent(t *testing.T) {
	largeManagedFields := make([]any, 0, 50)
	for i := 0; i < 50; i++ {
		largeManagedFields = append(largeManagedFields, map[string]any{
			"apiVersion": "v1",
			"fieldsType": "FieldsV1",
			"fieldsV1": map[string]any{
				"f:metadata": map[string]any{
					"f:annotations": map[string]any{
						"f:kubectl.kubernetes.io/restartedAt": map[string]any{},
					},
					"f:labels": map[string]any{
						"f:app":     map[string]any{},
						"f:version": map[string]any{},
						"f:release": map[string]any{},
					},
				},
				"f:spec": map[string]any{
					"f:containers": map[string]any{
						"k:{\"name\":\"app\"}": map[string]any{
							"f:image":           map[string]any{},
							"f:imagePullPolicy": map[string]any{},
							"f:resources":       map[string]any{},
						},
					},
				},
			},
			"manager":   "kube-controller-manager",
			"operation": "Update",
			"time":      "2026-10-06T08:00:00Z",
		})
	}

	rawPod := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":          "payment-service-75cf597956-abcde",
			"namespace":     "production",
			"managedFields": largeManagedFields,
			"annotations": map[string]any{
				pruning.LastAppliedConfigAnnotation: strings.Repeat(`{"kind":"Pod","metadata":{"annotations":{"app":"payment"}}}`, 30),
				"deployment.kubernetes.io/revision": "4",
			},
			"labels": map[string]any{
				"app": "payment",
			},
			"emptyStatus": map[string]any{},
			"nilField":    nil,
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "payment",
					"image": "registry.example.com/payment:v2.1.0",
					"ports": []any{
						map[string]any{
							"containerPort": 8080,
						},
					},
					"emptyConfig": map[string]any{},
				},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	}

	origBytes, err := json.Marshal(rawPod)
	if err != nil {
		t.Fatalf("序列化原始 Pod 失败: %v", err)
	}

	pruned, err := pruning.Prune(rawPod)
	if err != nil {
		t.Fatalf("Prune 失败: %v", err)
	}

	prunedBytes, err := json.Marshal(pruned)
	if err != nil {
		t.Fatalf("序列化修剪 Pod 失败: %v", err)
	}

	origLen := len(origBytes)
	prunedLen := len(prunedBytes)
	reductionRatio := float64(origLen-prunedLen) / float64(origLen)

	// 基于业内标准的字符/Token 经验换算比率（约 4 字符/Token）估算 Token 消耗
	origTokens := (origLen + 3) / 4
	prunedTokens := (prunedLen + 3) / 4
	tokenSavingRatio := float64(origTokens-prunedTokens) / float64(origTokens)

	t.Logf("原始体积: %d 字节 (约 %d Tokens), 修剪后体积: %d 字节 (约 %d Tokens), 降噪压缩率: %.2f%%, Token 节约率: %.2f%%",
		origLen, origTokens, prunedLen, prunedTokens, reductionRatio*100, tokenSavingRatio*100)

	if reductionRatio < 0.70 || tokenSavingRatio < 0.70 {
		t.Errorf("降噪/Token 节约率低于 70%%: 体积比 %.2f%%, Token 比 %.2f%%",
			reductionRatio*100, tokenSavingRatio*100)
	}
}

// TestPruneJSON_And_PruneYAML 验证便捷的 JSON 与 YAML 序列化管道封装。
func TestPruneJSON_And_PruneYAML(t *testing.T) {
	jsonStr := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"web","managedFields":[{"manager":"kubectl"}]}}`
	outJSON, err := pruning.PruneJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("PruneJSON 失败: %v", err)
	}
	if strings.Contains(string(outJSON), "managedFields") {
		t.Errorf("PruneJSON 输出未成功剔除 managedFields")
	}

	yamlStr := `
apiVersion: v1
kind: Secret
metadata:
  name: test-sec
data:
  token: abcdefg
`
	outYAML, err := pruning.PruneYAML([]byte(yamlStr))
	if err != nil {
		t.Fatalf("PruneYAML 失败: %v", err)
	}
	if strings.Contains(string(outYAML), "abcdefg") {
		t.Errorf("PruneYAML 输出未脱敏 token")
	}
	if !strings.Contains(string(outYAML), pruning.RedactedValue) {
		t.Errorf("PruneYAML 输出缺少 %s 掩码", pruning.RedactedValue)
	}
}
