# LCChat

LCChat 是基于 **Go 1.25** 的即时通信后端，采用微服务架构：Gateway 暴露 HTTP API，服务间通过 **gRPC + Protobuf** 同步协作，跨服务副作用与消息下行通过 **Kafka + Outbox + Debezium CDC** 最终一致投递。仓库同时包含 Electron + Vue 桌面客户端（`web/`）。

核心原则：**谁拥有数据谁写入**；其他服务只能通过 gRPC 查询、Kafka 事件或缓存投影协作，禁止跨服务改表。

---

## 目录

- [服务清单](#服务清单)
- [主要能力](#主要能力)
- [技术栈](#技术栈)
- [架构概览](#架构概览)
- [仓库结构](#仓库结构)
- [快速启动](#快速启动)
- [本地开发](#本地开发)
- [测试](#测试)
- [关键配置](#关键配置)
- [健康检查与端口](#健康检查与端口)
- [文档导航](#文档导航)

---

## 服务清单

| 服务 | 目录 | 主要职责 | 默认端口 |
| --- | --- | --- | --- |
| **gateway** | `apps/gateway` | HTTP 统一入口：路由、JWT 鉴权、限流、超时、DTO↔proto、聚合 gRPC | HTTP `:8080` |
| **auth** | `apps/auth` | 注册/登录/验证码/Token/设备会话/账号安全与注销 | gRPC `:9090`，metrics `:9190` |
| **user** | `apps/user` | 用户资料、头像、二维码、搜索、资料卡片 | gRPC `:9094`，metrics `:9194` |
| **relation** | `apps/relation` | 好友申请/列表/备注标签、黑名单、关系判断 | gRPC `:9093`，metrics `:9193` |
| **group** | `apps/group` | 群资料/成员/角色/禁言/入群审批、群缓存投影 | gRPC `:9095`，metrics `:9195` |
| **msg** | `apps/msg` | 消息落库、seq、会话、撤回、已读、推送事件生产 | gRPC `:9092`，metrics `:9192` |
| **connect** | `apps/connect` | WebSocket 长连接、在线路由、心跳、ACK、内部推送入口 | HTTP `:8081`，gRPC `:9091` |
| **message-push** | `apps/message-push` | 消费 `msg.push` / `realtime.push`，查路由后调 connect 下发 | HTTP `:8084` |

### 数据所有权（摘要）

| 服务 | 拥有数据 | 不做什么 |
| --- | --- | --- |
| gateway | 无业务表 | 不写领域规则、不写业务表 |
| auth | `user_account`、`device_sessions` | 不维护公开资料、好友关系 |
| user | `user_profile` | 不改账号凭证、不直接管关系/群成员 |
| relation | `user_relations`、好友申请 | 不查/改账号表 |
| group | `groups`、`group_members`、`group_join_requests` | 不维护消息表、不管理 WebSocket |
| msg | `message`、`conversation`、`group_conversation` | 不维护在线连接、不直接改群成员 |
| connect | 本地连接、Redis 路由/ACK | 不消费 Kafka、不做业务权限判断 |
| message-push | 无业务表 | 不落业务库、不做业务判定 |

完整边界见 [`doc/overview/服务边界.md`](doc/overview/服务边界.md)。

---

## 主要能力

- **账号**：邮箱注册、密码/验证码登录、刷新 Token、登出、改密、改邮箱、账号注销
- **资料**：个人资料、头像上传（MinIO）、二维码名片、用户搜索、批量资料
- **关系**：好友申请与审批、好友列表与增量同步、备注/标签、黑名单
- **群组**：建群/解散、资料与公告、成员邀请/移除/退群、群主转让、管理员角色、全员/单人禁言、入群申请与审批
- **消息**：单聊/群聊发送、拉取、撤回、会话列表、已读、免打扰与置顶
- **实时**：WebSocket 下行、多端同步、设备在线状态、消息 ACK 位点

---

## 技术栈

| 类别 | 技术 |
| --- | --- |
| 语言 / 运行时 | Go 1.25 |
| HTTP / WS | Gin、gorilla/websocket |
| RPC | gRPC + Protobuf + protoc-gen-validate |
| 存储 | MySQL 8（GORM）、Redis Stack 7.4（含 RedisBloom）、MinIO |
| 消息与一致性 | Kafka、Kafka Connect + Debezium（Outbox CDC） |
| 依赖注入 | Google Wire |
| 可观测 | Zap 日志、Prometheus `/metrics` |
| 编排 | Docker Compose；可选 k3s / Kustomize（`deploy/k8s/`） |
| 桌面端 | Electron + Vue（`web/`） |

---

## 架构概览

```text
客户端 (HTTP) ──► gateway ──gRPC──► auth / user / relation / group / msg
客户端 (WS)   ──► connect ◄──gRPC── message-push ◄──Kafka── msg.push / realtime.push
                                         │
                                         └── group（群成员扩散）

业务写路径：MySQL 事务内写业务表 + outbox_events
         ──► Debezium CDC ──► Kafka Topic（按 event_type 路由）
```

### 消息发送与下行（简图）

1. **发送**：gateway → msg `SendMessage` → 校验 relation/group → 幂等（Redis + DB）→ Redis 分配 seq → 同事务写 `message` + outbox(`msg.push`) → 更新会话
2. **下行**：Debezium → Kafka `msg.push` → message-push（P2P 查路由 / 群聊查成员后扩散）→ 按 Redis `user:routing:{uuid}` 调对应 connect 节点 `PushToDevice` → WebSocket 写出
3. **非阻断**：消息落库成功即返回成功；会话更新 / Kafka / 推送失败只告警，客户端靠 `PullMessages` 自愈

### 关键事件

| 事件 | 生产 | 消费 | 用途 |
| --- | --- | --- | --- |
| `user_created` | auth Outbox | user | 注册后初始化资料 |
| `profile_display_changed` | user | auth | 回写登录展示昵称/头像 |
| `account.deleted` | auth Outbox | user / relation / group 等 | 注销后异步清理 |
| `group.cache` | group Outbox | group 投影消费者 | 群写模型 → Redis 投影 |
| `msg.push` | msg | message-push | 消息/撤回/已读等下行 |
| `realtime.push` | relation / group | message-push | 好友、群申请等实时提醒 |

更细的拓扑与链路见 [`doc/architecture/`](doc/architecture/) 与 [`doc/flows/`](doc/flows/)。

---

## 仓库结构

```text
LCChat/
├── apps/                 # 各微服务（cmd + internal + pb）
├── proto/                # Protobuf 源文件（契约源头）
├── pkg/                  # 公共库：apperr、grpcx、kafka、outbox、logger…
├── model/                # GORM 数据模型
├── config/               # 配置加载；mysql/001_schema.sql
├── consts/               # 错误码、Redis Key
├── deploy/
│   ├── env/              # chatserver.env.example
│   ├── k8s/              # Kustomize base + overlays
│   └── nginx/            # 可选 LB 配置
├── scripts/              # CDC 注册、黑盒测试、迁移脚本
├── doc/                  # 项目文档（唯一正式文档入口）
├── web/                  # Electron + Vue 桌面客户端
├── docker-compose.yml
├── Dockerfile            # 多阶段构建：buf generate + 编译全部服务
└── Makefile              # tools / proto / env / docker-up
```

各服务内部分层约定：`cmd`（入口 + Wire）→ `handler`（薄层）→ `service` / `usecase` → `repository`。  
`msg` 采用 DDD：`domain/message`、`domain/conversation` + `usecase` 编排；domain 之间不互相依赖。

---

## 快速启动

### 1. 环境变量

```bash
# Linux / macOS
cp deploy/env/chatserver.env.example deploy/env/chatserver.env

# Windows PowerShell
Copy-Item deploy/env/chatserver.env.example deploy/env/chatserver.env

# 或
make env
```

至少检查：

| 变量 | 说明 |
| --- | --- |
| `JWT_SECRET` | auth / gateway / connect **必须相同**；生产务必替换 |
| `EMAIL_*` / `EMAIL_AUTH_CODE` | 验证码邮件（QQ SMTP 等） |
| `MINIO_*` | 对象存储凭据 |
| `CONNECT_ALLOWED_ORIGINS` | WebSocket Origin 白名单；Electron/浏览器跨域必须配置 |
| `CORS_ALLOWED_ORIGINS` | Gateway CORS 允许来源 |

### 2. 一键启动

```bash
docker compose up -d --build
# 或
make docker-up
```

会启动：MySQL、Redis Stack、Kafka、Kafka Connect、cdc-init（注册 Outbox Connector）、MinIO，以及 8 个业务服务（镜像内预编译二进制）。

### 3. 查看状态

```bash
docker compose ps
docker compose logs -f gateway auth user relation group msg connect message-push
```

### 4. 健康检查

```bash
curl http://127.0.0.1:8080/health   # gateway
curl http://127.0.0.1:8081/health   # connect
curl http://127.0.0.1:8084/health   # message-push
```

仅启动基础设施（例如 k3s 只跑应用）：

```bash
docker compose up -d mysql redis kafka kafka-connect minio
docker compose up --force-recreate cdc-init
```

---

## 本地开发

### 前置：生成 Protobuf

`*.pb.go` / `*.pb.validate.go` **不入库**（由 `make proto` 或 Docker builder 阶段 `buf generate` 产出）。克隆仓库后本地编译前必须生成：

```bash
make tools   # 安装 buf + protoc 插件（版本与 go.mod / Dockerfile 对齐）
make proto   # 生成 apps/*/pb、pkg/commonpb、pkg/realtimepb
```

改 `proto/` 后同样必须重新 `make proto`。

### 单独跑某个服务

建议先起基础设施，再在根目录执行：

```bash
docker compose up -d mysql redis kafka kafka-connect minio
docker compose up --force-recreate cdc-init

# 配置好环境变量后
go run ./apps/auth/cmd
go run ./apps/user/cmd
go run ./apps/gateway/cmd
# …
```

### Wire

`apps/*/cmd/wire_gen.go` 入库，由 Wire 生成，**不要手改**。改依赖装配时改 `wire.go` / `providers.go` 后重新生成。

### 桌面端

```bash
cd web
npm install
npm run dev
```

前端开发说明见 [`web/doc/LCchat-前端开发文档.md`](web/doc/LCchat-前端开发文档.md)。

---

## 测试

```bash
go test ./...                          # 全量（不依赖真实 MySQL/Redis：sqlite + miniredis）
go test ./apps/msg/...                 # 单服务
go test -run TestXxx ./apps/msg/internal/usecase/ -v

# 编译 Docker E2E；未设置 LCCHAT_E2E=1 时只编译并跳过执行
go test -tags=e2e ./tests/e2e

# 完整 Docker Compose 端到端测试，会创建真实数据并重启 Connect、Redis
LCCHAT_E2E=1 go test -tags=e2e -count=1 -v ./tests/e2e
```

覆盖范围、环境变量、按模块运行方式和当前测试基线见
[`doc/ops/端到端功能测试.md`](doc/ops/端到端功能测试.md)。

---

## 关键配置

完整说明见 [`doc/ops/配置说明.md`](doc/ops/配置说明.md) 与 [`deploy/env/chatserver.env.example`](deploy/env/chatserver.env.example)。

### 服务地址（Compose 内网）

| 变量 | 默认 |
| --- | --- |
| `AUTH_SERVICE_ADDR` | `passthrough:///auth:9090` |
| `USER_SERVICE_ADDR` | `passthrough:///user:9094` |
| `RELATION_SERVICE_ADDR` | `passthrough:///relation:9093` |
| `GROUP_SERVICE_ADDR` | `passthrough:///group:9095` |
| `MSG_SERVICE_ADDR` | `passthrough:///msg:9092` |
| `GATEWAY_ADDR` | `:8080` |
| `CONNECT_ADDR` / `CONNECT_GRPC_ADDR` | `:8081` / `:9091` |
| `MESSAGE_PUSH_HTTP_ADDR` | `:8084` |

### Snowflake 节点 ID

每个会调用 `util.GenIDString()` 的进程必须配置**唯一** `SNOWFLAKE_NODE_ID`（0–1023）。Compose 默认：auth=101，user=201，group=301，msg=401。多副本**不能**共享同一节点号。

### 消息推送相关

| 变量 | 说明 |
| --- | --- |
| `KAFKA_MSG_PUSH_TOPIC` / `KAFKA_MSG_PUSH_GROUP_ID` | 消息下行 Topic 与消费组 |
| `MESSAGE_PUSH_ROUTE_TTL_SECONDS` | 在线路由读取过期时间，默认 180s |
| `MESSAGE_PUSH_CONNECT_TIMEOUT_USER_MS` | 调用 connect 超时，默认 150ms |
| `GROUP_GRPC_ADDR` | msg / message-push 访问群服务 |

---

## 健康检查与端口

### 宿主机映射（Docker Compose 默认）

| 组件 | 宿主机端口 |
| --- | --- |
| gateway HTTP | `8080` |
| connect HTTP / WS | `8081` |
| message-push HTTP | `8084` |
| MySQL | `13306` → 3306 |
| Redis Stack | `16379` → 6379 |
| Kafka | `9092` |
| Kafka Connect | `8083` |
| MinIO API / Console | `9000` / `9001` |
| auth / user / relation / group / msg metrics | `19190` / `19194` / `19193` / `19195` / `19192` |
| 对应 gRPC（可选暴露） | `19090` / `19094` / `19093` / `19095` / `19092` |

### API 入口前缀

- 公开：`/api/v1/public/user/*`（登录、注册、验证码等）
- 需登录：`/api/v1/auth/*`（资料、好友、群、消息、会话、设备等）
- WebSocket：connect 服务 `ws://host:8081`（鉴权与帧协议见文档）

HTTP 路由以 [`apps/gateway/internal/router/router.go`](apps/gateway/internal/router/router.go) 为准；接口说明见 [`doc/api/`](doc/api/)。

---

## 文档导航

| 主题 | 入口 |
| --- | --- |
| 文档中心 | [`doc/README.md`](doc/README.md) |
| 项目总览 / 服务边界 / 目录 | [`doc/overview/`](doc/overview/) |
| 架构与协作 | [`doc/architecture/`](doc/architecture/) |
| HTTP / WebSocket 接口 | [`doc/api/`](doc/api/) |
| 各服务说明 | [`doc/services/`](doc/services/) |
| 数据库 / Redis / Kafka / Proto | [`doc/datas/`](doc/datas/) |
| 核心业务流程 | [`doc/flows/`](doc/flows/) |
| 本地运行、配置、监控 | [`doc/ops/`](doc/ops/) |
| k3s 本地部署 | [`doc/ops/k3s本地部署与接口联调.md`](doc/ops/k3s本地部署与接口联调.md) |
| k3s 迁移方案 | [`doc/guides/k3s迁移方案.md`](doc/guides/k3s迁移方案.md) |

**文档维护**：代码变更触及路由、proto、表结构、Redis Key、Kafka 事件、服务边界或部署配置时，必须同步更新 `doc/` 对应文档（规则表见文档中心）。以代码为准，文档漂移时立即修正。

---

## 约定速览

- **错误**：业务错误统一 `pkg/apperr`；跨 gRPC 用 `ToStatus` / `FromStatus`，禁止用裸 `status.Error` 表达业务码
- **日志**：中文消息 + 英文 field key；最终请求日志只在边界拦截点输出；禁止记录 password / 验证码 / 完整 token
- **ID**：用户/群 UUID 为 Snowflake 十进制字符串（无连字符）；消息 ID 为 ULID；P2P `conv_id` = `p2p-{sorted_uuid1}-{sorted_uuid2}`
- **注释与文档**：代码注释、日志消息、项目文档使用中文

---

## License

本项目为学习与实践用途；使用与分发请遵循仓库内相关约定。
