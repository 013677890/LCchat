# Kafka 事件

本文说明当前项目 Kafka Topic、事件生产者、消费者、payload 和一致性语义。事实来源包括 `config/kafka.go`、`scripts/cdc/register_outbox_connector.sh` 和各服务 producer/consumer。

## 1. Topic 总览

| Topic | 默认环境变量 | 生产者 | 消费者 | 说明 |
| --- | --- | --- | --- | --- |
| `auth.redis.invalidate` | `KAFKA_AUTH_REDIS_RETRY_TOPIC` | auth | auth | 只补偿 auth 所属缓存的安全 `DEL`。 |
| `user.redis.invalidate` | `KAFKA_USER_REDIS_RETRY_TOPIC` | user | user | 只补偿 user 所属缓存的安全 `DEL`。 |
| `user_created` | `KAFKA_USER_CREATED_TOPIC` | auth Outbox | user | 注册成功后初始化用户资料。 |
| `profile_display_changed` | `KAFKA_PROFILE_DISPLAY_CHANGED_TOPIC` | user | auth | 昵称/头像变化后回写登录展示冗余。 |
| `account.deleted` | `KAFKA_ACCOUNT_DELETED_TOPIC` | auth Outbox | user/relation/group 等 | 账号注销后的下游清理。 |
| `msg.push` | `KAFKA_MSG_PUSH_TOPIC` | msg Outbox | message-push | 消息下行、撤回、已读同步。 |
| `realtime.push` | `KAFKA_REALTIME_PUSH_TOPIC` | relation、group | message-push | 好友、关系、群申请和群状态等非消息类实时提醒。 |
| `group.cache` | `KAFKA_GROUP_CACHE_TOPIC` | group Outbox | group Redis projector、msg membership projector | 群事实的 Redis 缓存投影与 msg 成员会话投影。 |

## 2. Kafka 默认配置

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `KAFKA_BROKERS` | `kafka:9092` | Broker 地址。 |
| Producer `BatchSize` | 100 | 批量发送大小。 |
| Producer `BatchTimeout` | 10ms | 批量等待时间。 |
| Producer `MaxAttempts` | 3 | 生产重试次数。 |
| Producer `WriteTimeout` | 10s | 写入超时。 |
| Consumer `MinBytes` | 1 | 最小读取字节。 |
| Consumer `MaxBytes` | 10MB | 最大读取字节。 |
| Consumer `MaxWait` | 500ms | 最大等待。 |
| Consumer `CommitInterval` | 1s | Offset 提交间隔。 |
| Consumer `StartOffset` | -1 | 从最新开始消费。 |

`group.cache` 的两个 projector 必须使用不同 consumer group：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `KAFKA_GROUP_CACHE_GROUP_ID` | `group-cache-projector-group` | group-service Redis 投影。 |
| `KAFKA_MSG_GROUP_MEMBERSHIP_GROUP_ID` | `msg-group-membership-projector-group` | msg-service `conversation` 成员投影。 |
| `KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY` | `3` | group Redis projector 每进程独立 Reader 数。 |
| `KAFKA_MSG_GROUP_MEMBERSHIP_PROJECTOR_CONCURRENCY` | `3` | msg membership projector 每进程独立 Reader 数。 |

并发配置规则：未配置默认 `3`；显式值必须是 `1～64` 正整数；零、负数或非法文本导致服务启动失败，禁止静默回退。

### Redis 缓存失效补偿

auth 和 user 使用不同 Topic 与 consumer group，任务不会跨服务竞争，也不会因不同 group 广播到两个服务。payload 只包含待删除的 key 和追踪元数据，不接受 `SET`、`HSET`、Pipeline 或 Lua 等可写回旧值的命令。

消费者使用手动提交：`DEL` 成功后提交 offset；Redis 暂时失败时原地重试同一条消息；payload 非法或重试预算耗尽时，只有写入当前服务数据库的 `dead_events` 成功后才提交。初次投递使用第一个 key 作为 Kafka key。

升级时不要把旧 `redis-retry-queue` 直接接到新消费者；旧消息可能包含 `SET/HSET` 等已废弃写命令，应在停掉旧消费者后单独审计或清理。

### `group.cache` 分区与并行语义

| 规则 | 说明 |
| --- | --- |
| 固定 partition 数 | 目标 **3** partitions（Compose `kafka-topics-init` / scale 脚本以 `--partitions 3` 创建）。 |
| Kafka key | Outbox `entity_id=group_uuid`，同群事件进入同一 partition，保证严格有序。 |
| 不同 consumer group | group 与 msg 各自完整消费，互不抢消息。 |
| 进程内并行 | 每服务启动 N 个独立 `kafka.Reader`（同 group ID），由 Kafka 分配不同 partition。 |
| 同 partition 串行 | 每个 Reader 严格 `Fetch → handle 成功 → Commit → 下一条`。 |
| worker > partition | 多余 worker 只会空闲，不会共享 Reader 并发 Fetch。 |
| 禁止在线扩分区 | 应用不得 `alter --partitions`；扩容会改变 key 哈希落点。有积压时需停写、排空后重建或换新 topic 迁移。 |

