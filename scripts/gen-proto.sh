#!/usr/bin/env bash
# 在仓库根目录生成 protobuf / gRPC 代码（需已安装 protoc，见 Makefile tools）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GO_BIN="${GO_BIN:-go}"

normalize_path_for_bash() {
	local path="$1"
	path="${path%%;*}"

	if [[ "$path" =~ ^[A-Za-z]:\\ ]]; then
		if command -v wslpath >/dev/null 2>&1; then
			wslpath -a "$path"
			return
		fi

		local drive_letter="${path:0:1}"
		drive_letter="$(printf '%s' "$drive_letter" | tr '[:upper:]' '[:lower:]')"
		local rest="${path:2}"
		rest="${rest//\\//}"
		printf '/mnt/%s%s\n' "$drive_letter" "$rest"
		return
	fi

	printf '%s\n' "$path"
}

# 兼容在 WSL 中通过 Windows go.exe 读取 GOPATH 的场景。
GO_GOPATH="$(normalize_path_for_bash "$($GO_BIN env GOPATH)")"
PGV="${PGV_DIR:-$GO_GOPATH/pkg/mod/github.com/envoyproxy/protoc-gen-validate@v1.3.0}"
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
	proto/user/user_service.proto
	proto/user/internal_profile_service.proto
	proto/group/group_service.proto
	proto/connect/connect.proto
	proto/connect/ws_control.proto
	proto/msg/msg_common.proto
	proto/msg/msg_push_event.proto
	proto/msg/msg_service.proto
	proto/realtime/realtime_event.proto
)

PROTOC_ARGS=(
	-I .
	-I "$PGV"
	--experimental_allow_proto3_optional
)

GO_PLUGIN_ARGS=()
if [[ -n "${PROTOC_GEN_GO_BIN:-}" ]]; then
	GO_PLUGIN_ARGS+=(--plugin=protoc-gen-go="$PROTOC_GEN_GO_BIN")
fi
if [[ -n "${PROTOC_GEN_GO_GRPC_BIN:-}" ]]; then
	GO_PLUGIN_ARGS+=(--plugin=protoc-gen-go-grpc="$PROTOC_GEN_GO_GRPC_BIN")
fi

# 使用 go_package + module 组合将生成结果输出到各服务自己的 pb 目录。
protoc \
	"${PROTOC_ARGS[@]}" \
	"${GO_PLUGIN_ARGS[@]}" \
	--go_out=. \
	--go_opt=module=github.com/013677890/LCchat-Backend \
	--go-grpc_out=. \
	--go-grpc_opt=module=github.com/013677890/LCchat-Backend \
	"${PROTO_FILES[@]}"

VALIDATE_PLUGIN="${PROTOC_GEN_VALIDATE_GO_BIN:-}"
if [[ -z "$VALIDATE_PLUGIN" ]] && command -v protoc-gen-validate-go >/dev/null 2>&1; then
	VALIDATE_PLUGIN="$(command -v protoc-gen-validate-go)"
fi

if [[ -n "$VALIDATE_PLUGIN" ]]; then
	# validate 代码与 pb.go 保持同一输出目录，便于服务侧统一引用。
	protoc \
		"${PROTOC_ARGS[@]}" \
		--plugin=protoc-gen-validate="$VALIDATE_PLUGIN" \
		--validate_out=module=github.com/013677890/LCchat-Backend:. \
		"${PROTO_FILES[@]}"
	echo "已生成 validate 代码（protoc-gen-validate-go）"
else
	echo "跳过 validate：未在 PATH 中找到 protoc-gen-validate-go，可执行 make tools 安装"
fi

echo "proto 生成完成"
