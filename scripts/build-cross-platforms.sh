#!/usr/bin/env bash

# ==============================================================================
# 5 大主流平台架构 Go 原生二进制交叉编译构建脚本
# 遵循 Tag SSOT 军规，通过 -ldflags 动态注入构建元数据
# ==============================================================================

set -euo pipefail

VERSION="${1:-ci-test}"
COMMIT="${2:-none}"
BUILD_DATE="${3:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUT_DIR="${4:-dist}"

mkdir -p "${OUT_DIR}"

platforms=(
  "darwin/arm64/darwin-arm64/mcp-server-kubernetes"
  "darwin/amd64/darwin-x64/mcp-server-kubernetes"
  "linux/amd64/linux-x64/mcp-server-kubernetes"
  "linux/arm64/linux-arm64/mcp-server-kubernetes"
  "windows/amd64/win32-x64/mcp-server-kubernetes.exe"
)

for item in "${platforms[@]}"; do
  IFS='/' read -r goos goarch target binary <<< "$item"
  echo "==> 正在编译目标平台: ${target} (${goos}/${goarch})"
  target_dir="${OUT_DIR}/mcp-server-kubernetes-${target}"
  mkdir -p "${target_dir}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build \
    -trimpath \
    -ldflags="-s -w \
      -X 'github.com/atengk/mcp-server-kubernetes/internal/version.Version=${VERSION}' \
      -X 'github.com/atengk/mcp-server-kubernetes/internal/version.Commit=${COMMIT}' \
      -X 'github.com/atengk/mcp-server-kubernetes/internal/version.BuildDate=${BUILD_DATE}'" \
    -o "${target_dir}/${binary}" \
    ./cmd/mcp-server-kubernetes
done

echo "==> 5 大平台架构交叉编译全部完成，产物已写入 ${OUT_DIR}"
