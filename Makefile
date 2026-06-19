# LCchat 本地依赖与代码生成入口

.PHONY: help env mod tools proto tidy docker-up docker-down

help:
	@echo "常用目标:"
	@echo "  make env        - 从示例复制 deploy/env/chatserver.env（若不存在）"
	@echo "  make mod        - go mod download"
	@echo "  make tools      - 安装 buf 与 protoc 插件到 GOPATH/bin"
	@echo "  make proto      - 用 buf 生成 apps/*/pb 下 Go 代码"
	@echo "  make tidy       - go mod tidy"
	@echo "  make docker-up  - docker compose up -d（需已配置 chatserver.env）"
	@echo "  make docker-down - docker compose down"

env:
	@test -f deploy/env/chatserver.env || (cp deploy/env/chatserver.env.example deploy/env/chatserver.env && echo "已创建 deploy/env/chatserver.env，请按需修改密钥与邮箱授权码")
	@test -f deploy/env/chatserver.env && echo "deploy/env/chatserver.env 已存在"

mod:
	go mod download

tools:
	@echo "安装 buf 与 protoc 插件（版本与 Dockerfile / go.mod 对齐）…"
	go install github.com/bufbuild/buf/cmd/buf@v1.55.1
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
	# 独立模块，与 google.golang.org/grpc 运行时版本解耦
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1
	go install github.com/envoyproxy/protoc-gen-validate/cmd/protoc-gen-validate-go@v1.3.0
	@echo "请确保 PATH 包含 $$(go env GOPATH)/bin"

proto: mod
	@command -v buf >/dev/null 2>&1 || (echo "未找到 buf，请先执行: make tools" >&2; exit 1)
	PATH="$$(go env GOPATH)/bin:$$PATH" buf generate --path proto

tidy:
	go mod tidy

docker-up: env
	docker compose up -d --build

docker-down:
	docker compose down
