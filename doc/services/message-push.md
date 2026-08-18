# message-push 服务

message-push 是消息下行桥接服务，负责消费 Kafka `msg.push` 与 `realtime.push` 事件，查询在线路由，并通过 connect gRPC 将消息投递到在线设备。

## 职责

- 通过两个独立 `ManualConsumerPool` 消费 `msg.push` 与 `realtime.push` Kafka Topic。
- 解析 `MsgPushEvent`，支持新消息、撤回、已读同步、已读回执等下行类型。
- 根据会话类型选择扩散策略：单聊直接找接收者，群聊拆分群成员。
- 查询 Redis 在线路由 `user:routing:{user_uuid}`，定位用户在线设备所在 Connect 节点。
- 对 `msg.push` 按 Connect 节点有界并发扇出，同一节点内按用户串行处理。
- `msg.push` 的完整用户目标通过 `PushToUser` 批量投递；需要排除当前设备时通过 `PushToDevice` 精确投递。
- 处理发送方多端同步：向发送者除当前设备外的其他在线设备投递一份。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/message-push/cmd` | 服务启动、两个 Kafka Pool、Connect gRPC 客户端装配。 |
| `apps/message-push/internal/consumer` | Kafka Pool 壳与 best-effort 有限重试适配；不自行创建 Kafka Reader。 |
| `apps/message-push/internal/msgpush` | `msg.push` 下行 Handler：事件解析、路由策略、节点扇出。 |
| `apps/message-push/internal/realtime` | `realtime.push` 实时提醒 Handler：目标解析与设备投递。 |
| `apps/message-push/internal/pusherr` | 可重试错误哨兵 `ErrRetriable`，供 consumer/msgpush/realtime 共用。 |
| `pkg/kafka` | 独立 Reader 的 Fetch/Handle/Commit 循环、退避和 Pool fail-fast 编排。 |
| `pkg/presence` | Redis 在线路由查询（`user:routing:{user_uuid}`）。 |
| `proto/msg/msg_push_event.proto` | `MsgPushEvent`、撤回和已读通知契约。 |
| `proto/connect/connect.proto` | Connect gRPC 和 WebSocket Envelope 契约。 |

## 输入与输出

| 类型 | 来源/目标 | 说明 |
| --- | --- | --- |
| 输入 Topic | Kafka `msg.push` | msg-service 写入的下行事件。 |
| 输入 Topic | Kafka `realtime.push` | relation/group 写入的非消息实时提醒。 |
| 路由数据 | Redis `user:routing:{user_uuid}` | Connect 写入的在线设备路由。 |
| 群成员 | group 服务 gRPC | 群聊扩散需要拆分成员。 |
| 输出 gRPC | connect `PushToDevice` / `PushToUser` | 按设备精确投递或按节点内用户批量投递。 |
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
3. 对端属于完整用户目标，按 `connectGrpcAddr` 分组后在各节点调用一次 `PushToUser`。
4. `MSG_PUSH` / `MSG_RECALL` 对发送者执行多端同步，排除当前发送设备并逐设备调用 `PushToDevice`。

### 群聊

1. `receiver_uuid` 表示群 UUID。
2. message-push 查询群成员列表。
3. 根据事件类型决定是否排除发送者，以及是否投递给发送者其他设备。
4. 对每个成员查询在线路由并去重 `(user_uuid, device_id)`。
5. 按 Connect 节点有界并发；同节点同成员通过 `PushToUser` 批量投递。
6. 发送者其他设备存在排除条件，继续通过 `PushToDevice` 精确投递。

## 扇出并发与结果判定

- `KAFKA_MSG_PUSH_CONSUMER_CONCURRENCY` 和 `KAFKA_REALTIME_PUSH_CONSUMER_CONCURRENCY` 分别控制本进程两个 Pool 的独立 Reader 数；默认都是 3，显式值必须为 `1～64`，非法配置导致启动失败。
- workers 不是 partition 绑定。两个 topic 使用不同 consumer group，Kafka rebalance 自动分配 partition；同一 Reader 严格 `Fetch → Handle → Commit`，不同 Reader 并行。worker 多于 partition 或暂时分不到 partition 时保持 idle。
- 节点并发上限由 `MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY` 配置；未配置时默认 32，显式配置必须是正整数，否则服务初始化失败。节点少于上限时按实际节点数并发。
- Kafka Reader 并发和单条事件内的 Connect 节点扇出是两层独立限制，不能用后者代替前者。
- 同一节点内不对每台设备各开 goroutine，而是按用户串行调用，避免打满复用的 gRPC 连接。
- `msgpush.Handler`（`msg.push` 处理器）强制要求 sender 同时实现 `PushToUser` 和 `PushToDevice`；完整用户目标固定使用前者，不支持回退到旧的逐设备发送路径。
- `PushToUser` 返回成功入队的设备数：零投递计为失败，小于路由快照设备数计为部分成功。
- 所有已尝试设备都失败时返回可重试错误；部分成功时整体返回成功，避免重推已经成功的设备。

## 错误处理

| 错误 | 策略 | 说明 |
| --- | --- | --- |
| Kafka Fetch 临时失败 | Pool 内退避重试 | Broker 抖动不导致进程退出。 |
| Kafka payload 反序列化失败 | 永久错误，提交跳过 | 事件格式非法，重试无意义。 |
| 必填字段缺失 | 永久错误，提交跳过 | 防止毒消息阻塞消费。 |
| 不支持的事件类型 | 永久错误，提交跳过 | 当前进程无法处理，客户端仍可补拉事实。 |
| Redis 路由临时失败 | 可重试 | 避免短暂基础设施异常导致消息丢失。 |
| Connect gRPC 临时失败 | 本地有限重试 | 当前实现本地最多重试 3 次；**不**对 connect 配置 gRPC ServiceConfig 自动重试，避免与 Kafka/本地重试叠加。 |
| 用户无有效在线路由 | 正常跳过 | 离线用户通过 HTTP 拉取自愈。 |
| 路由存在但 connect 无对应连接 | 按投递失败统计 | 全部已尝试设备失败时进入本地有限重试。 |
| 本地重试耗尽 | 告警、计数并提交丢弃 | message-push 是 best-effort 下行，客户端按 seq 补拉，不写 `dead_events`。 |
| 任一 Pool worker 致命退出 | 取消同 Pool 兄弟并退出进程 | 消费是本服务主业，禁止半残并发。 |

## 可用性边界

- message-push 只负责在线实时投递，不保存离线消息。
- 离线消息和缺口补齐由客户端调用 msg HTTP/gRPC 拉取。
- Connect 节点不可达时，当前在线投递可能失败；客户端重连后应按 seq 补拉。
- Pool 致命失败会以非零状态退出；Kafka 暂时不可用只在 Reader 内持续退避，不触发重启雪崩。

## 不变量

- message-push 不写消息事实表。
- message-push 不判断消息发送权限，权限在 msg-service 发送前校验。
- message-push 不直接管理 WebSocket 连接，只调用 connect gRPC。
- 群聊投递必须对 `(user_uuid, device_id)` 去重，避免重复下发。
