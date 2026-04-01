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
	proto/user/common.proto
	proto/user/auth_service.proto
	proto/user/user_service.proto
	proto/user/device_service.proto
	proto/user/friend_service.proto
	proto/user/blacklist_service.proto
	proto/connect/connect.proto
	proto/msg/msg_common.proto
	proto/msg/msg_service.proto
)

protoc \
	-I . \
	-I "$PGV" \
	--experimental_allow_proto3_optional \
	--go_out=. \
	--go_opt=module=ChatServer \
	--go-grpc_out=. \
	--go-grpc_opt=module=ChatServer \
	"${PROTO_FILES[@]}"

if command -v protoc-gen-validate-go >/dev/null 2>&1; then
	protoc \
		-I . \
		-I "$PGV" \
		--experimental_allow_proto3_optional \
		--plugin=protoc-gen-validate="$(command -v protoc-gen-validate-go)" \
		--validate_out=module=ChatServer:. \
		"${PROTO_FILES[@]}"
	echo "已生成 validate 代码（protoc-gen-validate-go）"
else
	echo "跳过 validate：未在 PATH 中找到 protoc-gen-validate-go，可执行 make tools 安装"
fi

echo "proto 生成完成"
