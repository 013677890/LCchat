# User 服务

## 角色

- 拥有用户资料、头像 URL、二维码 Token 映射、搜索、批量资料和内部资料 RPC。
- 消费 user_created、account.deleted；资料展示字段变化通过 Outbox 发给 Auth。

## 修改前优先阅读

- apps/user/cmd/providers.go
- apps/user/internal/service
- apps/user/internal/repository/user_repository.go
- apps/user/internal/consumer/user_created_consumer.go
- apps/user/internal/consumer/account_deleted_consumer.go
- proto/user

## 稳定行为

- CreateProfile 由 user_created consumer 调用，并按用户 UUID 幂等创建。
- UpdateProfile 只更新有值字段；当前 API 不能用空字符串清空 signature 或 birthday。
- birthday 必须是真实 YYYY-MM-DD 日期。
- 资料或头像展示字段更新时，在同一事务更新 user_profile 并写 profile_display_changed Outbox。
- 二维码 Token 在用户映射仍存在时复用，映射 TTL 为 48 小时。
- 资料缓存失效写失败会进入 user.redis.invalidate 的整键 DEL 补偿链路。

## 事件与死信

- user_created 的配置 source 为 user-service:user_created；account.deleted 的配置 source 为 user-service:account.deleted。
- 两者都是手动提交消费者：解码成功后的数据库/业务失败会原地重试，预算耗尽后可进入 dead_events。
- 当前解码失败是例外：consumer 只记录日志并返回成功，offset 会提交，不会进入 dead_events。
- profile_display_changed 由 Auth 消费回写登录展示冗余；User 仍是资料权威来源。

## Redis 降级陷阱

- providers 有 MySQL-only Redis fallback 的意图，但资料读取、批量资料和二维码 repository 路径仍可能直接使用 Redis。
- 因此不能把 User 服务描述成已验证的无 Redis 可用；变更 provider 或部署拓扑前先做真实启动/路径验证。
- apps/user 中还有 legacy/proto-local 的 user.GroupService；当前 Msg 与 Message-push 使用 apps/group，不要把两者混为现役边界。
