# 使用更轻的 Go 1.25 Alpine 镜像
FROM golang:1.25-alpine

WORKDIR /app

# 1. 设置国内代理
ENV GOPROXY=https://goproxy.cn,direct

# 2. 拷贝 go.mod 和 go.sum 下载依赖
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# 3. 拷贝源码
COPY . .

# 4. 暴露端口（在 docker-compose 中会覆盖）
EXPOSE 8080 9090

# 注意：不指定 CMD，具体的启动命令由 docker-compose.yml 中的各服务定义
# 每个服务的 working_dir 和 command 在 docker-compose.yml 中指定
