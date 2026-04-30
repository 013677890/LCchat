#!/usr/bin/env bash
# 在仓库根目录生成 protobuf / gRPC 代码（需已安装 protoc，见 Makefile tools）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PGV="$(go env GOPATH)/pkg/mod/github.com/envoyproxy/protoc-gen-validate@v1.3.0"
if [[ ! -d "$PGV" ]]; then
	echo "未找到 protoc-gen-validate 模块目录，请先执行: go mod download" >&2
	exit 1
fi

PROTO_FILES=(
	proto/common/common.proto
	proto/auth/auth_service.proto
	proto/auth/device_service.proto
	proto/auth/account_service.proto
	proto/auth/internal_auth_service.proto
	proto/relation/friend_service.proto
	proto/relation/blacklist_service.proto
	proto/user/common.proto
	proto/user/auth_service.proto
	proto/user/user_service.proto
	proto/user/device_service.proto
	proto/user/friend_service.proto
	proto/user/blacklist_service.proto
	proto/user/internal_profile_service.proto
	proto/group/group_service.proto
	proto/connect/connect.proto
	proto/msg/msg_common.proto
	proto/msg/msg_push_event.proto
	proto/msg/msg_service.proto
)

# 使用 go_package + module 组合将生成结果输出到各服务自己的 pb 目录。
protoc \
	-I . \
	-I "$PGV" \
	--experimental_allow_proto3_optional \
	--go_out=. \
	--go_opt=module=github.com/013677890/LCchat-Backend \
	--go-grpc_out=. \
	--go-grpc_opt=module=github.com/013677890/LCchat-Backend \
	"${PROTO_FILES[@]}"

if command -v protoc-gen-validate-go >/dev/null 2>&1; then
	# validate 代码与 pb.go 保持同一输出目录，便于服务侧统一引用。
	protoc \
		-I . \
		-I "$PGV" \
		--experimental_allow_proto3_optional \
		--plugin=protoc-gen-validate="$(command -v protoc-gen-validate-go)" \
		--validate_out=module=github.com/013677890/LCchat-Backend:. \
		"${PROTO_FILES[@]}"
	echo "已生成 validate 代码（protoc-gen-validate-go）"
else
	echo "跳过 validate：未在 PATH 中找到 protoc-gen-validate-go，可执行 make tools 安装"
fi

echo "proto 生成完成"