msg membership projector 使用手动提交 consumer；新消费组没有已提交 offset 时从可见的
最早事件开始，不能从最新 offset 推断当前成员全集。事件幂等、死信、重试预算语义与单 Reader 时代相同，**不会**采用 message-push 的“重试耗尽后直接丢弃”。

## 3. Outbox CDC 路由

Outbox 表 `outbox_events` 由 Debezium MySQL Connector 监听，EventRouter 配置如下：

| 配置 | 值 | 说明 |
| --- | --- | --- |
| `route.by.field` | `event_type` | 使用事件类型决定目标 Topic。 |
| `route.topic.replacement` | `${routedByValue}` | Topic 名等于 `event_type`。 |
| `table.field.event.key` | `entity_id` | Kafka key 使用业务实体 ID。 |
| `table.field.event.payload` | `payload` | Kafka value 使用 payload。 |
| `table.expand.json.payload` | `true` | 将 Outbox JSON 字符串展开为 Kafka 顶层 JSON Object，禁止消费者自行解套旧包装。 |
| `table.field.event.id` | `id` | Outbox 事件 ID。 |
| `additional.placement` | `event_type/entity_id/created_at` | 写入消息 header。 |

链路：

```text
业务事务 -> outbox_events -> Debezium -> Kafka Topic(event_type) -> Consumer -> idempotent_events
```

## 4. `user_created`

| 项 | 说明 |
| --- | --- |
| 生产者 | auth 注册流程，事务内写 Outbox。 |
| 消费者 | user。 |
| Key | `entity_id`，通常为 `user_uuid`。 |
| 语义 | 用户账号创建成功后，异步初始化资料域。 |

消费要求：

1. user 必须按事件 ID 幂等处理。
2. 如果资料已存在，应视为幂等成功。
3. 注册接口成功只代表 auth 账号成立；资料初始化通过最终一致完成。

## 5. `profile_display_changed`

| 项 | 说明 |
| --- | --- |
| 生产者 | user 更新昵称或头像后生产。 |
| 消费者 | auth。 |
| 语义 | 回写 `user_account.login_nickname` 和 `login_avatar`。 |

该事件只维护登录返回展示冗余，不改变 user_profile 权威事实。

## 6. `account.deleted`

| 项 | 说明 |
| --- | --- |
| 生产者 | auth 注销账号流程。 |
| 消费者 | user、relation、group 等下游。 |
| 语义 | 账号已注销，下游按各自边界清理或标记相关数据。 |

注销同步阶段只保证 auth 标删账号和清理 Token；下游最终一致处理失败时应可重试，不应影响账号注销接口返回。

## 7. `group.cache`

| 项 | 说明 |
| --- | --- |
| 生产者 | group 写操作事务内 Outbox。 |
| 消费者 | group Redis cache projector；msg group membership projector。 |
| Key | `group_uuid`。 |
| 语义 | 分别维护群 Redis 缓存，以及 msg 的成员会话/群状态投影。 |

基础契约只接受 `schema_version=2` 且 `projection_version>0`。版本由 group 写事务递增 `groups.cache_version` 后生成；JSON 字符串包装、`payload/after/data` 包装、未知字段、缺省版本或未知 schema 均按永久错误首轮写入 `dead_events`，不做兼容解析。

两个 consumer group 都会收到每一条事件，不能配置成相同 group ID：

- group projector 把最终群快照和成员/申请变化用版本 Lua 投影到 Redis；
- msg projector 在一个 MySQL 事务内检查 `idempotent_events` 和单群连续版本，然后增量更新 `conversation.membership_*`、`group_conversation.group_status/projection_version`；
- message-push 不消费 `group.cache`，也不负责把它转发给 msg 或 group；
- msg 要求每个群从 `version=1/group_created` 开始，后续 `projection_version` 必须等于 `current+1`；缺首事件、版本跳号、重复建群或解散后继续变更都会被拒绝，原事件进入 `dead_events(source=msg-service:group-membership)`。

常见 action：

| action | 触发场景 |
| --- | --- |
| `group_created` | 创建群。 |
| `group_info_updated` | 修改群名称、头像、加群方式。 |
| `group_dismissed` | 解散群。 |
| `member_added` | 添加成员。 |
| `member_removed` | 移除成员或退群。 |
| `owner_transferred` | 转让群主。 |
| `member_role_updated` | 设置/取消管理员。 |
| `member_profile_updated` | 更新群名片。 |
| `member_muted` | 单人禁言。 |
| `group_mute_setting_updated` | 全员禁言。 |
| `join_request_created` | 创建入群申请。 |
| `join_request_reviewed` | 审批入群申请。 |
| `join_request_canceled` | 撤销入群申请。 |

