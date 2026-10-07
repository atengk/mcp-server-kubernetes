# ==============================================================================
# MCP Server Kubernetes - 生产级多阶段容器镜像构建
# ==============================================================================

# 构建阶段：多架构跨平台静态编译
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

WORKDIR /src

# 安装基础证书与工具
RUN apk add --no-cache ca-certificates git

# 优先缓存依赖模块层
COPY go.mod go.sum ./
RUN go mod download

# 拷贝项目源码
COPY . .

# 接收多架构构建参数
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

# 纯静态编译 (CGO_ENABLED=0)，注入构建期版本元数据
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w \
      -X 'github.com/atengk/mcp-server-kubernetes/internal/version.Version=${VERSION}' \
      -X 'github.com/atengk/mcp-server-kubernetes/internal/version.Commit=${COMMIT}' \
      -X 'github.com/atengk/mcp-server-kubernetes/internal/version.BuildDate=${BUILD_DATE}'" \
    -o /app/mcp-server-kubernetes \
    ./cmd/mcp-server-kubernetes

# 运行阶段：极简非 root 安全镜像
FROM alpine:3.20

# 安装运行时基础根证书与时区数据
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# 从构建阶段提取静态二进制文件
COPY --from=builder /app/mcp-server-kubernetes /usr/local/bin/mcp-server-kubernetes

# 切换为非特权用户
USER appuser

# 暴露 SSE 传输模式可选端口
EXPOSE 8080

# 默认启动命令 (stdio 模式)
ENTRYPOINT ["mcp-server-kubernetes"]
CMD ["--transport", "stdio"]
