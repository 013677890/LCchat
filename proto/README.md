# Proto 管理与生成说明

## 1. 目录约定

- Proto 源文件统一放在仓库根目录 `proto/` 下：
  - `proto/common/common.proto` — 跨服务共享类型（PaginationInfo）
  - `proto/auth/*.proto` — 认证、设备、账号安全、内部认证接口
  - `proto/relation/*.proto` — 好友、黑名单
  - `proto/user/*.proto` — 用户资料、内部资料接口
  - `proto/group/*.proto` — 群组
  - `proto/connect/connect.proto`
  - `proto/msg/*.proto`
  - `proto/realtime/realtime_event.proto` — 非消息类实时提醒 Kafka 事件与业务 payload
- 生成代码输出到各服务目录：
  - `pkg/commonpb/*.pb.go`
  - `apps/auth/pb/*.pb.go`
  - `apps/relation/pb/*.pb.go`
  - `apps/user/pb/*.pb.go`
  - `apps/group/pb/*.pb.go`
  - `apps/connect/pb/*.pb.go`
  - `apps/msg/pb/*.pb.go`
  - `pkg/realtimepb/*.pb.go`

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
  proto/common/common.proto `
  proto/auth/auth_service.proto `
  proto/auth/device_service.proto `
  proto/auth/account_service.proto `
  proto/auth/internal_auth_service.proto `
  proto/relation/friend_service.proto `
  proto/relation/blacklist_service.proto `
  proto/user/user_service.proto `
  proto/user/internal_profile_service.proto `
  proto/user/group_service.proto `
  proto/group/group_service.proto `
  proto/connect/connect.proto `
  proto/connect/ws_control.proto `
  proto/msg/msg_common.proto `
  proto/msg/msg_push_event.proto `
  proto/msg/msg_service.proto `
  proto/realtime/realtime_event.proto
```

## 4. 生成 validate 代码（可选）

```powershell
protoc `
  -I . `
  -I "$PGV" `
  --experimental_allow_proto3_optional `
  --validate_out=lang=go,module=github.com/013677890/LCchat-Backend:. `
  proto/common/common.proto `
  proto/auth/auth_service.proto `
  proto/auth/device_service.proto `
  proto/auth/account_service.proto `
  proto/auth/internal_auth_service.proto `
  proto/relation/friend_service.proto `
  proto/relation/blacklist_service.proto `
  proto/user/user_service.proto `
  proto/user/internal_profile_service.proto `
  proto/user/group_service.proto `
  proto/group/group_service.proto `
  proto/connect/connect.proto `
  proto/connect/ws_control.proto `
  proto/msg/msg_common.proto `
  proto/msg/msg_push_event.proto `
  proto/msg/msg_service.proto `
  proto/realtime/realtime_event.proto
```

## 5. Proto Package 映射

| 目录 | package | go_package | 归属服务 |
|------|---------|------------|---------|
| `proto/common/` | `common` | `pkg/commonpb` | 共享 |
| `proto/auth/` | `auth` | `apps/auth/pb` | auth-service |
| `proto/relation/` | `relation` | `apps/relation/pb` | relation-service |
| `proto/user/` | `user` | `apps/user/pb` | user-service |
| `proto/group/` | `group` | `apps/group/pb` | group-service |
| `proto/connect/` | `connect` | `apps/connect/pb` | connect-service |
| `proto/msg/` | `msg` | `apps/msg/pb` | msg-service |
| `proto/realtime/` | `realtime` | `pkg/realtimepb` | realtime.push 事件 |

## 6. 注意事项

- `module=github.com/013677890/LCchat-Backend` 用于按 `go_package` 生成到对应目录。
- `InternalXxxService` proto 文件以 `internal_` 前缀命名，通过 `x-internal-caller` 拦截器鉴权。
- 旧 `proto/user/` 下的 `common.proto`、`auth_service.proto`、`device_service.proto`、`friend_service.proto`、`blacklist_service.proto`、`group_service.proto` 在迁移完成后将被删除。
