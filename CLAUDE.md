# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

LCChat 是 Go 1.25 即时通信后端，微服务架构：Gateway 暴露 HTTP，服务间走 gRPC + Protobuf（含 validate 规则），跨服务一致性靠 Kafka + Outbox + Debezium CDC，存储用 MySQL(GORM)、Redis Stack(含 RedisBloom)、MinIO。依赖注入用 Wire。**代码注释、日志消息、文档全部使用中文**（日志字段 key 保持英文）。

| 服务 | 职责 | 端口 |
| --- | --- | --- |
| `apps/gateway` | HTTP 入口，DTO↔proto 转换，聚合 gRPC | :8080 |
| `apps/auth` | 注册/登录/Token/设备会话/注销 | gRPC :9090 |
| `apps/user` | 用户资料/头像/搜索 | gRPC :9094 |
| `apps/relation` | 好友/黑名单/好友申请 | gRPC :9093 |
| `apps/group` | 群资料/成员/角色/禁言/入群审批 | gRPC :9095 |
| `apps/msg` | 消息落库/seq/会话/撤回/已读 | gRPC :9092 |
| `apps/connect` | WebSocket 长连接/在线路由/ACK | HTTP :8081, gRPC :9091 |
| `apps/message-push` | 消费 msg.push、realtime.push 并调 connect 下发 | HTTP :8084 |

各服务另有 metrics 端口 91xx（`/metrics`）。

## 常用命令

```bash
make tools            # 安装 buf + protoc 插件（版本与 go.mod 对齐）
make proto            # 生成 pb 代码（改 proto/ 后必须执行）
make env              # 复制 deploy/env/chatserver.env.example → chatserver.env
docker compose up -d --build   # 全量启动（基础设施 + 8 个服务）
docker compose up -d mysql redis kafka kafka-connect minio   # 只起基础设施
go run ./apps/<service>/cmd    # 本地单独跑某个服务（需环境变量，见 env.example）

go test ./...                          # 全量测试
go test ./apps/msg/...                 # 单服务测试
go test -run TestXxx ./apps/msg/internal/usecase/ -v   # 单个测试
python scripts/gateway_blackbox_test.py   # 黑盒接口测试（需服务已启动）
```

测试不依赖真实基础设施：Redis 用 miniredis，MySQL 用 sqlite 内存库（见各 `*_test.go`）。

## 生成代码规则（重要）

- `*.pb.go` / `*.pb.validate.go` **不入库**（.gitignore），由 `make proto` 或 Docker builder 阶段的 `buf generate` 产出，落在 `apps/*/pb`、`pkg/commonpb`、`pkg/realtimepb`。克隆后或改 proto 后必须先 `make proto` 才能编译。
- `apps/*/cmd/wire_gen.go` 入库但由 Wire 生成，**不要手改**；改依赖装配时改 `wire.go` / `providers.go`。
- `third_party/validate/validate.proto` 是 buf 的 import 依赖，必须保留在仓库。

## 架构

### 服务边界与数据所有权

原则：**谁拥有数据谁写入**，其他服务只能通过 gRPC 查询、Kafka 事件或缓存投影协作，禁止跨服务改表。auth 拥有 `user_account`/`device_sessions`，user 拥有 `user_profile`，relation 拥有 `user_relations`/好友申请，group 拥有 `groups`/`group_members`/`group_join_requests`，msg 拥有 `message`/`conversation`/`group_conversation`。connect/message-push/gateway 不拥有业务表。完整契约见 `doc/overview/服务边界.md`。

登录展示昵称/头像是 auth 表里的冗余字段，由 user 的 `profile_display_changed` 事件回写。

### Outbox + CDC 事件链路

- 业务服务在**同一 MySQL 事务**内写业务表 + `outbox_events`（`pkg/outbox.InsertEvent`），Debezium 监听 binlog，EventRouter 按 `event_type` 字段路由到同名 Kafka Topic。数据库侧不维护 pending/failed 状态机，无进程内分发循环。
- CDC Outbox 事件：`user_created`、`account.deleted`（auth）、`group.cache`（group）、`msg.push`（msg）。直接 Producer 事件：`profile_display_changed`（user）、`realtime.push`（relation/group）、`redis-retry-queue`。
- 消费端幂等：`idempotent_events` 表，唯一键 `(event_type, event_id)`，用 `pkg/outbox.CheckIdempotent/MarkIdempotent`。
- `group.cache` 有两个独立消费组：group-service 投影 Redis；msg-service 按单群连续版本投影 `conversation.membership_*` 和 `group_conversation.group_status`。两者 group ID 禁止相同，message-push 不参与该链路。
- 毒消息处理：手动提交消费者在有界重试耗尽后旁路到 `dead_events` 表并提交 offset，解除队头阻塞（`pkg/outbox/deadletter.go`、`pkg/kafka/deadletter.go`）。
- connector 由 compose 的 `cdc-init` 服务跑 `scripts/cdc/register_outbox_connector.sh` 注册。

### 消息发送与下行推送链路

发送：gateway → msg `SendMessage` → usecase 调 relation/group 校验权限 → message domain 幂等检查（三元组 `from_uuid+device_id+client_msg_id`，Redis TTL 10 分钟 + DB 唯一约束）→ Redis `INCR msg:seq:{conv_id}` 分配 seq → 同事务写 `message` + outbox(`msg.push`) → conversation domain 更新会话（单聊写扩散；群聊只更新发送方个人行和一条 `group_conversation` 热数据，不扫描全体成员；共享热数据按 seq 整组单调推进）。幂等命中会修复固定数量的相关会话派生行，不恢复群成员写扩散。

