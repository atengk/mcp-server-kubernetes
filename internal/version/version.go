// Package version 提供构建期动态注入的版本元数据与格式化呈现。
//
// @author Ateng
// @since 2026-10-06
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version 是编译期通过 -ldflags "-X ...Version=..." 动态注入的版本号（遵循 Tag SSOT 军规）
	Version = "dev"

	// Commit 是编译期注入的 Git Commit 散列值
	Commit = "none"

	// BuildDate 是编译期注入的构建时间戳
	BuildDate = "unknown"
)

// Info 包含服务构建与运行时的全量元数据。
type Info struct {
	// Version 语义化版本号
	Version string

	// Commit Git 提交哈希
	Commit string

	// BuildDate 构建时间
	BuildDate string

	// GoVersion Go 运行时版本
	GoVersion string

	// Platform 运行目标操作系统与架构 (OS/Arch)
	Platform string
}

// GetInfo 返回当前二进制的版本元数据对象。
//
// @return Info 包含完整版本与平台信息的结构体
func GetInfo() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String 返回格式化的多行版本信息字符串。
//
// @return string 格式化文本
func (i Info) String() string {
	return fmt.Sprintf("mcp-server-kubernetes %s (commit: %s, built: %s, go: %s, platform: %s)",
		i.Version, i.Commit, i.BuildDate, i.GoVersion, i.Platform)
}
