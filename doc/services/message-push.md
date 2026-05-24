# message-push 服务

message-push 是消息下行桥接服务，负责消费 Kafka `msg.push` 事件，查询在线路由，并通过 connect gRPC 将消息投递到在线设备。

## 职责

- 消费 `msg.push` Kafka Topic。
- 解析 `MsgPushEvent`，支持新消息、撤回、已读同步、已读回执等下行类型。
- 根据会话类型选择扩散策略：单聊直接找接收者，群聊拆分群成员。
- 查询 Redis 在线路由 `user:routing:{user_uuid}`，定位用户在线设备所在 Connect 节点。
- 通过 Connect gRPC `PushToDevice` / `PushToUser` 投递 WebSocket 下行帧。
- 处理发送方多端同步：向发送者除当前设备外的其他在线设备投递一份。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/message-push/cmd` | 服务启动、Kafka Consumer、Connect gRPC 客户端装配。 |
| `apps/message-push/internal/consumer` | Kafka 消费、事件解析、下行分发和重试。 |
| `apps/message-push/internal/route` | Redis 在线路由查询。 |
| `proto/msg/msg_push_event.proto` | `MsgPushEvent`、撤回和已读通知契约。 |
| `proto/connect/connect.proto` | Connect gRPC 和 WebSocket Envelope 契约。 |

## 输入与输出

| 类型 | 来源/目标 | 说明 |
| --- | --- | --- |
| 输入 Topic | Kafka `msg.push` | msg-service 写入的下行事件。 |
| 路由数据 | Redis `user:routing:{user_uuid}` | Connect 写入的在线设备路由。 |
| 群成员 | group 服务或群成员缓存 | 群聊扩散需要拆分成员。 |
| 输出 gRPC | connect `PushToDevice` / `PushToUser` | 精确投递到在线设备。 |
| 输出 WebSocket | connect 转发给客户端 | 客户端收到 `MSG_PUSH`、`MSG_RECALL` 等帧。 |

## 路由格式

Redis Hash：

```text
key   = user:routing:{user_uuid}
field = {device_id}
value = {connectGrpcAddr}|{lastActiveMs}
```

示例：

```text
HGETALL user:routing:10000000000000000001
web-chrome-001 => connect:9091|1710000000123
```

## 支持的事件类型

| type | payload | 投递目标 | 客户端动作 |
| --- | --- | --- | --- |
| `MSG_PUSH` | `msg.MsgItem` | 接收者设备；发送方其他设备 | 插入新消息、更新会话。 |
| `MSG_RECALL` | `msg.RecallNotice` | 会话内在线设备 | 替换为撤回气泡。 |
| `MSG_MARK_READ` | `msg.MarkReadNotice` | 当前账号在线设备 | 清理未读红点。 |
| `MSG_READ_RECEIPT` | `msg.MarkReadNotice` | 对端在线设备 | 更新已读展示。 |

## 分发规则

### 单聊

1. `receiver_uuid` 表示对端用户 UUID。
2. message-push 查询对端在线路由。
3. 对每个在线设备调用对应 connect 节点投递。
4. `MSG_PUSH` / `MSG_RECALL` 对发送者执行多端同步，排除当前发送设备。

### 群聊

1. `receiver_uuid` 表示群 UUID。
2. message-push 查询群成员列表。
3. 根据事件类型决定是否排除发送者，以及是否投递给发送者其他设备。
4. 对每个成员查询在线路由并去重 `(user_uuid, device_id)`。
5. 按 Connect 节点发起 gRPC 投递。

## 错误处理

| 错误 | 策略 | 说明 |
| --- | --- | --- |
| Kafka payload 反序列化失败 | 永久错误，跳过 | 事件格式非法，重试无意义。 |
| 必填字段缺失 | 永久错误，跳过 | 防止毒消息阻塞消费。 |
| 不支持的事件类型 | 永久错误，跳过 | 保持兼容，等待代码支持后再投递。 |
| Redis 路由临时失败 | 可重试 | 避免短暂基础设施异常导致消息丢失。 |
| Connect gRPC 临时失败 | 本地有限重试 | 当前实现本地最多重试 3 次。 |
| 用户不在线 | 正常跳过 | 离线用户通过 HTTP 拉取自愈。 |

## 可用性边界

- message-push 只负责在线实时投递，不保存离线消息。
- 离线消息和缺口补齐由客户端调用 msg HTTP/gRPC 拉取。
- Connect 节点不可达时，当前在线投递可能失败；客户端重连后应按 seq 补拉。

## 不变量

- message-push 不写消息事实表。
- message-push 不判断消息发送权限，权限在 msg-service 发送前校验。
- message-push 不直接管理 WebSocket 连接，只调用 connect gRPC。
- 群聊投递必须对 `(user_uuid, device_id)` 去重，避免重复下发。
