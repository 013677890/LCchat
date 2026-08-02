# WebSocket 协议

本文描述 Connect 服务对前端暴露的 WebSocket 接入协议。实时消息下行、撤回通知、已读同步、非消息类实时提醒和设备级 ACK 均通过该协议完成。HTTP 消息写入与查询接口见 [消息与会话接口](05-消息与会话接口.md)。

## 1. 接入概览

### 1.1 服务入口

| 项 | 值 |
| --- | --- |
| 服务 | connect |
| 默认监听 | `CONNECT_ADDR`，未配置时为 `:8081` |
| WebSocket 路径 | `GET /ws` |
| 健康检查 | `GET /health` |
| 指标 | `GET /metrics` |
| 帧格式 | WebSocket Binary Frame（二进制帧） |
| 二进制协议 | Protobuf `connect.MessageEnvelope` |

浏览器连接示例：

```text
wss://connect.example.com/ws?token=<access_token>&device_id=<device_id>
```

本地开发示例：

```text
ws://localhost:8081/ws?token=<access_token>&device_id=web-chrome-001
```

### 1.2 Query 参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `token` | string | 是 | 登录接口返回的 `accessToken`，不要带 `Bearer ` 前缀。 |
| `device_id` | string | 是 | 设备 ID，必须与登录签发 Token 时的设备 ID 一致。 |

### 1.3 Origin 校验

Connect 会按环境变量 `CONNECT_ALLOWED_ORIGINS` 校验浏览器 `Origin`：

| 配置 | 行为 |
| --- | --- |
| 未配置 | 只允许同源，或 `Origin` 为空的非浏览器客户端。 |
| `*` | 放开所有来源，仅建议本地开发使用。 |
| 逗号分隔域名 | 只允许白名单来源，例如 `https://app.example.com,https://admin.example.com`。 |

## 2. 握手鉴权

### 2.1 鉴权流程

服务端握手阶段按以下顺序校验：

1. `token` 不能为空。
2. `device_id` 不能为空。
3. 解析 JWT（JSON Web Token，访问令牌），要求 claims 中存在 `user_uuid` 和 `device_id`。
4. JWT claims 中的 `device_id` 必须与 query `device_id` 完全一致。
5. Redis 存在时，校验 `auth:at:{user_uuid}:{device_id}` 中保存的 AccessToken MD5 与当前 token 一致。
6. Redis 读取异常时 Fail-Close（失败关闭），直接拒绝连接，保证踢线、登出等安全操作立即生效。

### 2.2 握手失败响应

握手失败发生在协议升级前，因此返回 HTTP JSON，而不是 WebSocket 帧。

| 场景 | HTTP 状态码 | Body `code` | 说明 |
| --- | --- | --- | --- |
| 缺少 `token` | 400 | `17001` | WebSocket 握手缺少 token。 |
| 缺少 `device_id` | 400 | `17002` | WebSocket 握手缺少 device_id。 |
| token 无效/过期/设备不匹配 | 401 | `20002` | Token 无效。 |
| 服务端异常 | 500 | `30001` | 内部错误。 |

失败响应示例：

```json
{
  "code": 17001,
  "message": "WebSocket 握手缺少 token",
  "data": null,
  "trace_id": "trace-xxx",
  "timestamp": 1710000000
}
```

### 2.3 同设备重复连接

同一个 `user_uuid + device_id` 建立新连接时，Connect 会用新连接替换旧连接。前端重连时不需要先主动关闭旧连接，但应避免多处代码并发创建同一设备连接。

## 3. 帧格式

### 3.1 只接受 Binary Frame

当前 Connect 入站协议只接受 WebSocket Binary Frame。Text Frame 会返回 `error` 帧，错误码为 `17003`。

旧的 JSON / Text Frame 兼容实现已移除，当前服务端只保留 Binary Protobuf 一条协议主路径。

### 3.2 MessageEnvelope

所有上行和下行二进制帧外层均使用 `proto/connect/connect.proto` 中的 `MessageEnvelope`：

| 字段 | Protobuf 类型 | 方向 | 说明 |
| --- | --- | --- | --- |
| `type` | string | 上行/下行 | 消息类型路由键，例如 `MSG_PUSH`、`MSG_ACK`、`heartbeat`。 |
| `data` | bytes | 上行/下行 | 业务 payload，按 `type` 再解析为具体 Protobuf。 |
| `seq` | int64 | 下行为主 | 会话内序号或事件顺序键。 |
| `server_ts` | int64 | 下行 | 服务端时间，Unix 毫秒。 |
| `trace_id` | string | 下行 | 链路追踪 ID。 |
| `ack_required` | bool | 下行 | 是否要求客户端发送 `MSG_ACK`。 |

