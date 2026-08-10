# 运行手册

本文件覆盖本地启动、环境变量、CDC、生成与常见排障。标准本地拓扑以根目录 docker-compose.yml 为准。

## 环境与准备

- 示例环境文件：deploy/env/chatserver.env.example。
- 本地环境文件：deploy/env/chatserver.env；可用 make env 创建。
- 不要把真实密钥、邮箱授权码或 Token 写入 .codex、日志或提交。
- Auth、Gateway、Connect 必须使用相同 JWT_SECRET。
- 浏览器/桌面端联调需配置 CORS_ALLOWED_ORIGINS 与 CONNECT_ALLOWED_ORIGINS。
- 每个会生成 Snowflake ID 的进程/副本必须有唯一 SNOWFLAKE_NODE_ID；Compose 默认 Auth=101、User=201、Group=301、Msg=401。

## 常用命令

- make env：从示例创建环境文件。
- make mod：下载 Go 依赖。
- make tools：安装 buf 与 protoc 插件。
- make proto：生成 Protobuf Go 代码；改 proto 后必须执行。
- make tidy：整理 go.mod/go.sum。
- make docker-up：构建并启动全套 Compose。
- make docker-down：停止 Compose。
- go test ./...：全量单测；默认使用 sqlite/miniredis，不要求真实基础设施。
- go test -tags=e2e ./tests/e2e：编译 E2E；LCCHAT_E2E=1 时执行真实 Docker Compose E2E。

仅本地运行服务时，先启动基础设施并初始化 Topic/CDC：

1. docker compose up -d mysql redis kafka kafka-connect minio
2. docker compose up --force-recreate kafka-topics-init cdc-init
3. 依次或按依赖运行 go run ./apps/服务/cmd

服务入口：gateway、auth、user、relation、group、msg、connect、message-push 都位于 apps/服务/cmd。

## Compose 端口

| 组件 | 宿主机默认端口 |
| --- | --- |
| Gateway HTTP | 8080 |
| Connect HTTP/WebSocket、gRPC | 8081、19091 |
| Message-push HTTP 健康检查 | 8084 |
| Auth gRPC/metrics | 19090、19190 |
| Msg gRPC/metrics | 19092、19192 |
| Relation gRPC/metrics | 19093、19193 |
| User gRPC/metrics | 19094、19194 |
| Group gRPC/metrics | 19095、19195 |
| MySQL、Redis、Kafka、Kafka Connect | 13306、16379、9092、8083 |
| MinIO API/Console | 9000、9001 |

Connect 的 CONNECT_SELF_GRPC_ADDR 用于节点自身推送 RPC 路由；多副本或改服务发现时必须同步核对它。

## Kafka、CDC 与缓存配置

- kafka-topics-init 用固定 3 分区创建业务 Topic。不要由应用在线扩分区，特别是 group.cache。
- Outbox connector 注册脚本为 scripts/cdc/register_outbox_connector.sh；检查 KAFKA_CONNECT_URL、DEBEZIUM_CONNECTOR_NAME、DEBEZIUM_MYSQL_*、DEBEZIUM_TOPIC_PREFIX、DEBEZIUM_OUTBOX_TABLE。
- MySQL DSN 默认应含 timeout=5s、readTimeout=10s、writeTimeout=10s；pkg/mysql 会补充缺失默认值。
- Group 缓存周期对账：GROUP_CACHE_RECONCILE_INTERVAL 默认 6h，GROUP_CACHE_RECONCILE_BATCH_SIZE 默认 100；非法显式值会阻止 Group 启动。
- Group 和 Msg 的 group.cache 投影并发默认各为 3；worker 数不应被当作分区扩容方案。
- Redis 失效补偿 Topic 为 auth.redis.invalidate、user.redis.invalidate、relation.redis.invalidate；任务只允许 DEL 回放。

## 排障顺序

- 服务不启动：先查 docker compose ps、服务依赖健康检查、实际 env 和对应 apps/服务/cmd/providers.go。
- Outbox 事件未到：查 MySQL outbox_events、Kafka Connect connector 状态、Topic 名称与 consumer group。
- 某分区停滞：查消费日志和 dead_events；注意部分账号事件的解码错误会直接提交，不会出现在 dead_events。
- 缓存异常：先确认 MySQL 事实，再查 Redis key、redis.invalidate Topic、补偿 consumer 与 Kafka 可用性；Kafka 投递失败的补偿没有持久后备。
- 群成员/缓存不一致：查 group.cache 事件、Group/Msg 两个消费组、读取触发的修复以及 CacheReconciler 日志。
- 消息或实时下行缺失：查 msg.push/realtime.push、Message-push 日志、Redis 路由、Group 目标查询、Connect gRPC 地址和 WebSocket ACK；随后用消息拉取/同步或业务查询验证自愈。
- 账号或设备状态异常：区分 Auth 的设备会话快照与 Connect 的实时在线路由。
