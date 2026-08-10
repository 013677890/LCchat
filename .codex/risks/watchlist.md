# 修改风险清单

每一项都说明风险、触发条件和优先检查位置。它们不是设计结论；修改前仍要回到代码确认。

## 异常账号事件会被直接跳过

- 风险：user_created、profile_display_changed、部分 account.deleted 的非法负载不会进入 dead_events，事件会不可追溯地提交。
- 触发：修改账号事件 schema、CDC 转换、消费者解码或宣称所有坏消息可追踪时。
- 先查：apps/auth/internal/consumer/profile_display_changed_consumer.go、apps/user/internal/consumer、apps/relation/internal/consumer/account_deleted_consumer.go。
- 当前结论：高置信。它们在 Decode 失败后返回 nil。

## Redis 补偿不是持久兜底

- 风险：Redis 写失败后若 Kafka producer 不可用，整键 DEL 任务只会记录日志并丢失；缓存不一致只能等待自然失效、读回源或其他修复。
- 触发：评估 Redis/Kafka 故障面、修改 WriteFailureHook、把缓存失败上抛给用户或承诺可靠补偿时。
- 先查：pkg/redisretry/helper.go、manager.go、write_failure_hook.go，Relation 的 cache_compensation.go。
- 当前结论：高置信。主业务 API 不会为补偿 Kafka 失败返回错误。

## RedisTask 是严格、不兼容的契约

- 风险：未知字段、尾随 JSON、旧负载都会被永久死信；生产者/消费者同时升级失败可能造成补偿停留在 dead_events。
- 触发：修改 RedisTask、Topic payload、灰度升级或设计向前兼容时。
- 先查：pkg/redisretry/consumer.go、redis_task.go、对应 dead_events。
- 当前结论：高置信。禁止向前兼容是当前明确策略。

## 群投影不等于强一致读模型

- 风险：Group 事实提交后，Redis 群缓存和 Msg 的成员资格投影到达时间不同；实时群下行和成员权限读取可能短暂不同步。
- 触发：修改成员、角色、禁言、群发送权限、Kafka 并发或 group.cache 分区时。
- 先查：Group event mapper/projector/reconciler、Msg group_membership_projector、Message-push 群路由。
- 当前结论：高置信。读 miss 与默认 6 小时周期对账只能最终修复。

## Conversation 身份与序号空洞

- 风险：错误假定 conv_id/upsert 语义跨数据库一致，或错误假定最大 seq 等于连续持久消息。
- 触发：改 conversation 索引、P2P conv_id、消息拉取补洞、ACK 或 max-seq 逻辑时。
- 先查：config/mysql/001_schema.sql、apps/msg/internal/domain/conversation、apps/msg/internal/domain/message。
- 当前结论：高置信。Redis INCR 可先于 MySQL 插入成功，seq 空洞可能存在。

## 手动消费者并不天然 exactly-once

- 风险：在业务副作用与 MarkIdempotent 之间崩溃/重平衡可重复执行非幂等动作。
- 触发：新增 Kafka consumer、外部通知、计数、邮件、资金或其他不可重入副作用时。
- 先查：pkg/kafka/consumer.go、pkg/outbox、对应 apps/服务/internal/consumer。
- 当前结论：高置信。只有把业务写与幂等标记放在同一事务才接近原子。

## dead_events 没有自动重放

- 风险：运维误以为 pending 记录会自动重新执行。
- 触发：增加 DLQ、制定告警或设计补偿流程时。
- 先查：pkg/outbox/deadletter.go、scripts/migration/004_dead_events.sql。
- 当前结论：高置信。当前没有完整的自动 replay worker。

## Message-push 失败后让出 offset

- 风险：msg.push 或 realtime.push 实时下行在有限重试后被跳过，不会进入 dead_events。
- 触发：提高推送可靠性、修改撤回/已读语义或扩大群扩散时。
- 先查：apps/message-push/internal/consumer、apps/connect/internal/grpc。
- 当前结论：高置信。这是避免单条消息卡住分区的取舍，客户端需拉取/同步或查询修复。

## 好友申请与黑名单的短时可见性

- 风险：并发申请或 deadline 重试可能重复写；拉黑/解除后缓存和 Msg 发送校验在短窗口内不同步。
- 触发：改申请去重、RPC 重试、关系缓存、消息发送权限时。
- 先查：apps/relation/internal/service/friend_service.go、repository/apply_repository.go、Msg 对 Relation 的调用点。
- 当前结论：中高置信。关系事实是 MySQL，缓存与实时提醒是最终一致。

## 路由与超时映射会悄然漂移

- 风险：新增路由遗漏 gatewayRequestTimeouts 会落到默认 2 秒；静态群路径放在动态 :groupUuid 后会被吞掉。
- 触发：新增/重命名 Gateway API、调整接口预算时。
- 先查：apps/gateway/internal/router/router.go 与 .codex/api-map.md。
- 当前结论：高置信。现有若干群路由明确依赖路由注册顺序。

## Connect ACK 的事件范围有限

- 风险：非 MSG_PUSH 类型的事件未必能像普通消息一样可靠记录 conv_id 水位。
- 触发：新增需要 ACK 的实时事件或改 envelope 元数据时。
- 先查：apps/connect/internal/grpc/server.go、apps/connect/internal/svc/ack.go、proto/msg/msg_push_event.proto。
- 当前结论：中高置信；客户端的实际协调行为需要联调确认。