Protobuf 定义摘要：

```proto
message MessageEnvelope {
  string type = 1;
  bytes data = 2;
  int64 seq = 3;
  int64 server_ts = 4;
  string trace_id = 5;
  bool ack_required = 6;
}
```

### 3.3 前端序列化要求

前端需要使用与后端一致的 Protobuf schema：

| 文件 | 用途 |
| --- | --- |
| `proto/connect/connect.proto` | `MessageEnvelope` 外层包。 |
| `proto/connect/ws_control.proto` | ACK 与错误帧 payload。 |
| `proto/msg/msg_common.proto` | 新消息 `MsgItem`。 |
| `proto/msg/msg_push_event.proto` | 撤回、已读等通知 payload。 |
| `proto/realtime/realtime_event.proto` | 好友、关系、群申请、群状态等实时提醒 payload。 |

前端伪代码：

```ts
const envelope = MessageEnvelope.encode({
  type: 'heartbeat',
  data: new Uint8Array(),
  seq: 0
}).finish();

socket.send(envelope);
```

## 4. 上行帧

### 4.1 heartbeat

客户端用于保持连接和刷新在线路由活跃时间（presence 契约：服务端对每个心跳无条件刷新路由，建议间隔约 30 秒，超过在线判定窗口 120 秒会被判离线）。

| 字段 | 值 |
| --- | --- |
| 外层 `type` | `heartbeat` |
| 外层 `data` | 空 bytes |
| WebSocket 帧 | Binary |
| 服务端响应 | `heartbeat_ack` |

服务端收到后会刷新连接生命周期，并返回：

| 字段 | 值 |
| --- | --- |
| 外层 `type` | `heartbeat_ack` |
| 外层 `data` | 空 bytes |
| 外层 `seq` | `0` |

建议前端心跳间隔控制在 20-60 秒。心跳失败或超过 2 个心跳周期未收到服务端响应时，应触发重连。

### 4.2 MSG_ACK

客户端收到需要回执的下行消息后，上行 `MSG_ACK` 表示该设备已处理到指定会话的最大连续 `seq`。

| 字段 | 值 |
| --- | --- |
| 外层 `type` | `MSG_ACK`，兼容小写 `msg_ack` |
| 外层 `data` | `MessageAck` Protobuf bytes |
| 服务端响应 | `MSG_ACK_ACK` |

`MessageAck` 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `conv_id` | string | 是 | 会话 ID。 |
| `seq` | int64 | 是 | 当前设备已处理的最大连续 seq，必须 > 0。 |
| `msg_id` | string | 否 | 消息 ID，仅用于调试和观测。 |

Protobuf 定义：

```proto
message MessageAck {
  string conv_id = 1;
  int64 seq = 2;
  string msg_id = 3;
}
```

服务端校验规则：

1. `conv_id` 必须非空。
2. `seq` 必须大于 0。
3. `seq` 不能超过该连接已成功下发的最大 seq，否则返回 `17003`。
4. Redis 使用 Lua 脚本单调合并，只保存更大的 seq，不会被旧 ACK 覆盖回退。
5. ACK 位点 key 为 `msg:ack:{user_uuid}:{device_id}:{conv_id}`，TTL 为 30 天。

### 4.3 message

当前服务端对 `type=message` 仅返回 `message_ack`，代码中标注后续接入消息路由与持久化。现阶段前端发送业务消息应走 HTTP：`POST /api/v1/auth/messages/send`。

## 5. 下行帧

### 5.1 heartbeat_ack

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `type` | string | 固定为 `heartbeat_ack`。 |
| `data` | bytes | 空。 |
| `seq` | int64 | 0。 |

前端收到后可更新连接健康状态和最后活跃时间。

### 5.2 MSG_PUSH

新消息下行。外层 `data` 是 `msg.MsgItem` Protobuf bytes。

| 外层字段 | 说明 |
| --- | --- |
| `type` | `MSG_PUSH`。 |
| `seq` | 消息会话内 seq。 |
| `server_ts` | 服务端下发时间，Unix 毫秒。 |
| `ack_required` | 若为 true，客户端必须发送 `MSG_ACK`。 |

`MsgItem` 关键字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `msg_id` | string | 服务端消息 ID。 |
| `client_msg_id` | string | 客户端幂等 ID。 |
| `conv_id` | string | 会话 ID。 |
| `seq` | int64 | 会话内序号。 |
| `from_uuid` | string | 发送者 UUID。 |
| `msg_type` | int32 | 消息类型。 |
| `content` | string | JSON 字符串内容。 |
| `status` | int32 | 0 正常，1 撤回，2 删除。 |
| `send_time` | int64 | 发送时间，Unix 毫秒。 |
| `reply_to_msg_id` | string | 回复目标消息 ID。 |
| `at_users` | string[] | 被 @ 用户 UUID。 |

