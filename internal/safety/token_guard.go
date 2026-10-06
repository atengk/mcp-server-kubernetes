// Package safety 提供面向 Kubernetes 资源的 Token Guard 上下文保护与超量输出截断守卫。
//
// @author Ateng
// @since 2026-10-06
package safety

import (
	"fmt"
	"strings"
)

const (
	// MaxListItems 声明 k8s_list_resources 等列表查询单次返回的对象数量上限
	MaxListItems = 100

	// DefaultLogTail 声明获取 Pod 容器日志时的默认行数
	DefaultLogTail = 100

	// MaxLogTail 声明客户端请求日志行数的硬性安全上限（超量将被强制截断）
	MaxLogTail = 1000
)

// TruncateItems 对任意类型的泛型切片执行长度截断保护，返回截断后的切片及是否发生截断标记。
//
// @param items 待检查的输入切片
// @param max 允许的最大元素数量
// @return []T 截断后的安全切片
// @return bool 为 true 表示输入切片超出上限并已执行截断
func TruncateItems[T any](items []T, max int) ([]T, bool) {
	if items == nil || max <= 0 {
		return []T{}, items != nil && len(items) > 0
	}
	if len(items) <= max {
		return items, false
	}
	return items[:max], true
}

// TruncateLogLines 对多行日志文本执行行数上限截断（保留最新尾部的 maxLines 行）。
//
// @param log 原始多行日志文本
// @param maxLines 允许保留的最大行数
// @return string 截断后的安全日志文本
// @return bool 为 true 表示日志行数超出上限并已执行截断
func TruncateLogLines(log string, maxLines int) (string, bool) {
	if log == "" || maxLines <= 0 {
		return "", log != ""
	}

	lines := strings.Split(log, "\n")
	if len(lines) <= maxLines {
		return log, false
	}

	// 保留末尾最新的 maxLines 行
	tailLines := lines[len(lines)-maxLines:]
	return strings.Join(tailLines, "\n"), true
}

// FormatTruncationWarning 格式化标准截断警告信息，用于追加在 MCP 工具输出末尾提示模型。
//
// @param reason 导致截断的具体原因说明
// @return string 格式化的警示文本
func FormatTruncationWarning(reason string) string {
	return fmt.Sprintf("\n\n[OUTPUT TRUNCATED: %s]", reason)
}
