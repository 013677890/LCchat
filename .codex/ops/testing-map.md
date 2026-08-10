# 验证命令映射

按改动范围选最小充分验证；代码、Proto 或运行配置变更后再扩大范围。仅修改 .codex 时无需跑 Go 测试，但必须检查差异。

## 通用命令

- go test ./...：全量后端单测；默认不依赖真实 MySQL/Redis。
- make proto：修改 proto/ 后生成全部 Protobuf 产物。
- make tidy：仅 Go module 依赖变更后运行。
- git diff --check：检查补丁空白错误。
- git status --short：确认没有误混入用户已有的未提交改动。

## 按服务验证

| 改动范围 | 首选命令 |
| --- | --- |
| Gateway 路由、DTO、middleware、聚合 | go test ./apps/gateway/... |
| Auth | go test ./apps/auth/... |
| User | go test ./apps/user/... |
| Relation | go test ./apps/relation/... |
| Group 与缓存投影/对账 | go test ./apps/group/... |
| Msg、会话、消息工作流、成员资格投影 | go test ./apps/msg/... |
| Connect | go test ./apps/connect/... |
| Message-push | go test ./apps/message-push/... |
| pkg/kafka、pkg/outbox、pkg/redisretry 等共享包 | go test ./pkg/对应包/...，再运行受影响服务测试 |

## Proto 与 Wire

- Proto 源在 proto/；生成输出包括 apps 各服务/pb、pkg/commonpb、pkg/realtimepb。
- 不要手改 .pb.go、.pb.validate.go 或 apps/服务/cmd/wire_gen.go。
- Proto 修改后执行 make tools（首次）和 make proto，再跑涉及的服务测试。
- 改 providers.go 或 wire.go 后重新生成对应 Wire 代码，再验证启动装配与服务测试。

## Kafka 消费者改动的必测语义

手动提交消费者至少覆盖：

1. 成功处理后提交 offset；
2. 可重试失败在同一消息上退避，不跳过失败 offset；
3. 永久错误、超时和 panic 的死信落地成功后才提交；
4. dead_events 落地失败时保持分区阻塞；
5. 幂等重复事件不会重复业务副作用；
6. 账号事件当前存在解码失败直接提交的例外。若修改它，必须显式决定并测试是否改为永久死信。

Redis 补偿改动还应覆盖：任务严格 JSON 解码、当前写命令键提取、只执行 DEL、补偿上下文不递归生产任务，以及 Kafka producer 不可用时的告警/降级。

## 集成与 E2E

- go test -tags=e2e ./tests/e2e：只编译并跳过真实 E2E（未设置 LCCHAT_E2E=1）。
- LCCHAT_E2E=1 go test -tags=e2e -count=1 -v ./tests/e2e：执行真实 Compose E2E，可能创建数据和重启依赖。
- 修改 Outbox、CDC、Kafka Topic 或 Compose 后，额外检查 connector 状态、Topic 分区和消费者日志，而不只看单测。

## 提交前

- 查看 git diff --cached --name-only 与 git diff --check。
- 再看 git status --short，确保只暂存本次文件。
- .codex 文档改动至少核对：引用的入口存在、路由条数/Topic/消费者事实与当前代码一致。
