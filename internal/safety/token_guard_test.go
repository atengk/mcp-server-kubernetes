// Package safety_test 针对 Token Guard 上下文保护与截断守卫开展单测。
//
// @author Ateng
// @since 2026-10-06
package safety_test

import (
	"strings"
	"testing"

	"github.com/atengk/mcp-server-kubernetes/internal/safety"
)

func TestTruncateItems(t *testing.T) {
	items := make([]int, 150)
	for i := 0; i < 150; i++ {
		items[i] = i
	}

	truncated, wasTruncated := safety.TruncateItems(items, safety.MaxListItems)
	if !wasTruncated {
		t.Errorf("预期发生截断，但 wasTruncated 为 false")
	}
	if len(truncated) != safety.MaxListItems {
		t.Errorf("截断后长度应为 %d, 实际为 %d", safety.MaxListItems, len(truncated))
	}

	// 验证未超限情况
	smallItems := []int{1, 2, 3}
	notTruncated, wasTruncated := safety.TruncateItems(smallItems, safety.MaxListItems)
	if wasTruncated {
		t.Errorf("未超限列表不应发生截断")
	}
	if len(notTruncated) != 3 {
		t.Errorf("未截断列表长度应为 3, 实际为 %d", len(notTruncated))
	}

	// 验证 nil 切片保底返回空切片
	var nilSlice []string
	safeNil, wasTrunc := safety.TruncateItems(nilSlice, 10)
	if wasTrunc {
		t.Errorf("nil 切片不应发生截断")
	}
	if safeNil == nil {
		t.Errorf("nil 入参预期返回空切片 []string{}，实际返回了 nil")
	}
	if len(safeNil) != 0 {
		t.Errorf("空切片长度应为 0")
	}
}

func TestTruncateLogLines(t *testing.T) {
	var lines []string
	for i := 1; i <= 200; i++ {
		lines = append(lines, "log line")
	}
	rawLog := strings.Join(lines, "\n")

	// 截断至 100 行
	res, wasTruncated := safety.TruncateLogLines(rawLog, 100)
	if !wasTruncated {
		t.Errorf("预期发生日志截断")
	}
	resLines := strings.Split(res, "\n")
	if len(resLines) != 100 {
		t.Errorf("截断后行数应为 100，实际为 %d", len(resLines))
	}
}

func TestFormatTruncationWarning(t *testing.T) {
	warn := safety.FormatTruncationWarning("已达 100 项上限")
	if !strings.Contains(warn, "[OUTPUT TRUNCATED: 已达 100 项上限]") {
		t.Errorf("截断警示格式不符合预期: %s", warn)
	}
}
