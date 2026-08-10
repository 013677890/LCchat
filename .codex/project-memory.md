# LCChat Codex 项目记忆

这是后续进入仓库时的第一读。它只记录稳定边界、入口和高风险约束；代码始终是事实来源。

## 建议阅读顺序

1. 先读本文件。
2. 读 services/overview.md 和本次要改的服务说明。
3. 涉及 HTTP 时读 api-map.md；涉及跨服务副作用时读 flows/ 对应流程和 flows/outbox-and-consumers.md。
4. 涉及运行、生成或验证时读 ops/；提交前读 risks/watchlist.md。

## 导航

- services/overview.md：服务所有权、依赖和降级边界。
- services/：八个服务各自的读优先文件、稳定行为和陷阱。
- api-map.md：Gateway HTTP 路由到下游 RPC 的映射与超时预算。
- flows/message-chain.md：消息发送、撤回、已读和下行。
- flows/account-profile.md：注册、资料展示同步和注销清理。
- flows/relation-lifecycle.md：好友申请、好友关系、黑名单和实时提醒。
- flows/group-lifecycle.md：群写模型、缓存投影、成员资格投影和群实时事件。
- flows/outbox-and-consumers.md：Outbox、Kafka、死信、补偿和幂等。
- ops/runbook.md、ops/testing-map.md：本地启动、配置、生成和验证。
- risks/watchlist.md：修改前必须复核的当前风险。
- memory-roadmap.md：记忆覆盖范围和维护规则。

## 稳定项目事实

- 模块为 github.com/013677890/LCchat-Backend；运行时为 Go 1.25。
- 这是仅含后端的即时通信微服务仓库；客户端在独立仓库。
- HTTP 统一从 Gateway 进入；服务间同步协作使用 gRPC 和 Protobuf。
- 业务数据按服务归属写入，禁止跨服务直接改表。
- MySQL 是业务事实来源；Redis 是缓存、路由、位点或临时状态，不是跨服务写模型。
- 跨服务的可靠业务事件优先使用 MySQL 事务内 Outbox 加 Debezium CDC；实时提醒是明确例外，Relation 和 Group 直接异步写 realtime.push，失败不阻断主流程。
- 本地标准编排是 Docker Compose，包含 MySQL、Redis Stack、Kafka、Kafka Connect、Debezium CDC、MinIO 和八个业务服务。

## 服务与主链路

- Gateway：HTTP 路由、鉴权、限流、超时和 DTO/RPC 转换，不承载领域写规则。
- Auth：账号、验证码、Token、设备会话和账号注销。
- User：资料、头像、二维码和搜索。
- Relation：好友申请、好友关系、备注标签和黑名单。
- Group：群资料、成员、角色、入群申请和群缓存投影。
- Msg：消息、序号、会话、撤回、已读及 msg.push 生产。
- Connect：WebSocket、在线路由、ACK 与内部推送 RPC。
- Message-push：消费 msg.push 与 realtime.push，路由后调用 Connect 下发。

主消息链路为：

Gateway -> Msg RPC -> 业务表与 Outbox -> Debezium/Kafka msg.push -> Message-push -> Connect RPC -> WebSocket。

## 当前必须记住的可靠性语义

- 手动提交 Kafka 消费者成功后才提交 offset；有 DeadLetterSink 时，单次处理默认超时 10 秒、单条重试预算默认 2 分钟。
- 失效补偿只允许整键 DEL。Redis 最终写失败会在 go-redis 内置短重试之后尝试投递 Kafka；Kafka 投递失败只记录日志，不能把该补偿视为已可靠保存。
- auth、user、relation 的 Redis 补偿任务采用严格 JSON 契约；未知字段和尾随 JSON 会作为永久错误写死信，绝不兼容旧字段或旧负载。
- message-push 使用独立的有限本地重试，失败后让出 offset，没有接入 dead_events；消息历史拉取/同步仍是最终自愈路径。
- 生成的 apps/*/pb 文件不可手改；改 proto 后执行 make proto。Wire 的 wire_gen.go 也不可手改。

## 先查代码的位置

1. apps/服务/cmd/main.go、app.go、providers.go：启动、依赖和后台消费者。
2. apps/服务/internal 下的 handler、service/usecase、repository、consumer。
3. proto/服务：RPC 契约与校验规则。
4. docker-compose.yml、config/、deploy/env/chatserver.env.example：运行拓扑和环境变量。
5. 与改动相关的 .codex 文件；doc/ 仅作补充意图，发生冲突时以代码为准。

## 不可假设

- 不要把 Relation 当成 User 的附属模块，也不要让 Connect 消费业务 Kafka。
- 不要假设 Redis 对每个服务都可选；Msg 和 Message-push 的核心路径依赖它。
- 不要把 dead_events 当作自动重放机制；当前没有完整的重放 worker。
- 不要假设所有异常负载都会进死信：部分账号事件消费者仍会记录解码失败并直接提交，详见 flows/outbox-and-consumers.md。
