# connect 服务

connect 服务是 WebSocket 长连接和最后一公里投递管道，负责连接鉴权、在线路由维护、设备生命周期同步和下行消息入队。

## 职责

- 暴露 `GET /ws` WebSocket 接入入口。
- 校验 `token` 和 `device_id`，建立连接级身份 `Session`。
- 维护本节点在线连接表，支持同设备新连接替换旧连接。
- 写入、刷新和删除 Redis 在线路由 `user:routing:{user_uuid}`。
- 接收 message-push 或内部服务的 gRPC 推送请求，并投递到本节点在线连接。
- 处理客户端心跳和 `MSG_ACK`，写入设备级 ACK 位点。
- 异步同步设备在线/离线/活跃状态到 auth/device 服务。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/connect/cmd` | HTTP/gRPC 服务启动、依赖注入。 |
| `apps/connect/internal/server` | Gin HTTP Server，暴露 `/health`、`/metrics`、`/ws`。 |
| `apps/connect/internal/handler` | WebSocket 握手、帧处理、错误返回。 |
| `apps/connect/internal/svc` | 鉴权、生命周期、路由、ACK 业务逻辑。 |
| `apps/connect/internal/manager` | 本节点连接注册表和客户端写队列。 |
| `apps/connect/internal/grpc` | ConnectService gRPC 服务。 |
| `proto/connect` | WebSocket Envelope、ACK、Connect gRPC 契约。 |

## 对外入口

| 协议 | 地址 | 说明 |
| --- | --- | --- |
| HTTP | `CONNECT_ADDR`，默认 `:8081` | `/health`、`/metrics`、`/ws`。 |
| gRPC | `CONNECT_GRPC_ADDR`，默认由环境变量配置 | message-push 调用推送能力。 |

## WebSocket 鉴权

握手 URL：

```text
GET /ws?token=<access_token>&device_id=<device_id>
```

鉴权规则：

1. `token` 和 `device_id` 必填。
2. 解析 JWT，要求 claims 中有 `user_uuid` 和 `device_id`。
3. claims 的 `device_id` 必须等于 query `device_id`。
4. Redis 存在时校验 `auth:at:{user_uuid}:{device_id}` 中的 token MD5。
5. Redis 异常时 Fail-Close，拒绝连接。

## WebSocket 帧

当前 connect 已移除旧的 JSON / Text Frame 兼容实现，只保留 Binary Protobuf 协议主路径。

| 方向 | type | payload | 说明 |
| --- | --- | --- | --- |
| 上行 | `heartbeat` | 空 | 刷新路由活跃时间，响应 `heartbeat_ack`。 |
| 上行 | `MSG_ACK` / `msg_ack` | `MessageAck` | 客户端确认已处理到某会话 seq。 |
| 上行 | `message` | 当前未接入业务持久化 | 仅返回占位 `message_ack`。 |
| 下行 | `MSG_PUSH` | `msg.MsgItem` | 新消息。 |
| 下行 | `MSG_RECALL` | `msg.RecallNotice` | 撤回通知。 |
| 下行 | `MSG_MARK_READ` | `msg.MarkReadNotice` | 已读多端同步。 |
| 下行 | `MSG_ACK_ACK` | `MessageAckAck` | ACK 已写入确认。 |
| 下行 | `error` | `ErrorFrame` | 协议层错误。 |

完整前端协议见 [WebSocket协议](../api/06-WebSocket协议.md)。

## 在线路由

Redis Hash：

```text
key   = user:routing:{user_uuid}
field = {device_id}
value = {connectGrpcAddr}|{lastActiveMs}
TTL   = 180s（默认）
```

生命周期：

| 时机 | 动作 |
| --- | --- |
| OnConnect | 写入路由，异步上报设备在线。 |
| OnHeartbeat | 刷新路由活跃时间和 TTL。 |
| OnDisconnect | 删除设备路由，异步上报设备离线。 |
| 节点启动/清理 | 可按 connect 地址移除本节点遗留路由。 |

## ACK 位点

| 项 | 值 |
| --- | --- |
| Redis Key | `msg:ack:{user_uuid}:{device_id}:{conv_id}` |
| 数据类型 | String，保存最大已 ACK seq。 |
| TTL | 30 天。 |
| 合并方式 | Lua 脚本单调前进，只保存更大 seq。 |

ACK 校验：

- `conv_id` 必须非空。
- `seq` 必须大于 0。
- `seq` 不能超过当前连接已成功下发的最大 seq。
- ACK 写入失败返回 `error` 帧，前端可稍后重试。

## gRPC 能力

| RPC | 调用方 | 说明 |
| --- | --- | --- |
| `PushToDevice` | message-push、内部服务 | 向指定用户指定设备投递。 |
| `PushToUser` | message-push | 向本节点上指定用户所有在线设备投递。 |
| `BroadcastToUsers` | 管理后台/运维 | 向多个用户广播相同消息。 |
| `KickConnection` | auth/user-service 或管理能力 | 精确踢掉指定设备连接。 |

## 降级策略

- auth device gRPC 客户端为 nil 时，在线/离线状态同步会降级跳过，但 WebSocket 连接仍可工作。
- Redis 不可用时，WebSocket 鉴权 Fail-Close；连接建立后路由/ACK 写入失败会影响实时投递可靠性。
- 下行写队列入队失败通常说明连接不可写，connect 会主动关闭连接。

## 不变量

- connect 不消费 Kafka，不做消息业务判断。
- connect 不保存离线消息，离线和缺口由 msg 拉取自愈。
- WebSocket 当前只接受 Binary Protobuf 帧，不接受 Text Frame。
- 同一 `user_uuid + device_id` 在同一节点只保留一条最新连接。
- token 被登出或踢设备清理后，后续 WebSocket 握手必须失败。
