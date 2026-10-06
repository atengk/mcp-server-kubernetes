// Package version_test 针对版本元数据展示开展单测。
//
// @author Ateng
// @since 2026-10-06
package version_test

import (
	"strings"
	"testing"

	"github.com/atengk/mcp-server-kubernetes/internal/version"
)

func TestInfo_String(t *testing.T) {
	info := version.GetInfo()
	str := info.String()

	if !strings.Contains(str, "mcp-server-kubernetes") {
		t.Errorf("版本输出应包含项目名称，实际得到: %s", str)
	}

	if !strings.Contains(str, info.Version) {
		t.Errorf("版本输出应包含版本号 %s，实际得到: %s", info.Version, str)
	}
}
