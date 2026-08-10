# Auth 服务

## 角色

- 拥有账号注册/登录、验证码、密码、邮箱、Token、设备会话和账号注销。
- 消费 profile_display_changed，把昵称/头像冗余回写到登录展示字段。

## 修改前优先阅读

- apps/auth/cmd/providers.go
- apps/auth/internal/service
- apps/auth/internal/repository
- apps/auth/internal/consumer/profile_display_changed_consumer.go
- proto/auth

## 稳定行为

- 注册在同一事务写 user_account 与 user_created Outbox。
- 注销先校验密码，在同一事务软删 user_account 并写 account.deleted Outbox，随后清理设备登录状态。
- FindAccountByEmail 未命中返回 Found=false，不是业务错误。
- UpdateLoginDisplay 是资料展示冗余的唯一消费目标。
- 验证码类型：1=注册、2=验证码登录、3=重置密码、4=改邮箱。

## Redis 与事件

- IP/用户限流可以失败放行；验证码与登录 Token 的读写是实际硬依赖。
- auth.redis.invalidate 只做缓存整键 DEL 补偿，严格任务契约和死信语义见 flows/outbox-and-consumers.md。
- profile_display_changed 使用手动提交消费者并配置 source 为 auth-service:profile_display_changed 的死信。
- 但该消费者对无法解码的消息只记录日志并返回成功，offset 会直接提交；死信只覆盖后续处理/重试失败，不能覆盖坏负载。