## 8. `msg.push`

| 项 | 说明 |
| --- | --- |
| 生产者 | msg 写 `outbox_events`，由 Debezium CDC 路由。 |
| 消费者 | message-push。 |
| Key | `conv_id`，保证同一会话尽量落在同一分区。 |
| Value | `msg.MsgPushEvent` protojson。 |
| 语义 | 将消息事实变更转为在线 WebSocket 下行。 |

`msg.push` 不兼容旧版 Protobuf bytes；消费端只接受严格 protojson，未知字段或缺少 `event_id` 均按永久错误跳过。

`MsgPushEvent` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `receiver_uuid` | string | 单聊为对端用户 UUID，群聊为群 UUID。 |
| `device_id` | string | 指定设备或空。 |
| `type` | string | `MSG_PUSH`、`MSG_RECALL`、`MSG_MARK_READ` 等。 |
| `conv_type` | enum | 1 单聊，2 群聊。 |
| `data` | bytes | 按 type 序列化的业务 payload。 |
| `trace_id` | string | 链路 ID。 |
| `server_ts` | int64 | 服务端时间，Unix 毫秒。 |
| `from_uuid` | string | 发送方 UUID。 |
| `seq` | int64 | 下行事件顺序键。 |
| `event_id` | string | Outbox 事件 ID，用于 CDC 重放和下游幂等。 |

支持类型：

| type | data | 说明 |
| --- | --- | --- |
| `MSG_PUSH` | `MsgItem` | 新消息。 |
| `MSG_RECALL` | `RecallNotice` | 撤回通知。 |
| `MSG_MARK_READ` | `MarkReadNotice` | 已读多端同步。 |
| `MSG_READ_RECEIPT` | `MarkReadNotice` | P2P 已读回执。 |

## 9. `realtime.push`

| 项 | 说明 |
| --- | --- |
| 生产者 | relation、group 等业务服务。 |
| 消费者 | message-push。 |
| Key | 单用户用 `user_uuid`，群聚合用 `group_uuid`，多用户列表用首个用户 UUID。 |
| Value | `realtime.RealtimePushEvent` Protobuf bytes。 |
| 语义 | 将好友、关系、群申请、群成员和群状态变化转为在线 WebSocket 实时提醒。 |

`RealtimePushEvent.target.kind` 支持：

| kind | 说明 |
| --- | --- |
| `user` | 投递给单个用户所有在线设备。 |
| `device` | 投递给单个用户的指定设备。 |
| `user_list` | 投递给多个用户所有在线设备。 |
| `group_members` | message-push 查询 group 后扩散给群成员。 |
| `group_admins` | message-push 查询 group 后扩散给群主和管理员。 |

常见 type：`FRIEND_APPLY_CREATED`、`FRIEND_APPLY_HANDLED`、`FRIEND_RELATION_CHANGED`、`GROUP_JOIN_REQUEST_CREATED`、`GROUP_JOIN_REQUEST_REVIEWED`、`GROUP_MEMBER_REMOVED`、`GROUP_DISMISSED`、`GROUP_MEMBER_MUTED`、`GROUP_STATE_CHANGED`。业务 payload 使用 `pkg/realtimepb` 中对应 protobuf message 编码到 `data`。

## 10. 错误处理策略

| 场景 | 策略 |
| --- | --- |
| Outbox 重复投递 | 消费者通过 `idempotent_events` 去重。 |
| payload 解析失败 | 视为永久错误，记录并跳过，避免毒消息阻塞。 |
| Redis 临时失败 | 返回可重试错误，由消费者重试。 |
| msg 成员投影 DB 临时失败 | 不提交 offset，在预算内原地重试。 |
| msg 单群投影版本缺口 | 视为永久错误并写死信，禁止跨过缺口继续构造不完整成员视图。 |
| Connect gRPC 临时失败 | message-push 本地有限重试，最终失败由客户端补拉自愈。 |
| msg outbox 写入失败 | 与业务事务一起回滚，请求返回失败。 |
| CDC/Kafka 投递失败 | 不在 msg 请求内等待，由 Debezium/Kafka Connect 重试；客户端可 Pull 自愈。 |

## 11. 维护规则

1. 新增 Topic 必须同步更新 `config/kafka.go`、环境变量示例和本文。
2. Outbox 事件 payload 必须包含可幂等处理的事件 ID。
3. 消费者必须明确区分“永久错误”和“可重试错误”。
4. `msg.push` 或 `realtime.push` 新增 type 时必须同步更新对应 proto、`message-push`、WebSocket 协议和前端处理逻辑。
