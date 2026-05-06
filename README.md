# LCchat Backend

> 基于 Go 的即时通讯后端，采用微服务架构。

## 架构概览

```
Client
  │
  ├─── HTTP ──► Gateway (Gin, :8080)
  │                │
  │        ┌───────┼───────────────┬───────────────┐
  │        ▼       ▼               ▼               ▼
  │     Auth     User           Relation          Msg
  │   (:9090)   (:9094)         (:9093)         (:9092)
  │                │                               │
  │                └───────────────┬───────────────┘
  │                                ▼
  │                           MySQL / Redis
  │
  └─── WebSocket ──► Connect (:8081)
                        │
                     Kafka (msg.push)
```

| 服务 | 职责 | 监听端口 |
|---|---|---|
| `gateway` | HTTP API 入口，JWT 鉴权，限流，转发 gRPC | `:8080` |
| `auth` | 注册、登录、验证码、设备管理、账号安全 | `:9090 (gRPC)` |
| `relation` | 好友、黑名单、好友申请 | `:9093 (gRPC)` |
| `user` | 用户资料、公开资料、搜索、二维码名片 | `:9094 (gRPC)` |
| `msg` | 消息发送/撤回/已读/会话管理 | `:9092 (gRPC)` |
| `connect` | WebSocket 长连接，心跳，消息下行推送 | `:8081 (HTTP/WS)`, `:9091 (gRPC)` |

**基础设施**：MySQL 8.0 · Redis 7.2 · Kafka (KRaft) · MinIO

## 快速启动

### 依赖

- Go 1.22+
- Docker & Docker Compose

### 一键启动（Docker Compose）

```bash
cp deploy/env/chatserver.env.example deploy/env/chatserver.env
# 按需填写 EMAIL_AUTH_CODE 等配置

docker compose up -d
```

服务启动后：
- Gateway API：`http://localhost:8080`
- 健康检查：`http://localhost:8080/health`
- Prometheus 指标：`http://localhost:8080/metrics`

### 本地开发（单独运行某个服务）

```bash
# 先启动基础设施
docker compose up -d mysql redis kafka minio

# 运行 user 服务
go run ./apps/user/cmd/main.go

# 运行 gateway 服务
AUTH_SERVICE_ADDR=localhost:9090 USER_SERVICE_ADDR=localhost:9094 RELATION_SERVICE_ADDR=localhost:9093 MSG_SERVICE_ADDR=localhost:9092 go run ./apps/gateway/cmd/main.go

# 运行 connect 服务
AUTH_GRPC_ADDR=localhost:9090 go run ./apps/connect/cmd/main.go
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `AUTH_SERVICE_ADDR` | `localhost:9090` | auth-service gRPC 地址 |
| `USER_SERVICE_ADDR` | `localhost:9094` | user-service gRPC 地址 |
| `RELATION_SERVICE_ADDR` | `localhost:9093` | relation-service gRPC 地址 |
| `MSG_SERVICE_ADDR` | `localhost:9092` | msg-service gRPC 地址 |
| `GATEWAY_ADDR` | `:8080` | gateway HTTP 监听地址 |
| `AUTH_GRPC_ADDR` | `:9090` | connect 服务连接 auth-service 的地址 |
| `USER_GRPC_ADDR` | `:9094` | msg / message-push 连接 user-service 的地址 |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker 地址（逗号分隔） |
| `KAFKA_MSG_PUSH_TOPIC` | `msg.push` | 消息推送 Kafka topic |
| `GIN_MODE` | `debug` | `debug` \| `release` |

## 项目结构

```
.
├── apps/
│   ├── auth/        认证与账号服务
│   ├── connect/     WebSocket 长连接服务
│   ├── gateway/     HTTP API 网关
│   ├── msg/         消息服务
│   ├── relation/    好友关系与黑名单服务
│   └── user/        用户服务
├── config/          配置结构 & MySQL Schema
├── consts/          错误码、Redis Key 常量
├── deploy/          环境配置文件
├── doc/             项目文档（见 doc/README.md）
├── model/           GORM 数据模型
├── pkg/             公共工具包（kafka/redis/logger/jwt 等）
├── proto/           Protobuf 定义文件
├── docker-compose.yml
└── go.mod
```

## 文档

详见 [doc/README.md](doc/README.md)，包含：
- 架构图与认证流程图
- 消息链路、数据库、Redis Key 设计
- API 接口规范
- 各服务详细文档

## 技术栈

| 类型 | 使用 |
|---|---|
| 语言 | Go 1.22 |
| HTTP 框架 | Gin |
| WebSocket | gorilla/websocket |
| RPC 框架 | gRPC + Protobuf |
| 依赖注入 | Google Wire |
| ORM | GORM (MySQL) |
| 缓存 | Redis (go-redis) |
| 消息队列 | Kafka (segmentio/kafka-go) |
| 对象存储 | MinIO |
| 日志 | Zap |
| 监控 | Prometheus |

## 开发规范

```bash
# 编译所有服务
go build ./...

# 运行单元测试
go test ./...

# 重新生成 Wire 依赖代码（修改 wire.go 后）
wire gen ./apps/<service>/cmd/
```
