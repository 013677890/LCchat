# 登录注册与 Token

本文描述注册、验证码、密码登录、验证码登录、刷新 Token、登出和 Token 在 HTTP / WebSocket 中的使用方式。对应 HTTP 接口见 [认证与账号接口](../api/01-认证与账号接口.md)。

## 1. 参与组件

| 组件 | 职责 |
| --- | --- |
| gateway | HTTP 入站、DTO 校验、JWT 中间件、统一响应。 |
| auth | 账号校验、Token 签发、验证码管理、设备会话写入。 |
| user | 注册完成后初始化资料。 |
| Redis | RefreshToken、验证码、设备在线状态、限流；AccessToken 不落 Redis。 |
| Kafka / Debezium | `user_created`、`account.deleted` 等最终一致事件。 |

## 2. 注册链路

### 2.1 前置：发送验证码

入口：`POST /api/v1/public/user/send-verify-code`

步骤：

1. gateway 校验 `email` 和 `type`。
2. auth 检查邮箱格式和验证码类型。
3. auth 执行三层限流：
   - 单邮箱 1 分钟限流；
   - 单邮箱 24 小时限流；
   - 单 IP 1 小时限流。
4. auth 生成 6 位验证码，写 Redis：`user:verify_code:{email}:{type}`。
5. auth 通过 SMTP 发送邮件。

相关 Redis Key：

| Key | 说明 |
| --- | --- |
| `user:verify_code:{email}:{type}` | 验证码正文。 |
| `user:verify_code:1m:{email}` | 1 分钟频控。 |
| `user:verify_code:24h:{email}` | 24 小时频控。 |
| `user:verify_code:1h:{ip}` | IP 维度频控。 |

### 2.2 注册本体

入口：`POST /api/v1/public/user/register`

步骤：

1. gateway 校验 `email`、`password`、`verifyCode`、`nickname`、`telephone`。
2. auth 校验邮箱验证码是否正确且未过期。
3. auth 检查邮箱/手机号是否已存在。
4. auth 创建 `user_account`，写入密码哈希和基础状态。
5. auth 在同一事务内写 `outbox_events`，事件类型为 `user_created`。
6. Debezium 将该事件投递到 Kafka `user_created`。
7. user 消费事件，初始化 `user_profile`。
8. HTTP 注册接口立即返回成功，不等待 user 资料初始化完成。

### 2.3 一致性语义

| 阶段 | 结果 |
| --- | --- |
| auth 事务提交成功 | 账号事实成立。 |
| `user_created` 尚未消费 | 资料域可能暂时未初始化。 |
| user 消费完成 | `user_profile` 完整可读。 |

## 3. 密码登录链路

入口：`POST /api/v1/public/user/login`

请求关键字段：

| 字段 | 说明 |
| --- | --- |
| `account` | 邮箱或手机号。 |
| `password` | 明文密码，服务端内部做哈希比对。 |
| `deviceInfo` | 设备名称、平台、系统版本、App 版本。 |
| `X-Device-ID` | 设备唯一 ID，建议稳定传入。 |

步骤：

1. gateway 校验请求体并透传 `X-Device-ID`。
2. auth 按邮箱或手机号定位 `user_account`。
3. auth 校验密码哈希。
4. auth 检查账号状态，已注销账号禁止登录。
5. auth 生成 AccessToken 和 RefreshToken。
6. auth 只将 RefreshToken 写入 Redis `auth:rt:{user_uuid}:{device_id}`；AccessToken 是无状态 JWT。
7. auth 更新或创建 `device_sessions` 记录，写入 `deviceInfo`、IP、平台、版本等。
8. auth 返回 `accessToken`、`refreshToken`、`tokenType`、`expiresIn`、`userInfo`。

### 3.1 登录成功判定

登录成功以 Token 成功签发并写入 Redis 为准。若只是设备附加信息写入失败，不应让前端拿到一个“看似成功但无法后续鉴权”的登录态。

