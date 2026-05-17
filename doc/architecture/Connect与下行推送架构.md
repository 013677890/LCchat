# Connect 与下行推送架构

Connect 和 message-push 共同组成消息下行链路。msg 负责产生 `msg.push` 事件，message-push 负责扩散和路由，connect 负责本节点 WebSocket 投递。

## 职责拆分

| 组件 | 职责 | 不负责 |
| --- | --- | --- |
| msg | 消息落库、会话更新、产生下行事件 | 不维护在线连接，不直接推 WebSocket。 |
| message-push | 消费 `msg.push`、查询群成员、查询在线路由、调用 connect | 不写消息库，不做发送权限判定。 |
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

## message-push 处理流程

1. 从 Kafka `msg.push` 读取 `MsgPushEvent`。
2. 反序列化失败、类型不支持、必要字段缺失时按永久错误跳过。
3. 根据事件类型和会话类型选择目标：
   - P2P：读取 `receiver_uuid` 在线路由。
   - 群聊：调用 group 获取成员，排除发送者，再批量读取路由。
   - 多端同步：读取 `from_uuid` 在线路由并排除原发送设备。
4. 对 `(user_uuid, device_id)` 去重。
5. 组装 connect `MessageEnvelope`。
6. 对每个设备调用目标 connect 节点 `PushToDevice`。
7. 可重试错误最多本地重试 3 次，超过后记录告警并按当前策略推进消费。

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

- 无路由：按离线处理，不报错；客户端下一次 PullMessages 自愈。
- connect 瞬时不可达：message-push 短重试，仍失败则记录告警。
- WebSocket 写失败：connect 关闭或清理连接，后续路由会过期或断连清理。
- ACK 写 Redis 失败：返回错误帧或记录失败，不影响消息权威事实。
