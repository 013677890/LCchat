# Connect 与下行推送架构

Connect 和 message-push 共同组成实时下行链路。msg 负责产生 `msg.push` 消息类事件，relation/group 等业务服务负责产生 `realtime.push` 非消息类提醒事件，message-push 负责扩散和路由，connect 负责本节点 WebSocket 投递。

## 职责拆分

| 组件 | 职责 | 不负责 |
| --- | --- | --- |
| msg | 消息落库、会话更新、产生下行事件 | 不维护在线连接，不直接推 WebSocket。 |
| message-push | 消费 `msg.push` / `realtime.push`、查询群成员或管理员、查询在线路由、调用 connect | 不写业务库，不做发送权限判定。 |
| connect | WebSocket 握手、连接管理、路由写入、gRPC 投递、ACK 位点 | 不消费 Kafka，不判断业务权限。 |

## 在线路由

connect 在连接建立和心跳时写入 Redis：

| Key | 结构 | 说明 |
| --- | --- | --- |
| `user:routing:{user_uuid}` | Hash，field=`device_id`，value=`connectGrpcAddr|lastActiveMs` | 表示用户设备当前所在 connect 节点。 |

默认路由 TTL 为 180 秒。message-push 读取路由时会按最近活跃时间过滤过期设备，即使 Redis 延迟清理也不会向长时间离线旧节点投递。

## 下行事件类型

| 类型 | 来源 | 投递规则 | ACK |
| --- | --- | --- | --- |
| `MSG_PUSH` | 发送消息 | P2P 投给接收方；群聊查询群成员后扩散；发送方其他设备同步 | seq 大于 0 时需要 ACK。 |
| `MSG_RECALL` | 撤回消息 | P2P 或群聊目标设备；发送方其他设备同步 | 通常不要求 ACK。 |
| `MSG_MARK_READ` | 标记已读 | 只同步发起用户其他设备 | 不要求 ACK。 |
| `MSG_READ_RECEIPT` | P2P 已读回执 | 只投递给对端在线设备 | 不要求 ACK。 |

非消息类提醒统一走 `realtime.push`，最终也封装成 `MessageEnvelope`：

| 类型 | 来源 | 投递规则 | ACK |
| --- | --- | --- | --- |
| `FRIEND_APPLY_CREATED` | relation | 投给好友申请目标用户 | 默认不要求。 |
| `FRIEND_APPLY_HANDLED` | relation | 投给申请发起人 | 默认不要求。 |
| `FRIEND_RELATION_CHANGED` | relation | 投给单用户或多用户列表 | 默认不要求。 |
| `GROUP_JOIN_REQUEST_CREATED` | group | 投给群主和管理员 | 默认不要求。 |
| `GROUP_JOIN_REQUEST_REVIEWED` | group | 投给入群申请人 | 默认不要求。 |
| `GROUP_STATE_CHANGED` 等群状态类 | group | 投给群成员或指定用户 | 默认不要求。 |

## message-push 处理流程

1. 从 Kafka `msg.push` 读取 `MsgPushEvent`。
2. 反序列化失败、类型不支持、必要字段缺失时按永久错误跳过。
3. 根据事件类型和会话类型选择目标：
   - P2P：读取 `receiver_uuid` 在线路由。
   - 群聊：调用 group 获取成员，排除发送者，再批量读取路由。
   - 消息/撤回多端同步：读取 `from_uuid` 在线路由并排除原发送设备。
   - 已读同步：读取 `receiver_uuid` 在线路由并排除已读发起设备。
   - 已读回执：读取 `receiver_uuid` 在线路由，不做设备排除。
4. 对 `(user_uuid, device_id)` 去重，并保留目标是否覆盖某用户在节点上全部设备的语义。
5. 组装 connect `MessageEnvelope`。
6. 按 `connectGrpcAddr` 分组，每个 connect 节点作为独立执行单元。
7. 不同节点有界并发执行；同一节点内按用户串行处理：
   - 接收方、群成员、已读回执接收方等完整用户目标，调用一次 `PushToUser`，由 connect 投递到该用户在本节点的全部在线设备。
   - 发送方其他设备、已读同步等需要排除当前设备的目标，逐设备调用 `PushToDevice`。
8. 等待所有已启动节点结束，按设备数汇总成功与失败，并在最终日志中记录各节点地址、目标设备数、成功数和失败数。
9. 所有已尝试设备都失败时返回可重试错误；部分成功仍视为成功。可重试错误最多本地重试 3 次，超过后记录告警并按当前策略推进消费。

节点并发上限由 `MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY` 配置；未配置时默认 32，显式配置必须是正整数，否则 message-push 初始化失败。实际并发数为配置值与目标 connect 节点数的较小值。信号量在启动节点 goroutine 前获取，避免节点数异常增大时一次性创建无界协程；处理预算到期或关停时停止调度新节点，并等待已启动节点收敛。

`EventHandler` 的 sender 契约同时要求 `PushToUser` 和 `PushToDevice`，不会探测能力或回退到旧的逐设备实现。`PushToUser` 只返回 `DeliveredCount`，不返回设备明细。message-push 将零投递记为失败，将小于路由快照设备数的返回记为部分成功；新增的 `PushToUserTotal` 和 `PushToUserDuration` 指标使用 `success`、`partial`、`error` 区分结果，原有 `PushToDevice` 指标保持设备调用粒度。

`realtime.push` 的目标解析流程类似，但 Kafka value 是 `RealtimePushEvent`：message-push 先按 `target.kind` 解析 `user`、`device`、`user_list`、`group_members`、`group_admins`，再批量查询在线路由，最后复用同一个 `MessageEnvelope` 下发。当前节点级有界并发和 `PushToUser` 混合策略只应用于 `msg.push` 的 `EventHandler`；`realtime.push` 仍按设备调用 `PushToDevice`。connect 不感知 payload 的业务含义。

## connect 连接生命周期

| 阶段 | 行为 |
| --- | --- |
| 握手 | 校验 token、`device_id`、JWT claims，并从 Redis 校验 AccessToken 哈希。 |
| OnConnect | 刷新设备活跃时间，写入在线路由，异步通知 auth 设备在线。 |
| OnHeartbeat | 节流刷新活跃时间和在线路由 TTL。 |
| OnDisconnect | 清理本地节流缓存，删除在线路由，异步通知 auth 设备离线。 |
| 节点退出 | 按 `CONNECT_SELF_GRPC_ADDR` 扫描并清理当前节点残留路由。 |

## ACK 位点

客户端收到 `ack_required=true` 的下行帧后发送 `MSG_ACK`。connect 将用户、设备、会话维度最大连续 ACK seq 写入 Redis：

- Key：`msg:ack:{user_uuid}:{device_id}:{conv_id}`。
- TTL：30 天。
- 写入策略：Lua 脚本单调合并，只允许位点前进，不允许旧 ACK 覆盖新位点。
- 服务端确认帧：`MSG_ACK_ACK`。

ACK 当前用于短期可靠同步和排障观测，长期权威已读仍以 msg 的会话 read_seq 为准。

## 失败与自愈

- 无路由：按离线处理，不报错；客户端下一次单会话 PullMessages 或多会话 BatchSyncMessages 自愈。
- 单个 connect 节点瞬时不可达：不阻塞其他节点完成投递；只要存在成功设备，整体不重试。
- 所有已尝试设备投递失败：message-push 短重试，仍失败则记录告警。
- WebSocket 写失败：connect 关闭或清理连接，后续路由会过期或断连清理。
- ACK 写 Redis 失败：返回错误帧或记录失败，不影响消息权威事实。