## 4. 验证码登录链路

入口：`POST /api/v1/public/user/login-by-code`

与密码登录的主要区别：

1. 用 `email + verifyCode` 代替 `account + password`。
2. 仍然要校验账号状态。
3. Token 签发、Redis 写入、设备会话更新流程与密码登录一致。

前端可将密码登录与验证码登录统一收敛为“成功拿到 Token 后建立会话”。

## 5. Token 结构与使用

### 5.1 AccessToken

用途：

- HTTP `Authorization: Bearer <access_token>`。
- WebSocket 握手 `GET /ws?token=<access_token>&device_id=<device_id>`。

服务端依赖只有 JWT 签名、有效期和 claims 校验，不查询 Redis 中的 AccessToken 状态。

这意味着：

- 未过期且 claims 合法的 AccessToken 可以访问 HTTP，也可以建立 WebSocket；
- 登出、踢设备和注销只撤销续期能力，旧 AccessToken 会自然使用到 `exp`。

### 5.2 RefreshToken

入口：`POST /api/v1/public/user/refresh-token`

当前 HTTP DTO 请求字段：

| 字段 | 说明 |
| --- | --- |
| `uuid` | 用户 UUID。 |
| `device_id` | 设备 ID。 |
| `refreshToken` | 刷新令牌。 |

刷新成功后返回新的 `accessToken`，通常不返回新的 `refreshToken`。前端应覆盖本地 AccessToken，并继续复用同一设备 ID。

## 6. 登出链路

入口：`POST /api/v1/auth/user/logout`

步骤：

1. gateway 通过 JWT 中间件识别当前用户。
2. auth 根据请求中的 `deviceId` 找到目标设备会话。
3. auth 删除 Redis 中对应的 RefreshToken。
4. auth 更新设备会话状态。
5. 已有 WebSocket 不会主动断开，未过期 AccessToken 仍可重连；Token 到期后设备因无法续期而失效。

## 7. WebSocket 与 Token 的关系

WebSocket 握手使用：

```text
GET /ws?token=<access_token>&device_id=<device_id>
```

服务端校验规则：

1. query `device_id` 必须与 JWT claims 中的 `device_id` 一致。
2. JWT 必须签名有效且未过期，claims 必须包含 `user_uuid`、`device_id`。
3. 握手不读取 Redis，Redis 故障不影响 JWT 鉴权。

因此前端要点是：

- 同一设备登录、刷新 Token、建立 WS 时必须使用同一个 `device_id`；
- Token 刷新后新旧 AccessToken 在各自 `exp` 前都可用于重连；
- 被踢、登出、注销后，客户端应主动清理本地凭据，但服务端接受旧 AccessToken 到期前的有限窗口。

## 8. 常见失败与处理

| 场景 | 典型错误码 | 前端处理 |
| --- | --- | --- |
| 验证码错误 | `11006` | 提示重新输入。 |
| 验证码过期 | `11007` | 引导重新发送验证码。 |
| 密码错误 | `11003` | 提示密码错误。 |
| 账号不存在 | `11017` | 引导注册或检查账号。 |
| 账号已注销 | `11029` | 禁止登录，提示账号状态。 |
| Token 已过期 | `20003` | 尝试刷新 Token。 |
| Token 无效 | `20002` | 清理本地登录态并重新登录。 |
| 发送验证码过于频繁 | `11028` 或 `10005` | 倒计时禁用发送按钮。 |

## 9. 前端实现建议

1. 生成稳定 `device_id`，不要每次刷新页面都变化。
2. `client storage` 中至少保存：`accessToken`、`refreshToken`、`deviceId`、`userInfo.uuid`。
3. 遇到 `20003` 优先刷新 Token，刷新失败再跳转登录。
4. 登录成功后再建立 WebSocket，不要并发抢跑。
5. 注册成功不等于资料域完全初始化，首次拉资料失败时可短暂重试。
