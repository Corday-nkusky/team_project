# 后端 + 前端镜像：多阶段构建，运行期镜像不含编译工具链与 Node。
#
# 阶段 1：构建前端（产物拷进 Go 的嵌入目录，最终由同一个二进制提供）
# 阶段 2：构建 Go 服务
# 阶段 3：精简运行镜像
#
# 构建上下文为仓库根目录：docker build -f deploy/backend.Dockerfile .

FROM node:22-alpine AS frontend
WORKDIR /fe
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
# 输出到容器内独立目录，稍后拷入 Go 的嵌入目录。
RUN VITE_OUT_DIR=dist npx vite build

FROM golang:1.27 AS build
# 模块代理走国内镜像（proxy.golang.org 在部分网络下不可达）；
# GOTOOLCHAIN=local：镜像内 go 版本已满足 go.mod，禁止再联网下载工具链。
ENV GOPROXY=https://goproxy.cn,direct \
    GOTOOLCHAIN=local
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# 用刚构建的前端覆盖嵌入目录（清掉可能存在的本地构建残留）。
RUN rm -rf ./internal/web/dist && mkdir -p ./internal/web/dist
COPY --from=frontend /fe/dist/ ./internal/web/dist/
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.21
# ca-certificates：调用模型网关（HTTPS）必需；tzdata：让日志时间可读。
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
USER app
COPY --from=build /out/server /usr/local/bin/server
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
