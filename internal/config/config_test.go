// Package config_test 针对配置解析器开展契约与边界测试。
//
// @author Ateng
// @since 2026-10-06
package config_test

import (
	"testing"

	"github.com/atengk/mcp-server-kubernetes/internal/config"
)

func TestParse_DefaultValues(t *testing.T) {
	cfg, err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("预期默认参数解析成功，但得到错误: %v", err)
	}

	if cfg.Transport != config.TransportStdio {
		t.Errorf("预期默认 Transport 为 %q，实际得到 %q", config.TransportStdio, cfg.Transport)
	}

	if cfg.Port != config.DefaultPort {
		t.Errorf("预期默认 Port 为 %d，实际得到 %d", config.DefaultPort, cfg.Port)
	}

	if cfg.AllowWrite {
		t.Errorf("预期默认只读模式 AllowWrite 为 false")
	}

	if cfg.AllowExec {
		t.Errorf("预期默认 AllowExec 为 false")
	}

	if cfg.ShowHelp || cfg.ShowVersion {
		t.Errorf("预期默认 ShowHelp 与 ShowVersion 均为 false")
	}
}

func TestParse_CustomFlags(t *testing.T) {
	args := []string{
		"--transport", "sse",
		"--port", "9090",
		"--allow-write",
		"--allow-exec",
		"--kubeconfig", "/path/to/kubeconfig",
		"--context", "prod-cluster",
	}

	cfg, err := config.Parse(args)
	if err != nil {
		t.Fatalf("预期自定义参数解析成功，但得到错误: %v", err)
	}

	if cfg.Transport != config.TransportSSE {
		t.Errorf("预期 Transport 为 %q，实际为 %q", config.TransportSSE, cfg.Transport)
	}
	if cfg.Port != 9090 {
		t.Errorf("预期 Port 为 9090，实际为 %d", cfg.Port)
	}
	if !cfg.AllowWrite {
		t.Errorf("预期 AllowWrite 为 true")
	}
	if !cfg.AllowExec {
		t.Errorf("预期 AllowExec 为 true")
	}
	if cfg.Kubeconfig != "/path/to/kubeconfig" {
		t.Errorf("预期 Kubeconfig 为 %q，实际为 %q", "/path/to/kubeconfig", cfg.Kubeconfig)
	}
	if cfg.KubeContext != "prod-cluster" {
		t.Errorf("预期 KubeContext 为 %q，实际为 %q", "prod-cluster", cfg.KubeContext)
	}
}

func TestParse_HelpAndVersionFlags(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		wantHelp    bool
		wantVersion bool
	}{
		{
			name:     "short help flag",
			args:     []string{"-h"},
			wantHelp: true,
		},
		{
			name:     "long help flag",
			args:     []string{"--help"},
			wantHelp: true,
		},
		{
			name:        "short version flag",
			args:        []string{"-v"},
			wantVersion: true,
		},
		{
			name:        "long version flag",
			args:        []string{"--version"},
			wantVersion: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Parse(tc.args)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if cfg.ShowHelp != tc.wantHelp {
				t.Errorf("ShowHelp 不符合预期: 预期 %v, 实际 %v", tc.wantHelp, cfg.ShowHelp)
			}
			if cfg.ShowVersion != tc.wantVersion {
				t.Errorf("ShowVersion 不符合预期: 预期 %v, 实际 %v", tc.wantVersion, cfg.ShowVersion)
			}
		})
	}
}

func TestParse_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "invalid transport",
			args: []string{"--transport", "websocket"},
		},
		{
			name: "port too low",
			args: []string{"--port", "0"},
		},
		{
			name: "port too high",
			args: []string{"--port", "70000"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Parse(tc.args)
			if err == nil {
				t.Errorf("参数 %v 预期报错，但成功解析", tc.args)
			}
		})
	}
}