群成员会话：group 写事务 → outbox(`group.cache`, schema v2, 单群严格递增版本) → Debezium → Kafka → msg 独立 `GroupMembershipProjector` → 同事务更新成员 `conversation.membership_*` / 共享 `group_conversation`、推进版本并写消费幂等。GROUP 行只允许 Active/Left，`membership_status=0` 不作为旧数据兼容态；当前空库直接使用新基线，无双读、回填或旧结构 fallback。

下行：Debezium → Kafka `msg.push` → message-push 消费（P2P 查接收方路由；群聊调 group 拿成员再扩散；多端同步查发送方其他设备）→ 读取 Redis 路由 `user:routing:{user_uuid}`（Hash, field=device_id, value=`connectAddr|lastActiveMs`, TTL 180s）并按 connect 节点分组 → 节点间有界并发（`MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY`，未配置时默认 32，显式配置非法则启动失败），节点内按用户串行处理：完整用户目标必须调用一次 `PushToUser`，需要排除当前设备的目标逐设备调用 `PushToDevice`；两种 RPC 都是 `EventHandler` 的强制发送契约，不提供旧单设备 sender 的降级路径 → connect 写 WebSocket。

非阻断原则：**`message` 与 `msg.push` Outbox 同事务提交成功即发送成功**；后续会话派生更新失败只 `logger.Warn`，事务提交后的 CDC/Kafka/实时推送失败也不回滚消息。客户端对单会话用 `PullMessages`、登录恢复/重连对多个已有位点的会话用 `BatchSyncMessages` 自愈。connect 是纯管道：不消费 Kafka、不做业务判断。

### ID 体系（易混淆，注意区分）

- **user_uuid / 群 UUID**：Snowflake 十进制字符串 `util.GenIDString()`（`pkg/util/snowflake.go`），DB 存 `char(20)`，**不含连字符**。每个进程必须配唯一 `SNOWFLAKE_NODE_ID`（0-1023；compose 默认 auth=101, user=201, group=301, msg=401），多副本共享节点号会产生重复 ID。
- **msg_id / msg 侧 event_id**：ULID 26 字符（`pkg/id.GenerateULID()`），时间有序。
- **conv_id**：单聊 = `"p2p-" + join(sort(uuid1, uuid2), "-")`，群聊 = 群 UUID。P2P 解析（`SplitN(body, "-", 2)`）**依赖 UUID 内不含 "-" 的前提**，见 `apps/msg/internal/domain/message/service.go` 的 `computeConvId`/`extractPeerUUIDFromP2PConvID`。
- **refresh token**：crypto/rand 32 字节 base64（不是雪花 ID）。

### 服务内分层

通用结构：`cmd`（入口 + wire 装配 + 生命周期）→ `internal/handler`（gRPC/HTTP 薄层，只做参数转换和错误映射）→ `internal/service` → `internal/repository`（MySQL+Redis）；Kafka 消费者在 `internal/consumer`，下游 gRPC 客户端封装在 `internal/*cli`。

msg 服务用 DDD 分层：`handler` → 跨领域操作走 `usecase`（SendMessage/RecallMessage/MarkRead workflow），单领域操作直调 `domain/message` 或 `domain/conversation`；**domain 之间不互相依赖**，只有 usecase 能编排多 domain。

gateway 特有：`internal/dto`（请求/响应 DTO，用 `ConvertToProto*` / `Convert*FromProto` 转换）、`internal/router/v1`（handler）、`internal/service`（gRPC 调用封装）、`internal/middleware`。

## 错误处理与日志约定

- 业务错误统一用 `pkg/apperr`：`apperr.New(code)`（普通业务失败）、`apperr.Wrap(err, code, "中文上下文")`（底层错误上抛）。禁止用 `status.Error(codes.Xxx, ...)` 表达业务错误。
- 跨服务传输：出站 `apperr.ToStatus(apperr.Sanitize(err))`（脱敏，不透传下游 stack）；入站 `apperr.FromStatus(err)`。
- 错误码集中在 `consts/const.go`：10000-29999 业务错误（日志打 Info），>=30000 服务器错误（打 Error+stack）。优先复用现有码。
- **最终请求日志只在边界拦截点输出**：`grpcx.LoggingUnaryInterceptor`、gateway 的 GinLogger / gRPC client logger、Recovery。中间层（handler/service/usecase/domain/repo）不打成功日志、不打可上抛错误的 Error，只允许降级 Warn 和长连接生命周期日志。
- Gateway 响应：业务错误 `result.Fail(...)`，系统/上游错误 `result.FailServer(c, err, code)`（`pkg/result`）。
- 禁止记录 password/verify_code/完整 token；email、telephone 须脱敏（`utils.MaskEmail` 等）。
- 上下文元数据（trace_id/user_uuid/device_id/client_ip）用 `pkg/ctxmeta` 读写，gateway 写入 gRPC metadata，服务端 interceptor 注入 context。

## 文档同步义务

`doc/` 是唯一项目文档入口，**代码变更必须同步更新对应文档**（规则表见 `doc/README.md`）：HTTP 路由/DTO 改动更新 `doc/api/`，proto 改动更新 `doc/datas/Protobuf契约.md`，MySQL/Redis/Kafka 改动更新 `doc/datas/` 对应文档，服务边界/核心流程改动更新 `doc/overview/`、`doc/architecture/`、`doc/flows/`。文档只描述当前代码事实，代码与文档不一致时以代码为准并立即修文档。

事实来源速查：HTTP 路由 `apps/gateway/internal/router/router.go`；错误码 `consts/const.go`；Redis Key `consts/redisKey/`；数据模型 `model/` + `config/mysql/001_schema.sql`；环境变量 `deploy/env/chatserver.env.example`。
