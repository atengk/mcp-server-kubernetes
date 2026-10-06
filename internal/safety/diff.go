// Package safety 封装变更演练对比引擎、Token Guard 守卫与运行时安全截断逻辑。
//
// @author Ateng
// @since 2026-10-06
package safety

import (
	"fmt"
	"strings"
)

// GenerateUnifiedDiff 计算两份多行文本的行级差异并输出标准 Unified Diff 格式文本。
// 若两份文本完全一致，返回空字符串。
//
// @param oldContent 变更前的原始文本内容
// @param newContent 变更后的期望文本内容
// @param oldLabel 差异头中原始文本的标识标签（如 current）
// @param newLabel 差异头中目标文本的标识标签（如 proposed）
// @return string 标准 Unified Diff 差异字符串
func GenerateUnifiedDiff(oldContent, newContent, oldLabel, newLabel string) string {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// 计算 LCS (最长公共子序列) 矩阵
	m := len(oldLines)
	n := len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			if oldLines[i] == newLines[j] {
				dp[i+1][j+1] = dp[i][j] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i+1][j+1] = dp[i+1][j]
			} else {
				dp[i+1][j+1] = dp[i][j+1]
			}
		}
	}

	// 回溯生成 Diff 编辑操作
	type diffOp struct {
		op   byte // ' ', '+', '-'
		text string
	}
	var ops []diffOp
	i, j := m, n
	hasChange := false

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			ops = append(ops, diffOp{op: ' ', text: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			ops = append(ops, diffOp{op: '+', text: newLines[j-1]})
			hasChange = true
			j--
		} else if i > 0 && (j == 0 || dp[i][j-1] < dp[i-1][j]) {
			ops = append(ops, diffOp{op: '-', text: oldLines[i-1]})
			hasChange = true
			i--
		}
	}

	if !hasChange {
		return ""
	}

	// 翻转为正序
	for left, right := 0, len(ops)-1; left < right; left, right = left+1, right-1 {
		ops[left], ops[right] = ops[right], ops[left]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", oldLabel))
	sb.WriteString(fmt.Sprintf("+++ %s\n", newLabel))
	sb.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", m, n))

	for _, op := range ops {
		sb.WriteByte(op.op)
		sb.WriteString(op.text)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// splitLines 标准化按行分割多行文本并清理末尾换行回车符。
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	// 规范化 CRLF 为 LF
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// 去除末尾单个冗余换行避免多出空行
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}
