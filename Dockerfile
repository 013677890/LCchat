FROM golang:1.25-alpine AS builder

WORKDIR /src

# 使用国内代理加速依赖下载。
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0
ENV GOOS=linux

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

# 统一预编译所有服务二进制，避免 k8s Pod 启动时再 go run 现场编译。
RUN mkdir -p /out \
    && go build -trimpath -ldflags="-s -w" -o /out/auth ./apps/auth/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/user ./apps/user/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/relation ./apps/relation/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/group ./apps/group/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/msg ./apps/msg/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/gateway ./apps/gateway/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/connect ./apps/connect/cmd \
    && go build -trimpath -ldflags="-s -w" -o /out/message-push ./apps/message-push/cmd

FROM alpine:3.21

WORKDIR /app

# 运行时保留证书、时区数据和基础调试命令，兼容现有 healthcheck 与 preStop 脚本。
# 同时补齐各服务目录，兼容当前 compose / k8s 清单里保留的 workingDir。
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /app/apps/auth /app/apps/user /app/apps/relation /app/apps/group /app/apps/msg /app/apps/gateway /app/apps/connect /app/apps/message-push

COPY --from=builder /out /app/bin

EXPOSE 8080 8081 8084 9090 9091 9092 9093 9094 9095 9190 9192 9193 9194 9195

# 具体启动命令由 docker-compose 与 k8s 清单按服务注入。
