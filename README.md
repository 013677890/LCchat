# LCChat
LCChat 是一个基于 Go 的即时通信后端项目，当前采用多服务拆分架构，覆盖认证、用户资料、关系链、群组、消息、长连接接入与消息下行推送等核心能力。
## 当前服务结构
| 服务 | 目录 | 主要职责 | 默认端口 |
| --- | --- | --- | --- |
| gateway | `apps/gateway` | HTTP 统一入口，聚合下游 gRPC 服务 | `8080` |
| auth | `apps/auth` | 注册、登录、验证码、设备与账号安全 | gRPC `9090`，metrics `9190` |
| user | `apps/user` | 用户资料、账号展示信息 | gRPC `9094`，metrics `9194` |
| relation | `apps/relation` | 好友、黑名单等关系能力 | gRPC `9093`，metrics `9193` |
| group | `apps/group` | 群服务骨架与只读查询能力 | gRPC `9095`，metrics `9195` |
| msg | `apps/msg` | 消息写入、拉取、会话编排 | gRPC `9092`，metrics `9192` |
| connect | `apps/connect` | WebSocket 连接管理与下行投递入口 | HTTP `8081`，gRPC `9091` |
| message-push | `apps/message-push` | 消费 `msg.push`，查路由并调用 connect 推送 | HTTP `8084` |
## 当前重要说明
- `msg` 与 `message-push` 已改为通过 `apps/group/pb.GroupService` 访问群服务。
- 群相关下游调用统一使用 `GROUP_GRPC_ADDR`，不再复用旧的用户服务群接口。
- `apps/group` 当前已具备服务骨架、仓储接口、模型映射与只读查询链路，写入型群管理逻辑仍可继续补充。
## 技术栈
- Go 1.25
- gRPC
- Gin
- GORM + MySQL
- Redis
- Kafka
- Wire 依赖注入
- Docker Compose
## 快速启动
### 1. 准备环境变量
复制示例配置：
```bash
cp deploy/env/chatserver.env.example deploy/env/chatserver.env
```
Windows PowerShell：
```powershell
Copy-Item deploy/env/chatserver.env.example deploy/env/chatserver.env
```
然后按需修改邮箱、MinIO、数据库等配置。
### 2. 使用 Docker Compose 启动
```bash
docker compose up -d --build
```
项目默认会启动以下基础组件与业务服务：
- MySQL
- Redis
- Kafka
- Kafka Connect
- MinIO
- auth / user / relation / group / msg / gateway / connect / message-push
### 3. 查看运行状态
```bash
docker compose ps
docker compose logs -f gateway auth user relation group msg connect message-push
```
## 本地开发启动示例
如果你想单独启动某个服务，可在仓库根目录执行：
```bash
go run ./apps/group/cmd
go run ./apps/msg/cmd
go run ./apps/message-push/cmd
```
建议先保证 MySQL、Redis、Kafka 等基础依赖已启动。
## 关键环境变量
### 群服务
- `GROUP_GRPC_ADDR`：group gRPC 监听地址，默认 `:9095`
- `GROUP_METRICS_ADDR`：group metrics 地址，默认 `:9195`
### 消息服务
- `MSG_GRPC_ADDR`：msg gRPC 监听地址，默认 `:9092`
- `MSG_METRICS_ADDR`：msg metrics 地址，默认 `:9192`
- `RELATION_GRPC_ADDR`：relation gRPC 地址
- `GROUP_GRPC_ADDR`：msg 访问群服务时使用的地址
### 消息推送服务
- `MESSAGE_PUSH_HTTP_ADDR`：message-push HTTP 地址，默认 `:8084`
- `KAFKA_MSG_PUSH_GROUP_ID`：Kafka 消费组 ID
- `MESSAGE_PUSH_ROUTE_TTL_SECONDS`：在线路由读取过期时间
- `MESSAGE_PUSH_CONNECT_TIMEOUT_USER_MS`：调用 connect 的单次超时
- `GROUP_GRPC_ADDR`：message-push 查询群成员时使用的地址
## 文档导航
- 文档索引：[`doc/README.md`](doc/README.md)
- k3s 迁移方案：[`doc/guides/k3s迁移方案.md`](doc/guides/k3s迁移方案.md)
## 后续建议
当前更适合优先继续完善 `group` 服务业务逻辑，例如：
- 建群、解散群
- 加人、移人、成员角色变更
- 群资料更新
- 群侧权限校验与批量查询优化
待 group 业务边界稳定后，再推进 k3s / k3s 的部署迁移，会更少返工。
