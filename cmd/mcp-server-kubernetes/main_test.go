// Package main 针对启动入口开展即时指令与异常入参测试。
//
// @author Ateng
// @since 2026-10-06
package main

import (
	"testing"
)

func TestRun_HelpAndVersion(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("预期 --help 退出无错误，实际得到: %v", err)
	}

	if err := run([]string{"--version"}); err != nil {
		t.Errorf("预期 --version 退出无错误，实际得到: %v", err)
	}
}

func TestRun_InvalidArgs(t *testing.T) {
	if err := run([]string{"--invalid-flag"}); err == nil {
		t.Errorf("预期未知参数返回错误，但未报错")
	}
}
