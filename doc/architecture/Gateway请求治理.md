# Gateway 请求治理
Gateway 是所有 HTTP API 的统一入口，当前基于 Gin 实现，路由事实来源是 [apps/gateway/internal/router/router.go](../../apps/gateway/internal/router/router.go)。

## 路由分层

| 路由 | 鉴权 | 用途 |
| --- | --- | --- |
| `GET /health` | 否 | 网关健康检查。 |
| `GET /metrics` | 否 | Prometheus 指标。 |
| `/api/v1/public/user/*` | 否 | 登录、注册、验证码、刷新 Token、二维码解析。 |
| `/api/v1/auth/user/*` | 是 | 资料、设备、账号安全、登出。 |
| `/api/v1/auth/friend/*` | 是 | 好友申请、好友列表、同步、备注标签。 |
| `/api/v1/auth/blacklist/*` | 是 | 黑名单增删查和判断。 |
| `/api/v1/auth/messages/*` | 是 | 发送、拉取、反查、撤回消息。 |
| `/api/v1/auth/conversations/*` | 是 | 会话列表、已读、删除、设置。 |
| `/api/v1/auth/groups/*` | 是 | 群资料、成员、申请、角色、禁言。 |

## 中间件顺序
Gateway 初始化时按以下顺序挂载中间件：

1. Recovery：拦截 panic，避免进程崩溃。
2. Trace：生成或透传 `trace_id`。
3. Client IP：解析真实客户端 IP。
4. GinLogger：最终 HTTP 请求日志。
5. Prometheus：请求指标。
6. CORS：跨域控制。
7. Timeout：按路由配置请求预算。
8. IP Rate Limit：全局 IP 黑名单与限流。
9. JWTAuthMiddleware：仅 `/api/v1/auth` 路由启用。
10. UserRateLimit：仅已登录用户路由启用。

## 超时预算
Gateway 维护显式路由预算，未命中新接口回退到默认 2 秒。

| 预算 | 典型接口 |
| --- | --- |
| 0 | `/health`、`/metrics`，不启用业务超时包装。 |
| 1 秒 | 二维码、在线状态、申请未读数。 |
| 2 秒 | 登录、关系写操作、会话设置、群资料读写。 |
| 3 秒 | 搜索、批量资料、头像上传、发送消息。 |

新增 HTTP 路由时必须同步补充预算，避免长期依赖默认值。

## 限流策略

| 策略 | 范围 | 默认值 | Redis Key |
| --- | --- | --- | --- |
| IP 黑名单 | 全局 | 命中黑名单直接拒绝 | `gateway:blacklist:ips` |
| IP 令牌桶 | 全局 | 10 rps，burst 20 | `rate:limit:ip:{ip}` |
| 用户令牌桶 | 登录用户 | 100 rps，burst 200 | `gateway:rate:limit:user:{user_uuid}` |
| 敏感操作限流 | 修改密码、换绑邮箱、注销账号 | 2 rps，burst 5 | 同用户限流 Key |

Redis 不可用时限流链路 Fail-Open，优先保证核心服务可用。

## 鉴权上下文
认证路由通过 JWT 中间件解析用户身份，并向上下文写入：

- `user_uuid`
- `device_id`
- `trace_id`
- `client_ip`

这些字段会继续透传到后端 gRPC 服务。消息发送、设备状态、Token 管理和多端同步都依赖 `device_id`。

## 响应与错误映射
Gateway 统一使用 `pkg/result` 输出响应：

| 类型 | HTTP 状态码 | Body `code` |
| --- | --- | --- |
| 成功 | 200 | `0` |
| 业务失败 | 200 | `10000-29999` |
| 系统失败 | 500 | `30000-39999` |

业务错误直接返回标准业务码；上游系统错误通过 `result.FailServer` 挂入 Gin 错误链，由最终日志统一记录。

## 下游 gRPC 重试

gateway 出站连接在 `apps/gateway/internal/pb/client_connection.go` 中装配。原则：

- **默认不重试**，只有显式列出的 full method 才写入 ServiceConfig `retryPolicy`。
- 白名单只含只读查询（设备状态、资料、关系列表、群 Get/List/Search/Check、消息 Pull/Get 等）。
- 写接口与 `SendMessage` 不进白名单，避免响应丢失时自动重放产生重复副作用。
- 详细规则与内部客户端对照表见 [调用链路与治理](./调用链路与治理.md)。

## 维护要求
- 新增接口必须同时更新 [api](../api)、本文件的路由分层和超时预算描述。
- 新增中间件必须说明顺序原因，尤其是日志、超时、鉴权、限流之间的先后关系。
- 新增限流或安全策略必须同步更新 [data/Redis-Key设计.md](../data/Redis-Key设计.md) 与 [ops/监控指标.md](../ops/监控指标.md)。
- 新增或变更下游 gRPC 方法时：写方法默认不加配置式重试；只读方法若需重试，必须在 `client_connection.go` 对应白名单中显式追加完整 full method，并同步更新 [调用链路与治理](./调用链路与治理.md)。
