# 账号、资料与注销流程

本流程覆盖 Auth 与 User 的边界。账号事实属于 Auth，公开资料事实属于 User，二者只用 RPC 和事件协作。

## 注册到资料创建

1. 客户端调用 POST /api/v1/public/user/register。
2. Gateway 调用 Auth.Register。
3. Auth 在一个 MySQL 事务内写入 user_account 与 user_created Outbox 事件；提交成功即账号创建成功。
4. Debezium CDC 将事件路由到 user_created。
5. User 的手动提交消费者解码事件，先查 idempotent_events，再调用内部 CreateProfile，最后写入幂等标记。
6. 资料创建是异步的。账号已存在并不意味着 User 资料已即时可读；重试、重放和补偿以事件消费为准。

当前例外：user_created 解码失败时 User consumer 只记日志并返回成功，offset 会提交而不会写死信。不要误把 dead_events 当作该类坏事件的追踪来源。

## 资料与展示字段同步

1. 已登录用户通过 GET/PUT /api/v1/auth/user/profile、头像上传等 Gateway 路由访问 User。
2. User 在同一事务更新 user_profile，并在昵称或头像展示字段变化时写 profile_display_changed Outbox。
3. CDC 将事件投递到 profile_display_changed。
4. Auth consumer 以 event_id 做幂等检查，调用 UpdateLoginDisplay 回写 Auth 内的登录展示冗余，再标记已处理。
5. User 的资料缓存更新/失效是最终一致副作用；Redis 最终写失败的 DEL 补偿规则见 outbox-and-consumers.md。

Auth 登录展示字段不是资料权威来源。读写资料、字段校验和头像 URL 规则仍在 User 服务。

## 登录、Token 与设备

- 登录、验证码登录、刷新 Token、登出、设备管理、改密码和改邮箱都由 Auth 处理。
- Gateway 的敏感账号写接口额外使用每秒 2、突发 5 的用户限流。
- 验证码和 Token 的 Redis 状态是 Auth 核心路径依赖；不要按 IP 限流可失败放行的语义推断它们也可以降级。
- Connect 的在线路由/活跃状态与 Auth 的设备会话快照不同，排查在线状态时必须分别查看。

## 注销与异步清理

1. 客户端调用 POST /api/v1/auth/user/delete-account。
2. Auth 校验密码，在同一事务软删 user_account 并写 account.deleted Outbox。
3. Auth 随后清理自身设备登录状态。
4. CDC 将 account.deleted 投递给 User 和 Relation；它们各自软删/清理本域资料、关系和申请，并写 idempotent_events。
5. 不跨服务直接删除对方表；新增清理需求时应由数据拥有服务订阅事件或提供受控 RPC。

当前 User 与 Relation 的 account.deleted consumer 对解码失败会直接提交 offset。解码成功后的数据库失败才会原地重试，并在预算耗尽后写相应 dead_events。

## 改动检查

- 改注册或注销事务时，同时检查 Outbox 插入、event_id、消费者幂等和 CDC connector。
- 改昵称/头像时，同时检查 User 事务、profile_display_changed、Auth 冗余回写和缓存失效。
- 不要把 Auth 的显示冗余或 Redis 缓存当作账号/资料的权威数据。
