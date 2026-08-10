# 服务总览

先用本文件确定数据归属；再打开具体服务文件。业务表只能由拥有它的服务写入。

## 数据与职责归属

| 服务 | 拥有的核心事实 | 主要职责 |
| --- | --- | --- |
| Gateway | 无业务表 | HTTP 入口、JWT、限流、超时、DTO/RPC 转换、少量读聚合 |
| Auth | user_account、device_sessions | 注册、登录、验证码、Token、设备会话、改密改邮箱、注销 |
| User | user_profile | 资料、头像 URL、二维码、搜索、内部资料 RPC |
| Relation | user_relations、好友申请 | 好友关系、申请、备注、标签、黑名单、注销后的本域清理 |
| Group | groups、group_members、group_join_requests | 群资料、成员、角色、禁言、审批和缓存事件 |
| Msg | message、conversation、group_conversation | 消息、序号、幂等、会话、撤回、已读和推送事件 |
| Connect | 进程内连接与 Redis 路由/ACK | WebSocket、在线路由、心跳、确认位点、推送 RPC |
| Message-push | 无业务表 | 消费 Kafka、计算在线目标、调用 Connect 下发 |

## 依赖形状

- Gateway 同步调用 Auth、User、Relation、Group、Msg。
- Msg 在发送、撤回和群消息路径调用 Relation、Group；它自己持有消息和会话事实。
- Message-push 使用 Redis 路由，调用 Group 查询群目标并调用 Connect 推送。
- Connect 可调用 Auth 的设备相关 RPC；它不消费业务 Kafka。
- Auth 产生 user_created 与 account.deleted；User 产生 profile_display_changed；三者经 Outbox/CDC。
- Group 产生 group.cache Outbox 事件；Group 和 Msg 用独立消费组投影它。
- Relation 与 Group 直接异步产生 realtime.push；Message-push 消费它。

## 强一致与最终一致边界

- 同一个服务内的业务表与 Outbox 必须同一 MySQL 事务提交。
- Redis 缓存、群投影、资料展示冗余、推送与 WebSocket 下行都是最终一致副作用。
- 消息发送成功以消息事实和 msg.push Outbox 提交为界；后续推送失败由拉取/同步弥补。
- 直接 realtime.push 不进 Outbox，且发布失败不向 API 上抛；客户端必须能重新查询权威状态。

## 启动与降级

- Compose 会让服务等待声明的依赖健康；这描述本地标准拓扑，不等于代码的所有降级语义。
- Gateway 的 Redis IP 限流可失败放行；下游 gRPC 对业务 API 是硬依赖。
- Auth 可启动但验证码和登录 Token 读写离不开 Redis。
- User 的 provider 虽有 Redis 降级意图，部分资料/二维码路径仍直接使用 Redis，不能承诺完全无 Redis 运行。
- Relation、Group 有 MySQL 事实源与缓存降级路径；改动前仍须核对具体 repository。
- Msg 需要 Redis 与 Kafka 生产能力；Message-push 对 Redis、Group、Connect 的可用性敏感。
- 任何后台任务使用 pkg/async；服务关闭时先停消费者/服务器，再关连接池与客户端。
