# gateway 服务

gateway 是 HTTP API 统一入口，基于 Gin 实现。它只做接入层治理、参数适配和 gRPC 聚合，不拥有业务数据。

## 职责

- 暴露 `/health`、`/metrics` 和 `/api/v1` HTTP 路由。
- 解析 JWT，将 `user_uuid`、`device_id`、`trace_id`、`client_ip` 写入上下文。
- 执行 CORS、请求日志、Prometheus、全局超时、IP 限流、用户限流。
- 将 HTTP DTO 转换为 Protobuf 请求，调用后端 gRPC 服务。
- 将 Protobuf 响应转换为 HTTP 统一响应。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/gateway/cmd` | 服务启动和依赖注入。 |
| `apps/gateway/internal/router/router.go` | 路由、中间件和超时预算事实来源。 |
| `apps/gateway/internal/router/v1` | HTTP handler。 |
| `apps/gateway/internal/service` | gRPC 客户端聚合服务。 |
| `apps/gateway/internal/dto` | HTTP 请求和响应 DTO。 |
| `apps/gateway/internal/middleware` | HTTP 和 gRPC 中间件。 |

## 下游依赖

| 服务 | 用途 |
| --- | --- |
| auth | 登录注册、验证码、Token、设备、账号安全。 |
| user | 用户资料、头像、二维码、搜索。 |
| relation | 好友、黑名单、关系状态。 |
| group | 群资料、成员、入群审批。 |
| msg | 消息和会话。 |
| Redis | IP 黑名单、IP 限流、用户限流。 |
| MinIO | 头像对象上传。 |

## 对外接口

HTTP API 见 [api](../api)。gateway 不暴露业务 gRPC 服务。

## 关键治理

- 每个主要路由都有显式请求预算，默认回退 2 秒。
- `/api/v1/auth` 下所有路由都需要 JWT。
- 全局 IP 限流和登录用户限流均依赖 Redis，Redis 不可用时 Fail-Open。
- 修改密码、换绑邮箱、注销账号使用更严格的用户限流。
- 下游 gRPC **默认不重试**；仅对只读 full method 显式配置重试白名单（见 [调用链路与治理](../architecture/调用链路与治理.md)）。写方法（注册、建群、加好友、发消息等）不会被 gRPC 自动重放。

## 不变量

- gateway 不写业务表。
- gateway 不实现核心业务规则，只做入站校验、参数转换和错误映射。
- 新增路由时必须同步更新 API 文档和超时预算。