前端处理建议：

1. 先按 `conv_id + seq` 去重。
2. 若 `seq` 等于本地最大 seq + 1，直接插入。
3. 若 `seq` 大于本地最大 seq + 1，先插入或暂存，再调用消息拉取接口补齐缺口。
4. 若 `ack_required=true`，渲染或持久化成功后发送 `MSG_ACK`。

### 5.3 MSG_RECALL

消息撤回通知。外层 `data` 是 `msg.RecallNotice` Protobuf bytes。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `conv_id` | string | 会话 ID。 |
| `msg_id` | string | 被撤回消息 ID。 |
| `operator` | string | 撤回操作者 UUID。 |
| `recall_time` | int64 | 撤回时间，Unix 毫秒。 |

前端收到后应把对应消息气泡替换为撤回样式，不应继续展示原 `content`。

### 5.4 MSG_MARK_READ

已读多端同步通知。外层 `data` 是 `msg.MarkReadNotice` Protobuf bytes。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `conv_id` | string | 会话 ID。 |
| `read_seq` | int64 | 已读到的最大 seq。 |

前端收到后应更新同一账号其他设备上的未读数和红点状态。

### 5.5 MSG_READ_RECEIPT

对端已读回执。当前 `message-push` 支持该类型下发，前端可按业务约定解析 payload 并更新发送方消息的已读展示。若当前客户端版本未实现，可先忽略未知 payload，但必须保留不崩溃的兜底逻辑。

### 5.6 非消息类实时提醒

非消息类提醒与消息类下行共用 `MessageEnvelope` 外层。外层 `type` 使用业务事件类型，外层 `data` 是 `proto/realtime/realtime_event.proto` 中对应 payload 的 Protobuf bytes。

| 外层 `type` | data payload | 前端建议动作 |
| --- | --- | --- |
| `FRIEND_APPLY_CREATED` | `FriendApplyCreatedPayload` | 刷新收到的好友申请和未读提示。 |
| `FRIEND_APPLY_HANDLED` | `FriendApplyHandledPayload` | 刷新发出的申请状态。 |
| `FRIEND_RELATION_CHANGED` | `FriendRelationChangedPayload` | 刷新好友、备注、标签或黑名单视图。 |
| `GROUP_JOIN_REQUEST_CREATED` | `GroupJoinRequestCreatedPayload` | 管理端刷新入群申请列表和未处理数。 |
| `GROUP_JOIN_REQUEST_REVIEWED` | `GroupJoinRequestReviewedPayload` | 申请人刷新入群申请状态。 |
| `GROUP_MEMBER_REMOVED` | `GroupMemberRemovedPayload` | 目标用户清理本地群状态。 |
| `GROUP_DISMISSED` | `GroupDismissedPayload` | 群成员清理本地群状态。 |
| `GROUP_MEMBER_MUTED` | `GroupMemberMutedPayload` | 目标成员刷新禁言状态。 |
| `GROUP_STATE_CHANGED` | `GroupStateChangedPayload` | 按 `changed` 字段刷新群资料、公告、成员、角色或禁言状态。 |

客户端应对未知 `type` 做兼容忽略；需要最新权威状态时回源 HTTP/gRPC 对应接口刷新。

### 5.7 MSG_ACK_ACK

服务端确认客户端 ACK 已处理。外层 `data` 是 `MessageAckAck` Protobuf bytes。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `conv_id` | string | 会话 ID。 |
| `seq` | int64 | Redis 单调合并后保存的最大 ACK seq。 |

前端可用该帧清理本地 ACK 重试队列。

### 5.8 error

协议层错误帧。外层 `data` 是 `ErrorFrame` Protobuf bytes。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | int32 | 错误码。 |
| `message` | string | 错误文案。 |

常见错误：

| code | 含义 | 触发场景 |
| --- | --- | --- |
| `17003` | WebSocket 上行消息格式错误 | 发送 Text Frame、Protobuf 解析失败、ACK 字段非法、ACK seq 超过已下发位点。 |
| `17004` | WebSocket 上行消息类型不支持 | `type` 不在服务端支持列表中。 |
| `30002` | 服务暂不可用 | ACK 位点写入 Redis 失败等可恢复基础设施异常。 |

## 6. 连接生命周期

### 6.1 建连成功

建连成功后 Connect 会：

1. 将连接注册到本节点连接管理器。
2. 若同设备已有旧连接，则关闭旧连接。
3. 写入在线路由 `user:routing:{user_uuid}`，field 为 `device_id`。
4. 异步同步设备在线状态到 auth/device 服务。

