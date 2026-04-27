# Proto 管理与生成说明

## 1. 目录约定

- Proto 源文件统一放在仓库根目录 `proto/` 下：
  - `proto/user/*.proto`
  - `proto/connect/connect.proto`
  - `proto/msg/*.proto`
- 生成代码输出到各服务目录：
  - `apps/user/pb/*.pb.go`
  - `apps/connect/pb/*.pb.go`
  - `apps/msg/pb/*.pb.go`

## 2. 前置依赖

确保已安装以下工具并在 `PATH` 中：

- `protoc`
- `protoc-gen-go`
- `protoc-gen-go-grpc`
- （可选）`protoc-gen-validate`

## 3. 生成命令（PowerShell）

```powershell
# 仓库根目录执行
$PGV = Join-Path (go env GOPATH) "pkg\mod\github.com\envoyproxy\protoc-gen-validate@v1.3.0"

protoc `
  -I . `
  -I "$PGV" `
  --experimental_allow_proto3_optional `
  --go_out=. `
  --go_opt=module=github.com/013677890/LCchat-Backend `
  --go-grpc_out=. `
  --go-grpc_opt=module=github.com/013677890/LCchat-Backend `
  proto/user/common.proto `
  proto/user/auth_service.proto `
  proto/user/user_service.proto `
  proto/user/device_service.proto `
  proto/user/friend_service.proto `
  proto/user/blacklist_service.proto `
  proto/connect/connect.proto `
  proto/msg/msg_common.proto `
  proto/msg/msg_push_event.proto `
  proto/msg/msg_service.proto
```

说明：

- `module=github.com/013677890/LCchat-Backend` 用于按 `go_package` 生成到 `apps/*/pb`，不会输出到 `proto/` 目录。
- 上面命令生成 `*.pb.go` 和 `*_grpc.pb.go`。

## 4. 生成 validate 代码（可选）

若你本地安装了 `protoc-gen-validate`，可再执行：

```powershell
protoc `
  -I . `
  -I "$PGV" `
  --experimental_allow_proto3_optional `
  --validate_out=lang=go,module=github.com/013677890/LCchat-Backend:. `
  proto/user/common.proto `
  proto/user/auth_service.proto `
  proto/user/user_service.proto `
  proto/user/device_service.proto `
  proto/user/friend_service.proto `
  proto/user/blacklist_service.proto `
  proto/connect/connect.proto ` 
  proto/msg/msg_common.proto `
  proto/msg/msg_push_event.proto `
  proto/msg/msg_service.proto
```

如果你的 `protoc-gen-validate` 版本不支持 `module` 参数，请按你本地插件版本文档调整参数。

## 5. 生成命令（Ubuntu / Bash）

### 5.1 生成 Go + gRPC 代码

```bash
# 仓库根目录执行
PGV="$(go env GOPATH)/pkg/mod/github.com/envoyproxy/protoc-gen-validate@v1.3.0"

protoc \
  -I . \
  -I "$PGV" \
  --experimental_allow_proto3_optional \
  --go_out=. \
  --go_opt=module=github.com/013677890/LCchat-Backend \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/013677890/LCchat-Backend \
  proto/user/common.proto \
  proto/user/auth_service.proto \
  proto/user/user_service.proto \
  proto/user/device_service.proto \
  proto/user/friend_service.proto \
  proto/user/blacklist_service.proto \
  proto/connect/connect.proto \
  proto/msg/msg_common.proto \
  proto/msg/msg_push_event.proto \
  proto/msg/msg_service.proto
```

### 5.2 生成 validate 代码（可选）

```bash
PGV="$(go env GOPATH)/pkg/mod/github.com/envoyproxy/protoc-gen-validate@v1.3.0"

protoc \
  -I . \
  -I "$PGV" \
  --experimental_allow_proto3_optional \
  --validate_out=lang=go,module=github.com/013677890/LCchat-Backend:. \
  proto/user/common.proto \
  proto/user/auth_service.proto \
  proto/user/user_service.proto \
  proto/user/device_service.proto \
  proto/user/friend_service.proto \
  proto/user/blacklist_service.proto \
  proto/connect/connect.proto \
  proto/msg/msg_common.proto \
  proto/msg/msg_push_event.proto \
  proto/msg/msg_service.proto
```
