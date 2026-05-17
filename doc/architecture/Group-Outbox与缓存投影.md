# Group-Outbox 与缓存投影

group 服务使用 Outbox + Debezium + Kafka + Redis projector 维护群缓存。该链路用于把群写模型的最终事实异步投影到 Redis，避免写请求直接承担复杂缓存更新和跨服务传播成本。

## 目标

- 群写操作和事件落库在同一 MySQL 事务内完成。
- 同一个群的缓存事件以 `group_uuid` 为 key，保证同群事件按分区有序。
- projector 只投影缓存，不做业务权限判断。
- Redis 写失败时允许重试，非法 payload 跳过，避免阻塞分区。

## 写入侧

group repository 在关键写操作中调用 `insertGroupCacheEvent`，事件类型固定为 `group.cache`，`entity_id` 固定为 `group_uuid`。

| 动作 | 事件 Action | 主要 payload |
| --- | --- | --- |
| 建群 | `group_created` | 群快照、初始成员快照、成员 UUID 列表。 |
| 新增或恢复成员 | `member_added` | 群快照、成员快照、用户 UUID 列表。 |
| 移除成员或退群 | `member_removed` | 群快照、目标用户 UUID。 |
| 解散群 | `group_dismissed` | 群快照、活跃成员 UUID 列表。 |
| 更新群资料 | `group_info_updated` | 最新群快照。 |
| 转让群主 | `owner_transferred` | 群快照、老群主和新群主成员快照。 |
| 更新成员角色 | `member_role_updated` | 群快照、目标成员快照。 |
| 更新群名片 | `member_profile_updated` | 群快照、目标成员快照。 |
| 单人禁言 | `member_muted` | 群快照、目标成员禁言快照。 |
| 全员禁言 | `group_mute_setting_updated` | 群快照。 |
| 创建入群申请 | `join_request_created` | 申请快照。 |
| 审批入群申请 | `join_request_reviewed` | 申请快照。 |
| 撤销入群申请 | `join_request_canceled` | 申请快照。 |

## CDC 到 Kafka

1. group 在业务事务内写群表和 `outbox_events`。
2. Debezium 监听 `outbox_events`。
3. EventRouter 按 `event_type=group.cache` 路由到 Kafka `group.cache`。
4. Kafka key 使用 `entity_id`，也就是 `group_uuid`。

## 投影侧

`apps/group/internal/consumer/cache_projector.go` 使用手动提交 Consumer：

1. 解析 `group.cache` payload。
2. 用 `idempotent_events` 检查事件是否已处理。
3. 调用 `ApplyGroupCacheEvent` 更新 Redis。
4. 写幂等记录。
5. 成功后提交 Kafka offset。

## Redis 投影对象

| Key | 用途 | 更新策略 |
| --- | --- | --- |
| `group:info:{group_uuid}` | 群资料主缓存 | 建群完整创建，后续资料、禁言、解散等 patch-if-exists。 |
| `group:members:{group_uuid}` | 群成员 Hash | 建群重建，成员增删和角色禁言 patch。 |
| `group:user_groups:{user_uuid}` | 用户加入群反向索引 | patch-if-exists，缓存不存在时由读路径回源重建。 |
| `group:join_requests:{group_uuid}` | 待审批入群申请缓存 | 申请创建、审批、撤销按事件增删。 |

## 失败处理

| 失败 | 处理 |
| --- | --- |
| payload 无法解析 | 记录 Warn，跳过该消息。 |
| payload 缺必要字段 | 记录 Warn，跳过该消息。 |
| 幂等检查失败 | 返回错误，不提交 offset，等待重试。 |
| Redis 写失败 | 返回错误，不提交 offset，等待重试。 |
| 写幂等记录失败 | 返回错误，不提交 offset，避免重复处理不可见。 |

## 读路径要求

- 缓存命中时直接返回 Redis 投影。
- 缓存不存在时回源 MySQL 并重建缓存。
- patch-if-exists 的设计要求读路径始终具备全量重建能力。
- 消息发送权限不能只依赖可能缺失的 Redis 缓存，必要时应由 group 服务回源判断。

## 维护注意

- 新增群写动作时，必须同时补充 `group.cache` action、payload、projector 分支和本文档。
- 改动群缓存 Key 或 TTL 时，必须同步更新 [data/Redis-Key设计.md](../data/Redis-Key设计.md)。
- 改动 Outbox 字段或 Debezium 路由时，必须同步更新 [事件驱动与最终一致性](事件驱动与最终一致性.md)。
