# Gateway 服务

## 角色

- 唯一 HTTP API 入口：路由、JWT、追踪、日志、CORS、限流、请求超时、DTO/RPC 转换和少量读聚合。
- 不拥有业务表，不复写 Auth、User、Relation、Group、Msg 的领域规则。

## 修改前优先阅读

- .codex/api-map.md
- apps/gateway/cmd/providers.go
- apps/gateway/internal/router/router.go
- apps/gateway/internal/router/v1
- apps/gateway/internal/service
- apps/gateway/internal/pb/client_connection.go
- apps/gateway/internal/middleware

## 路由与中间件约束

- 公开接口在 /api/v1/public/user；受保护接口在 /api/v1/auth 并先经过 JWTAuthMiddleware。
- 全局 IP 限流为每秒 10、突发 20，Redis 不可用时失败放行；已登录用户限流为每秒 100、突发 200。
- 改密码、改邮箱、注销有额外每秒 2、突发 5 的用户限流；消息 sync-batch 为每秒 5、突发 10。
- 大多数业务接口显式预算 1 至 5 秒；遗漏 gatewayRequestTimeouts 的新路径回退默认 2 秒。
- /groups/search 等静态路由必须注册在 /groups/:groupUuid 之前。完整表见 api-map.md。

## 聚合约定

- 当前用户身份从认证上下文获得，不信任请求体中可伪造的 owner/from/current-user 字段。
- 他人资料会聚合 User 资料与 Relation 好友状态。
- 搜索分流：邮箱精确检索走 Auth 账号查找加 User 批量资料；非邮箱关键字走 User 搜索并批量补充好友状态。
- 批量资料填充应延续现有去重和有界批量策略；资料补充失败时仅在当前处理已明确允许的读聚合路径降级，不能吞掉领域写错误。

## 修改陷阱

- gRPC 重试策略必须使用精确的 protobuf full method 名称；不可把非幂等写接口加入重试白名单。
- 路由、DTO、超时或认证变化必须同步 api-map.md；改下游所有者时同步更新服务/流程记忆。
- Gateway 返回超时不代表下游一定未执行，写接口的幂等性必须由目标服务与数据库约束保证。