### 6.2 心跳刷新

每个心跳都会无条件刷新在线路由活跃时间和 TTL（默认 360 秒）。在线路由 value 格式为：

```text
connectGrpcAddr|lastActiveMs
```

默认路由 TTL 为 180 秒。前端应保持心跳频率低于 TTL，避免服务端误判离线。

### 6.3 断开连接

连接断开后 Connect 会移除本设备路由，并异步更新设备离线状态。前端应根据关闭原因执行指数退避重连。

## 7. 前端推荐实现

### 7.1 状态机

建议前端维护以下连接状态：

| 状态 | 说明 |
| --- | --- |
| `idle` | 未连接。 |
| `connecting` | 正在握手。 |
| `connected` | 已升级并正常心跳。 |
| `reconnecting` | 断线重连中。 |
| `auth_failed` | 鉴权失败，需要重新登录或刷新 Token。 |

### 7.2 重连策略

1. 网络断开、异常关闭：指数退避重连，例如 1s、2s、4s、8s、最高 30s。
2. HTTP 401 或握手 `code=20002`：先尝试刷新 Token；刷新失败则跳转登录。
3. HTTP 400 且 `code=17001/17002`：前端参数错误，修复参数后再连接。
4. 页面进入后台可继续保持连接；若平台限制后台连接，应在恢复前台后重新连接并补拉消息。

### 7.3 补拉策略

WebSocket 只保证在线实时投递，不替代历史拉取。前端应在以下场景调用 HTTP 拉取接口：

| 场景 | 动作 |
| --- | --- |
| 登录后首次打开会话页 | 调用 `/api/v1/auth/messages/pull` 拉最近消息。 |
| 登录恢复或 WebSocket 重连成功 | 调用 `/api/v1/auth/messages/sync-batch`，提交多个已有本地缓存会话各自的 `afterSeq`。 |
| 收到 `MSG_PUSH` 发现 seq gap | 按缺口范围补拉。 |
| ACK 写入失败或服务端返回 `30002` | 保留本地 ACK 队列，稍后重试或等待下次连接后重新上报。 |

批量同步只用于按已有位点向后追新。没有本地位点的新会话仍应在首次打开时调用单会话 `/messages/pull` 并使用 `direction=2` 拉最近一页。批量结果中单个会话失败不会导致整批失败，客户端必须检查每个 `results[].errorCode`，并且只在消息成功持久化后推进该会话的 `nextSeq`。

### 7.4 ACK 策略

1. 仅对 `ack_required=true` 的下行消息发送 ACK。
2. ACK 的 `seq` 应为当前设备已连续处理的最大 seq，不是“刚收到的最大 seq”。
3. 若消息列表存在缺口，不要越过缺口 ACK。
4. 收到 `MSG_ACK_ACK` 后，以返回的 `seq` 更新本地设备 ACK 位点。

## 8. 常见错误码

| code | 含义 | 前端建议 |
| --- | --- | --- |
| `17001` | 握手缺少 token | 检查连接 URL。 |
| `17002` | 握手缺少 device_id | 检查设备 ID 初始化。 |
| `17003` | 上行消息格式错误 | 确认使用 Binary Frame 和正确 Protobuf schema。 |
| `17004` | 上行消息类型不支持 | 检查 `MessageEnvelope.type`。 |
| `20002` | Token 无效 | 刷新 Token 或重新登录。 |
| `30002` | 服务暂不可用 | 稍后重试 ACK 或重连。 |

## 9. 与 HTTP 接口的关系

| 能力 | 推荐通道 | 说明 |
| --- | --- | --- |
| 发送消息 | HTTP `POST /api/v1/auth/messages/send` | 当前 WebSocket `message` 只返回占位 ACK，不做持久化。 |
| 拉历史消息 | HTTP `GET /api/v1/auth/messages/pull` | WebSocket 不负责历史补齐。 |
| 多会话重连追新 | HTTP `POST /api/v1/auth/messages/sync-batch` | 各会话提交独立 `afterSeq`，用于登录恢复和重连批量追平。 |
| 新消息实时到达 | WebSocket `MSG_PUSH` | 由 Kafka `msg.push` 经 message-push 转发到 Connect。 |
| 撤回通知 | WebSocket `MSG_RECALL` | HTTP 撤回成功后异步下发。 |
| 已读同步 | WebSocket `MSG_MARK_READ` | HTTP 标记已读成功后异步下发。 |
| 在线状态 | HTTP 设备接口 + WebSocket 生命周期 | 连接生命周期会同步设备在线状态。 |
