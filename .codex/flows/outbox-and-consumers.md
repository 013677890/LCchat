# Outbox、Kafka 与补偿消费者

涉及账号/资料/群事件、Redis 补偿、死信或幂等时先读本文件和对应 consumer。

## 本地 Topic 拓扑

Docker Compose 的 kafka-topics-init 用固定 3 分区创建以下业务 Topic：

- auth.redis.invalidate、user.redis.invalidate、relation.redis.invalidate
- user_created、profile_display_changed、account.deleted
- msg.push、realtime.push、group.cache

应用不得在线扩展业务 Topic 分区。特别是 group.cache 按 group_uuid 作为 key，分区改变会改变同 key 的哈希落点和既有顺序假设。

## 生产和消费关系

| Topic | 生产者 | 消费者 | 目的 |
| --- | --- | --- | --- |
| user_created | Auth Outbox | User | 注册后创建资料 |
| profile_display_changed | User Outbox | Auth | 回写登录展示昵称/头像 |
| account.deleted | Auth Outbox | User、Relation | 异步清理各自数据 |
| group.cache | Group Outbox | Group、Msg | Redis 群缓存与 Msg 群成员资格投影 |
| msg.push | Msg Outbox | Message-push | 消息、撤回、已读等下行 |
| realtime.push | Relation、Group 直接 Producer | Message-push | 关系/群变化的尽力实时提醒 |
| 各服务 redis.invalidate | Auth、User、Relation 的补偿 Producer | 对应服务 | Redis 最终写失败后的整键 DEL |

Outbox 的业务表写入与 outbox_events 插入必须在同一个 MySQL 事务中完成。Debezium/Kafka Connect 按 event_type 路由，entity_id 作为 Kafka key。group.cache 与 msg.push 只接受当前顶层 JSON 对象；不要向消费者加入旧 schema、包装对象或字符串负载的向前兼容。

## 手动提交与死信

- 所有服务侧消费者统一经 pkg/kafka.ManualConsumerPool 启动；每个 worker 持有独立 Reader，并只在 handler 成功或死信成功落地后提交 offset。
- workers 默认 3，显式值只接受 1～64；它不是 partition 绑定，Kafka rebalance 自动分配，多余 Reader 正常 idle。
- 有 DeadLetterSink 时，每次处理默认 10 秒超时；可重试失败默认在同一消息上重试最多 2 分钟，再落 dead_events。
- 永久错误和 handler panic 会首轮尝试落 dead_events；死信写失败则保持 offset 不提交，继续阻塞。
- idempotent_events 通常采用 Check -> 业务操作 -> Mark。除非业务写和 Mark 在同一事务，否则它不是严格 exactly-once。
- Msg 的 group.cache 成员资格投影会把业务更新、群版本推进和 MarkIdempotent 放在同一 MySQL 事务。

当前已配置的 dead_events source：

- auth-service:profile_display_changed、auth-service:redis-invalidation
- user-service:user_created、user-service:account.deleted、user-service:redis-invalidation
- relation-service:account.deleted、relation-service:redis-invalidation
- group-service:group.cache
- msg-service:group-membership

Auth 的 profile_display_changed、User 的 user_created/account.deleted、Relation 的 account.deleted 对解码失败统一返回永久错误；只有对应 `dead_events` 写入成功后才提交，坏事件不会无落点跳过。

## Redis 整键 DEL 补偿

- 适用服务为 Auth、User、Relation。go-redis 的最终写失败发生在其内置短重试之后。
- WriteFailureHook 仅为当前明确支持的写命令提取键：DEL、EVAL/EVALSHA、SET、INCR、EXPIRE、HSET、ZADD；读取或未知命令不生成任务。
- Relation 的异步缓存任务被协程池拒绝时也会生成整键 DEL 任务。
- 任务以第一个 Redis key 作为 Kafka key；消费者只调用 DEL，绝不重放 SET、HSET、Pipeline 或 Lua 写。
- RedisTask 严格解码：未知字段、尾随 JSON、校验失败都是永久错误，进入对应 redis-invalidation dead_events。这里禁止向前兼容。
- Redis 或 Kafka 均失败时，补偿 Producer 只记录日志并放弃；主业务 API 不会因这条后台补偿再向上失败。

## Message-push 例外

Message-push 仍保留有限本地重试和 best-effort 丢弃语义，但编排统一使用两个 ManualConsumerPool。msg.push 和 realtime.push 的处理在重试耗尽后记录告警/指标并让出 offset，不写 dead_events。它避免单条消息永久卡住分区，但实时下行不保证至少一次；客户端应通过消息拉取、批量同步或权威业务查询修复。
