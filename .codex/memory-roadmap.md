# LCChat Codex 记忆维护路线

## 当前覆盖

项目记忆已按渐进披露组织：

- project-memory.md 是单屏入口，给出稳定架构事实与阅读顺序。
- services/ 覆盖八个服务的数据归属、读优先文件、行为与陷阱。
- api-map.md 覆盖 Gateway 当前全部 HTTP 路由、认证、下游 RPC 和超时预算。
- flows/ 覆盖消息、账号资料、关系、群生命周期以及 Outbox/消费者契约。
- ops/ 覆盖 Compose、环境变量、CDC、生成与测试入口。
- risks/watchlist.md 把当前可见的可靠性和一致性风险变成修改前检查项。

## 已完成的关键校准

- 以 router.go 校准所有 Gateway 路由，并指出默认 2 秒超时和静态/动态路径顺序约束。
- 以 docker-compose.yml 校准八服务、基础设施、业务 Topic、固定 3 分区及默认配置。
- 以 consumer 与 kafka 包校准手动提交、10 秒尝试超时、2 分钟预算和 dead_events 行为。
- 明确 Auth/User/Relation 部分账号事件消费者对解码失败直接提交的例外。
- 明确 Redis 补偿只做整键 DEL、任务严格解码、Kafka 投递失败没有持久后备。
- 补充 Group 的双投影、读修复、默认 6 小时周期对账和 realtime.push 尽力语义。

## 后续维护触发器

发生以下变更时必须同步更新相应记忆：

| 代码变更 | 至少更新 |
| --- | --- |
| Gateway 路由、handler、DTO、超时、认证/限流 | api-map.md，必要时 services/gateway.md 与风险清单 |
| Proto、RPC 所有者、跨服务调用 | 服务说明、对应 flow、api-map.md |
| MySQL 表、Redis key、数据所有权 | services/overview.md、目标服务说明、flow 或风险清单 |
| Outbox event、Kafka Topic、consumer、死信、幂等 | flows/outbox-and-consumers.md、目标服务说明、runbook、风险清单 |
| 推送、ACK、在线路由、重试策略 | message-chain.md、connect/message-push 服务说明、风险清单 |
| Compose、环境变量、端口、CDC、迁移 | ops/runbook.md，必要时 project-memory.md |
| 新产品主流程 | 新增或扩展 flows/文件，并在 project-memory.md 加入口 |

## 写作规则

- 只记录经代码或当前编排确认的稳定事实；不确定内容必须写明需要验证。
- 记忆应指向读优先文件和约束，而不是复制实现细节或真实密钥。
- 每个风险项至少说明：会坏什么、何时检查、先看哪里。
- 新增兼容、重试或降级策略前，先把它写进对应 flow 和风险项，再实现/评审。
- 改生成代码相关内容时，记录 proto 或 Wire 入口，而不是手改生成产物。

## 完成标准

未来一次会话在不扫描全仓库的情况下，可以先从 .codex 快速确定：

1. 行为与数据归哪个服务；
2. 要修改的 HTTP 路由/RPC/事件在哪里；
3. 运行依赖、环境变量和验证命令是什么；
4. 该链路的事实边界、最终一致副作用和已知陷阱是什么。
