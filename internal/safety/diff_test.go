// Package safety_test 针对差异对比引擎开展单元测试。
//
// @author Ateng
// @since 2026-10-06
package safety_test

import (
	"strings"
	"testing"

	"github.com/atengk/mcp-server-kubernetes/internal/safety"
)

func TestGenerateUnifiedDiff_NoChanges(t *testing.T) {
	content := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx\n"
	diff := safety.GenerateUnifiedDiff(content, content, "current", "proposed")
	if diff != "" {
		t.Errorf("内容完全一致时预期返回空 diff，实际得到: %q", diff)
	}
}

func TestGenerateUnifiedDiff_AddedAndRemovedLines(t *testing.T) {
	oldContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  image: nginx:1.20
`
	newContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  image: nginx:1.21
  env: prod
`

	diff := safety.GenerateUnifiedDiff(oldContent, newContent, "current", "proposed")
	if diff == "" {
		t.Fatalf("预期产生 diff，但结果为空")
	}

	if !strings.Contains(diff, "--- current") || !strings.Contains(diff, "+++ proposed") {
		t.Errorf("diff 缺少标准 unified 头部: %s", diff)
	}
	if !strings.Contains(diff, "-  replicas: 1") || !strings.Contains(diff, "+  replicas: 3") {
		t.Errorf("diff 未能正确标记 replicas 变更: %s", diff)
	}
	if !strings.Contains(diff, "-  image: nginx:1.20") || !strings.Contains(diff, "+  image: nginx:1.21") {
		t.Errorf("diff 未能正确标记 image 变更: %s", diff)
	}
	if !strings.Contains(diff, "+  env: prod") {
		t.Errorf("diff 未能正确标记新增行: %s", diff)
	}
}

func TestGenerateUnifiedDiff_EmptyOld(t *testing.T) {
	newContent := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n"
	diff := safety.GenerateUnifiedDiff("", newContent, "current", "proposed")
	if diff == "" {
		t.Fatalf("原有为空时预期产生新增 diff")
	}
	if !strings.Contains(diff, "+apiVersion: v1") {
		t.Errorf("新建资源未标记为新增行: %s", diff)
	}
}
