# Relation 服务

## 角色

- 拥有好友申请、好友关系、备注/标签、黑名单以及注销时本域数据清理。
- 不拥有账号、公开资料、群成员、消息或 WebSocket 连接。

## 修改前优先阅读

- apps/relation/cmd/providers.go
- apps/relation/internal/service/friend_service.go
- apps/relation/internal/service/blacklist_service.go
- apps/relation/internal/service/realtime_notify.go
- apps/relation/internal/repository/friend_repository.go
- apps/relation/internal/repository/apply_repository.go
- apps/relation/internal/repository/blacklist_repository.go
- apps/relation/internal/repository/cache_compensation.go
- apps/relation/internal/consumer/account_deleted_consumer.go
- proto/relation

## 稳定业务行为

- 关系记录以单向记录为主；同意好友申请会在同一事务创建双向关系。
- 删除好友目前只删除当前用户指向对方的单向关系。
- 黑名单是单向的，解除时按之前关系状态恢复。
- SyncFriendList 会把最新版本回退 2 秒，降低边界漏读风险。
- GetTagList 当前没有实现，不要把标签列表当作已有契约。

## 缓存与补偿

- 好友缓存是以用户为键、对端 UUID 为 field 的 Redis Hash，值为关系元数据 JSON。
- 空好友列表使用短 TTL 的 __EMPTY__ 标记；黑名单和待处理申请分别使用用户维度 ZSet。
- 缓存异步任务被协程池拒绝时会投递整键 DEL 补偿。
- go-redis 结束其自身短重试后的最终写失败会由 WriteFailureHook 提取当前支持命令的键并投递 DEL 补偿；支持 DEL、EVAL/EVALSHA、SET、INCR、EXPIRE、HSET、ZADD。
- 补偿 Kafka topic 为 relation.redis.invalidate，consumer 只执行 DEL；任务 JSON 不兼容未知字段、尾随 JSON 或旧格式，永久失败写入 dead_events，source 为 relation-service:redis-invalidation。
- Kafka 补偿投递失败只告警并放弃，因此这不是 Redis 故障时的同步可用性保证。

## 事件与实时提醒

- account.deleted 消费后软删本域关系/申请并失效相关缓存；它配置的死信 source 为 relation-service:account.deleted。
- account.deleted 的解码错误当前只记录并返回成功，会提交 offset，因而不会进入死信。
- 好友申请创建、处理、关系/备注/标签/黑名单变化会尽力异步投递 realtime.push。
- realtime.push 不走 Outbox；编码、Kafka 发布或异步执行失败都只记录日志，不阻断成功的关系写操作。

## 修改陷阱

- 好友申请并发去重、数据库约束和 RPC 重试必须一起核对；网关或 gRPC 超时重试可能让非幂等写重复执行。
- 不要以缓存补偿为理由重放 SET/HSET/Lua 写；过期局部状态可能覆盖 MySQL 新事实。
- 拉黑后立刻阻断消息依赖 Msg 侧的关系校验和缓存时效；关系服务提交成功不代表每个读缓存已同时刷新。
