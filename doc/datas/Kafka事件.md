# Kafka 事件

本文说明当前项目 Kafka Topic、事件生产者、消费者、payload 和一致性语义。事实来源包括 `config/kafka.go`、`scripts/cdc/register_outbox_connector.sh` 和各服务 producer/consumer。

## 1. Topic 总览

| Topic | 默认环境变量 | 生产者 | 消费者 | 说明 |
| --- | --- | --- | --- | --- |
| `redis-retry-queue` | `KAFKA_RETRY_TOPIC` | 缓存重试逻辑 | 重试消费者 | Redis 重试队列。 |
| `user_created` | `KAFKA_USER_CREATED_TOPIC` | auth Outbox | user | 注册成功后初始化用户资料。 |
| `profile_display_changed` | `KAFKA_PROFILE_DISPLAY_CHANGED_TOPIC` | user | auth | 昵称/头像变化后回写登录展示冗余。 |
| `account.deleted` | `KAFKA_ACCOUNT_DELETED_TOPIC` | auth Outbox | user/relation/group 等 | 账号注销后的下游清理。 |
| `msg.push` | `KAFKA_MSG_PUSH_TOPIC` | msg Outbox | message-push | 消息下行、撤回、已读同步。 |
| `realtime.push` | `KAFKA_REALTIME_PUSH_TOPIC` | relation、group | message-push | 好友、关系、群申请和群状态等非消息类实时提醒。 |
| `group.cache` | `KAFKA_GROUP_CACHE_TOPIC` | group Outbox | group cache projector | 群缓存投影事件。 |

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
| 消费者 | group cache projector。 |
| Key | `group_uuid`。 |
| 语义 | 维护群资料、成员、用户群列表、入群申请等 Redis 投影。 |

基础契约只接受 `schema_version=2` 且 `projection_version>0`。版本由 group 写事务递增 `groups.cache_version` 后生成；JSON 字符串包装、`payload/after/data` 包装、未知字段、缺省版本或未知 schema 均按永久错误首轮写入 `dead_events`，不做兼容解析。

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
| Connect gRPC 临时失败 | message-push 本地有限重试，最终失败由客户端补拉自愈。 |
| msg outbox 写入失败 | 与业务事务一起回滚，请求返回失败。 |
| CDC/Kafka 投递失败 | 不在 msg 请求内等待，由 Debezium/Kafka Connect 重试；客户端可 Pull 自愈。 |

## 11. 维护规则

1. 新增 Topic 必须同步更新 `config/kafka.go`、环境变量示例和本文。
2. Outbox 事件 payload 必须包含可幂等处理的事件 ID。
3. 消费者必须明确区分“永久错误”和“可重试错误”。
4. `msg.push` 或 `realtime.push` 新增 type 时必须同步更新对应 proto、`message-push`、WebSocket 协议和前端处理逻辑。
