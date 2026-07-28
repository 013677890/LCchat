# msg 服务

msg 服务拥有消息、会话和已读位点事实，负责消息落库、会话维护、消息拉取、撤回，以及将下行事件可靠写入 `outbox_events`。

## 职责

- 单聊和群聊统一发送入口，分配消息 ID 和会话内递增 `seq`。
- 基于 `(from_uuid, device_id, client_msg_id)` 做发送幂等，避免弱网重试重复落库。
- 拉取单会话历史消息、按多个会话各自的 seq 位点批量追新、按消息 ID 批量反查、撤回消息。
- 维护会话列表、最后消息预览、未读数、免打扰、置顶和逻辑删除位点。
- 独立消费 `group.cache`，把群成员关系和群状态投影到 msg 自己的会话表。
- 标记会话已读，并与多端同步、P2P 已读回执 outbox 事件同事务提交。
- 将新消息、撤回、已读事件写入 MySQL `outbox_events`，由 Debezium CDC 路由到 Kafka `msg.push`，再由 message-push 做下行投递。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/msg/cmd` | 服务启动、gRPC 服务、依赖注入。 |
| `apps/msg/internal/handler` | gRPC handler，薄适配层。 |
| `apps/msg/internal/domain/message` | 消息领域：消息落库、seq 分配、撤回、拉取。 |
| `apps/msg/internal/domain/conversation` | 会话领域：会话列表、已读、删除、设置。 |
| `apps/msg/internal/consumer` | `group.cache` 群成员会话投影消费者。 |
| `apps/msg/internal/usecase` | 跨领域工作流：发送、消息读取/批量同步、撤回、已读。 |
| `apps/msg/internal/groupcli` | 群权限校验客户端。 |
| `proto/msg` | MsgService、MsgItem、MsgPushEvent 契约。 |

## 分层规则

| 层 | 规则 |
| --- | --- |
| handler | 只做参数转换、鉴权主体解析和错误映射，不写业务规则。 |
| message domain | 只处理消息事实，不依赖 conversation domain。 |
| conversation domain | 只处理会话事实，不依赖 message domain。 |
| usecase | 唯一允许协调多个 domain；不直接投递 `msg.push` Kafka。 |
| repository | 负责 MySQL/Redis 访问，不做跨领域编排。 |

## 数据所有权

| 数据 | 存储 | 说明 |
| --- | --- | --- |
| 消息 | MySQL 消息表 | 消息内容、状态、发送者、seq。 |
| 会话 | MySQL 会话表 | 会话归属、未读、置顶、免打扰、清理位点。 |
| 群成员会话投影 | MySQL `conversation.membership_*` | group 成员事实的可重建本地投影，不是成员权威表。 |
| 群共享会话投影 | MySQL `group_conversation` | 群最大 seq、最后消息、群状态和已消费投影版本。 |
| 会话 seq | Redis `msg:seq:{conv_id}` | 使用 `INCR` 分配会话内递增序号。 |
| 消息幂等 | Redis `msg:idempotent:{from_uuid}:{device_id}:{client_msg_id}` | TTL 10 分钟，保存首次发送结果。 |
| 消息下行 Outbox | MySQL `outbox_events` | 新消息、撤回、已读与 `msg.push` 事件同事务落库。 |

## `msg.push` 事件契约

- `msg.push` Kafka key 使用 `conv_id`，保证同一会话内事件尽量落到同一分区。
- Kafka value 是严格 `protojson` 编码的 `msg.MsgPushEvent`，并且必须包含 `event_id`。
- 消费端不兼容旧版 Protobuf bytes；未知字段、缺少 `event_id` 或非 JSON payload 都会按永久错误跳过。
- `MsgPushEvent.data` 仍按 `type` 存放对应业务 Protobuf bytes，例如 `MsgItem`、`RecallNotice`、`MarkReadNotice`。

## 暴露的 gRPC 服务

| 服务 | 能力 |
| --- | --- |
| `MsgService` | 发送消息、单会话拉取、多会话位点同步、消息反查、撤回、会话列表、已读、删除会话、更新设置。 |

关键 RPC：

- `SendMessage`：单聊/群聊统一发送入口。
- `PullMessages`：按 `conv_id + anchor_seq + direction` 拉取历史。
- `BatchSyncMessages`：按最多 50 个会话各自的 `after_seq` 有界并发追新；结果保持请求顺序，并用逐项 `error_code` 表达部分失败。
- `GetMessagesByIds`：按消息 ID 批量反查。
- `RecallMessage`：撤回消息。
- `GetConversations`：全量或增量获取会话列表。
- `MarkRead`：更新已读位点和未读数。
- `DeleteConversation`：当前用户视角逻辑删除会话。
- `UpdateConversationSettings`：更新免打扰/置顶设置。

## 事件

| 事件类型 | Topic | data payload | 说明 |
| --- | --- | --- | --- |
| `MSG_PUSH` | `msg.push` | `msg.MsgItem` | 新消息下行，由 msg Outbox 经 CDC 生产。 |
| `MSG_RECALL` | `msg.push` | `msg.RecallNotice` | 撤回消息与 outbox 同事务提交后，由 CDC 生产。 |
| `MSG_MARK_READ` | `msg.push` | `msg.MarkReadNotice` | 已读位点与 outbox 同事务提交后，推给当前账号其他设备。 |
| `MSG_READ_RECEIPT` | `msg.push` | `msg.MarkReadNotice` | P2P 已读位点与 outbox 同事务提交后，通知对端。 |
| `group.cache` | `group.cache` | 严格 JSON v2 | msg 作为消费者维护群成员会话投影；与 group Redis projector 使用不同消费组。 |

## 发送消息链路

1. 操作者身份（`user_uuid`/`device_id`）由 gateway 侧 gRPC metadata 携带（`grpcx` 客户端拦截器从 ctx 注入），请求体不含身份字段；msg handler 经 `ctxmeta` 解析鉴权主体。
2. msg usecase 调用权限检查：
   - 单聊校验好友/黑名单关系；
   - 群聊校验群成员、群状态、禁言等权限。
3. message domain 做幂等检查、分配 seq，并在同一 MySQL 事务内写入 `message` 与 `outbox_events(event_type=msg.push)`。
4. conversation domain 更新发送方会话。
5. 单聊更新接收方会话；群聊只更新发送方个人会话和一条群共享热数据，不扫描群成员；共享行只有收到更大 seq 才整组推进 `max_seq/last_msg_*/updated_at`。
6. 消息与 outbox 提交成功即返回发送成功；会话 Upsert 失败只记录 Warn，不回滚消息。

同一 `client_msg_id` 重试命中首次结果时，会幂等修复固定数量的派生行：P2P 为双方个人行，
GROUP 为发送方个人行与一条共享群行；不会重新扫描群成员。
7. Debezium 监听 `outbox_events`，将 `msg.push` protojson 事件投递到 Kafka。

## 群成员会话投影链路

1. group 写事务更新 `groups/group_members`、递增 `groups.cache_version`，并同事务写入 `group.cache` Outbox。
2. Debezium 以 `group_uuid` 为 key 路由到 Kafka `group.cache`。
3. msg 的 `GroupMembershipProjector` 使用 `KAFKA_MSG_GROUP_MEMBERSHIP_GROUP_ID` 独立消费，不与 group Redis projector 抢占消息。
4. 消费者只接受当前 `schema_version=2` 顶层 JSON；旧 schema、旧包装、未知字段和缺省版本直接进入死信。
5. 仓储在同一个 MySQL 事务内锁定 `group_conversation`，检查事件幂等和严格连续版本；空库下每个群的首事件必须是 `version=1` 的 `group_created`，禁止把中途收到的增量事件猜成初始化快照，再按 action 增量更新：
   - 建群/加人：激活事件中的 K 个成员；
   - 退群/踢人：把已存在的 Active 成员改成退出 tombstone；缺行或重复移除视为投影历史损坏，不用 upsert 猜测；
   - 解散群：更新一条共享群状态；
   - 非成员事件：只推进版本。
6. 业务更新、`projection_version` 和 `idempotent_events` 标记一起提交后，consumer 才提交 offset。

发送群消息的 msg 数据库写路径因此不随群人数增长。在线推送仍需按在线成员/设备扇出，
那是 message-push 的下行复杂度，不是群消息持久化的写放大。

群会话删除同样遵守读扩散：删除时从 `group_conversation.max_seq` 写入个人
`clear_seq/read_seq`；后续若共享 max_seq 超过 clear_seq，查询层把该会话重新映射为
`status=0`，不需要在新消息到达时逐成员更新个人行。

## 撤回与已读链路

- `RecallMessage`：权限与时间窗口校验通过后，`message.status/content` 更新和 `MSG_RECALL` outbox 在同一个 MySQL transaction 内提交。
- `MarkRead`：`conversation.read_seq/unread_count` 更新和 `MSG_MARK_READ` outbox 在同一个 MySQL transaction 内提交。
- P2P `MarkRead` 会额外写入 `MSG_READ_RECEIPT` outbox；群聊只写当前用户 self-sync 的 `MSG_MARK_READ`。
- 上述 outbox 的 `entity_id` 均为 `conv_id`，保持同会话分区有序。

## 多会话批量同步链路

1. Gateway 严格校验会话数、逐项位点、重复 `conv_id` 和有效 limit 总和，再调用 `BatchSyncMessages`。
2. msg handler 只解析一次登录用户身份，`MessageReadWorkflow` 为每个会话复用单会话读取逻辑：
   - `ResolveReadAccess` 裁决当前用户读取资格并取得 `clear_seq`；
   - `GetMaxSeq` 读取当前最大 seq；
   - `PullMessages` 固定使用 FORWARD 方向查询 `seq > after_seq AND seq > clear_seq`。
3. 单批最多 50 个会话，内部查询并发上限为 8，所有有效 limit 之和最多 500。
4. 每个 goroutine 只写预分配结果切片的固定下标，所以查询可并发完成、响应仍严格保持请求顺序。
5. 所有查询收敛后按请求顺序执行约 3 MiB 的消息本体预算裁剪；被裁剪项只推进到实际返回的最后 seq，并强制 `has_more=true`，确保 Gateway 4 MiB gRPC 接收上限不会把整批结果变成传输失败。
6. 单会话无权读取或查询失败只写入该项 `error_code`；请求参数非法、父 deadline 到期或调用取消才整体失败。

该接口消除的是客户端对 Gateway/msg-service 的 N 次网络往返，不会把消息表改成跨会话扫描。底层仍按每个 `conv_id + seq` 索引独立读取，因此查询并发必须有界。

## 非阻断策略

| 步骤 | 失败处理 | 原因 |
| --- | --- | --- |
| 消息落库 | 阻断返回错误 | 消息事实未成立。 |
| 撤回 / 已读 outbox 落库 | 阻断并回滚业务变更 | 业务事实与下行通知必须原子提交。 |
| 会话 Upsert | Warn，不阻断 | 后续消息或拉取可自愈。 |
| CDC/Kafka 投递 | 不在请求内等待 | 客户端可通过单会话 `PullMessages` 或多会话 `BatchSyncMessages` 自愈。 |
| `group.cache` 投影临时失败 | 不提交 offset 并重试 | 成员资格必须按事件版本连续投影，不能由下一条群消息猜测补偿。 |

## 不变量

- 消息事实以 MySQL 为准，Redis 只用于 seq 和幂等缓存。
- 发送成功的判定点是“消息与 `msg.push` Outbox 同事务提交成功”，不是“下行推送成功”。
- `seq` 只在单个会话内有序，不能跨会话比较；批量同步必须为每个会话携带独立位点。
- `client_msg_id` 必须由客户端在重试时复用。
- group 拥有成员事实；msg 的 `membership_*` 仅是可重建投影。
- GROUP 会话必须显式为 Active 或 Left，禁止把 `membership_status=0` 当作旧数据兼容态。
- 撤回后客户端渲染应优先看 `status`，不要继续展示原 `content`。
