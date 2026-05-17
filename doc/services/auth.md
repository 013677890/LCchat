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
| AccessToken / RefreshToken 哈希 | Redis `auth:at:*`、`auth:rt:*`。 |
| 验证码和限流 | Redis `user:verify_code:*`。 |

## 暴露的 gRPC 服务

| 服务 | 能力 |
| --- | --- |
| `AuthService` | 注册、登录、验证码、刷新、登出、重置密码。 |
| `AccountService` | 修改密码、换绑邮箱、换绑手机号、注销账号。 |
| `DeviceService` | 设备列表、踢设备、在线状态、设备活跃和状态更新。 |
| `InternalAuthService` | 账号查询、登录展示冗余字段回写等内部能力。 |

## 事件

| 事件 | 方向 | 说明 |
| --- | --- | --- |
| `user_created` | 生产 | 注册成功后写 Outbox，驱动 user 初始化资料。 |
| `account.deleted` | 生产 | 注销账号后写 Outbox，驱动下游异步清理。 |
| `profile_display_changed` | 消费 | user 资料展示字段变化后，回写登录冗余字段。 |

## 降级策略

- 登录成功后设备会话和活跃时间写入失败，应按业务重要性区分是否阻断；Token 写入失败必须阻断登录态成立。
- 注销账号同步阶段只保证账号标删和 Token 清理，下游清理通过 `account.deleted` 最终一致。
- connect 上报在线/离线状态失败时，不影响 WebSocket 连接本身，后续查询可自愈。

## 不变量

- auth 不维护 `user_profile` 的权威资料。
- WebSocket 鉴权依赖 auth 写入 Redis 的 AccessToken 哈希。
- 修改账号安全信息时不能记录密码、验证码、完整 Token。
