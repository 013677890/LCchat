# auth 服务

auth 服务拥有账号、认证、Token 和设备会话事实，是登录态和账号生命周期的权威服务。

## 职责

- 注册、密码登录、验证码登录、刷新 Token、登出。
- 发送和校验邮箱验证码。
- 修改密码、换绑邮箱、换绑手机号、注销账号。
- 管理设备会话、踢设备、在线状态查询。
- 在注册和注销等关键节点写 Outbox 事件，驱动跨服务最终一致。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/auth/cmd` | 服务启动、gRPC 服务和后台消费者装配。 |
| `apps/auth/internal/service` | 认证、账号、设备业务逻辑。 |
| `apps/auth/internal/repository` | 账号、Token、验证码、设备数据访问。 |
| `proto/auth` | Auth、Account、Device、InternalAuth gRPC 契约。 |

## 数据所有权

| 数据 | 存储 |
| --- | --- |
| 账号凭证和状态 | MySQL `user_account`。 |
| 设备会话 | MySQL `device_sessions`。 |
| RefreshToken | Redis `auth:rt:*`；AccessToken 是无状态 JWT，不落 Redis。 |
| 验证码和限流 | Redis `user:verify_code:*`。 |

## 暴露的 gRPC 服务

| 服务 | 能力 |
| --- | --- |
| `AuthService` | 注册、登录、验证码、刷新、登出、重置密码。 |
| `AccountService` | 修改密码、换绑邮箱、换绑手机号、注销账号。 |
| `DeviceService` | 设备列表、踢设备、在线状态（以 presence 路由为事实源聚合）、设备状态更新。 |
| `InternalAuthService` | 账号查询、登录展示冗余字段回写等内部能力。 |

## 事件

| 事件 | 方向 | 说明 |
| --- | --- | --- |
| `user_created` | 生产 | 注册成功后写 Outbox，驱动 user 初始化资料。 |
| `account.deleted` | 生产 | 注销账号后写 Outbox，驱动下游异步清理。 |
| `profile_display_changed` | 消费 | user 资料展示字段变化后，回写登录冗余字段。 |

## Kafka 消费编排

- `profile_display_changed` 与 `auth.redis.invalidate` 各使用一个独立 `ManualConsumerPool` 和独立 consumer group。
- `KAFKA_AUTH_PROFILE_DISPLAY_CHANGED_CONSUMER_CONCURRENCY`、`KAFKA_AUTH_REDIS_RETRY_CONSUMER_CONCURRENCY` 分别配置本进程 Reader 数；默认 3，显式值必须为 `1～64`。
- workers 由 Kafka rebalance 分配 partition，不接受 partition 绑定。领域事件解码失败与 Redis 补偿毒消息先写 `dead_events`，成功后才提交 offset。
- 消费是 API 旁路：Pool 内 worker 致命退出会先取消兄弟，AuthApp 随后记录指标与 Error 日志并退避重启该 Pool，不中断 gRPC。

## 降级策略

- 登录成功后设备会话写入失败按降级处理；RefreshToken 写入失败必须阻断登录态成立。
- 注销账号同步阶段只保证账号标删和 RefreshToken 清理，下游清理通过 `account.deleted` 最终一致。
- connect 上报在线/离线状态失败时，不影响 WebSocket 连接本身，后续查询可自愈。

## 不变量

- auth 不维护 `user_profile` 的权威资料。
- HTTP 与 WebSocket 都直接校验 AccessToken JWT，不依赖 Redis 中的 AccessToken 状态。
- RefreshToken 是服务端控制续期能力的唯一 Token 状态；刷新时必须重新校验账号状态。
- 修改账号安全信息时不能记录密码、验证码、完整 Token。
