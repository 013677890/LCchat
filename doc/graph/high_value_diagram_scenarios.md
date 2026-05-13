# 高价值画图场景清单（跨组件复杂链路）
> 选择标准：只收录跨 3 个以上组件、存在异步/降级/幂等/状态跃迁的链路。
> 不收录简单 CRUD 接口（看代码即可理解）。
## 1. 核心链路时序图（Sequence Diagram）
- [ ] `P0` 登录与鉴权全流程（密码登录）
  `Client -> Gateway -> User(AuthService) -> Redis(Token) + MySQL(DeviceSession) + Redis(active)`；包含登录即写活跃时间。
  代码锚点：`apps/gateway/internal/service/auth_service.go`、`apps/user/internal/service/auth_service.go`、`apps/user/internal/repository/device_repository.go`
- [ ] `P0` 验证码登录全流程（含验证码限流与验证码消耗）
  `Gateway -> User(AuthService) -> Redis(verify_code + rate_limit) -> MySQL/Redis(session+token)`。
  代码锚点：`apps/user/internal/service/auth_service.go`、`apps/user/internal/repository/auth_repository.go`
- [ ] `P0` RefreshToken 链路（上下文透传 + Redis 校验 + 设备缓存续期）
  `Gateway(JWT/metadata) -> User(AuthService) -> Redis(refresh/access) -> device_info TTL`。
  代码锚点：`apps/gateway/internal/middleware/grpc_metadata.go`、`apps/user/internal/service/auth_service.go`
- [ ] `P0` WebSocket 握手鉴权链路（Connect）
  `Client WS -> Connect handler -> JWT Parse -> Redis accessToken MD5 校验(fail-open) -> ConnectionManager Register`。
  代码锚点：`apps/connect/internal/handler/ws_handler.go`、`apps/connect/internal/svc/auth.go`、`apps/connect/internal/manager/connection_manager.go`
- [ ] `P0` 设备保活心跳链路（你当前重点）
  `Gateway/Connect Touch -> 分片节流 map -> pending map -> flush -> gRPC UpdateDeviceActive(批量) -> User BatchSetActiveTimestamps -> Redis ZSet`。
  代码锚点：`pkg/deviceactive/cache.go`、`apps/gateway/internal/middleware/device_active.go`、`apps/connect/internal/svc/lifecycle.go`、`apps/user/internal/service/device_service.go`、`apps/user/internal/repository/device_repository.go`
- [ ] `P0` 断连状态同步链路（异步队列）
  `Connect OnDisconnect -> statusQueue(channel) -> worker -> UpdateDeviceStatus RPC -> User UpdateOnlineStatus(DB+Redis cache)`。
  代码锚点：`apps/connect/internal/svc/lifecycle.go`、`apps/connect/internal/svc/connect_service.go`、`apps/user/internal/service/device_service.go`、`apps/user/internal/repository/device_repository.go`
- [ ] `P0` 批量在线状态查询链路（你刚做的批量优化）
  `Gateway BatchGetOnlineStatus -> User BatchGetOnlineStatus -> BatchGetOnlineStatus(session缓存/DB) + BatchGetActiveTimestamps + BatchGetLastSeenTimestamps`。
  代码锚点：`apps/gateway/internal/service/device_service.go`、`apps/user/internal/service/device_service.go`、`apps/user/internal/repository/device_repository.go`
- [ ] `P0` Redis 写失败重试链路（Kafka 补偿）
  `Repository Redis写失败 -> 构造 RedisTask -> Kafka -> RedisRetryConsumer -> 重试/最大重试放弃`。
  代码锚点：`apps/user/internal/repository/*.go`（`LogAndRetryRedisError`调用点）、`apps/user/mq/redis_task.go`、`apps/user/mq/manager.go`、`apps/user/mq/redis_consumer.go`、`apps/user/cmd/main.go`
- [ ] `P1` 好友申请发送链路（多前置校验 + 缓存更新）
  `FriendService SendFriendApply -> IsFriend/ExistsPending/IsBlocked 双向检查 -> CreateApply -> Redis pending/unread`。
  代码锚点：`apps/user/internal/service/friend_service.go`、`apps/user/internal/repository/apply_repository.go`、`apps/user/internal/repository/friend_repository.go`、`apps/user/internal/repository/blacklist_repository.go`
- [ ] `P1` 同意好友申请链路（事务 + CAS 幂等）
  `HandleFriendApply -> AcceptApplyAndCreateRelation(tx)`，包含申请状态 CAS、双向关系 upsert、异步缓存增量更新。
  代码锚点：`apps/user/internal/service/friend_service.go`、`apps/user/internal/repository/apply_repository.go`
- [ ] `P1` 好友申请列表读取链路（冷热分离 + 异步已读）
  `GetPendingList(status=0优先Redis ZSet, miss回源DB) -> MarkAsReadAsync -> ClearUnreadCount`。
  代码锚点：`apps/user/internal/service/friend_service.go`、`apps/user/internal/repository/apply_repository.go`
- [ ] `P1` 搜索用户聚合链路（跨域聚合）
  `Gateway SearchUser -> User SearchUser -> Gateway BatchCheckIsFriend 补充 isFriend`。
  代码锚点：`apps/gateway/internal/service/user_service.go`、`apps/user/internal/service/user_service.go`、`apps/user/internal/service/friend_service.go`
- [ ] `P1` 他人资料聚合链路（并发调用 + 脱敏）
  `Gateway GetOtherProfile 并发调用 UserProfile + CheckIsFriend -> 非好友脱敏邮箱/手机号`。
  代码锚点：`apps/gateway/internal/service/user_service.go`
- [ ] `P1` 黑名单读写链路（ZSet cache-aside + 条件增量）
  `Add/RemoveBlacklist` 与 `IsBlocked/GetBlacklistList` 的 Redis 命中/回源/异步回填。
  代码锚点：`apps/user/internal/repository/blacklist_repository.go`
- [ ] `P1` Connect 内部推送链路（RPC -> ConnectionManager）
  `PushToDevice/PushToUser/Broadcast/KickConnection` 到连接索引与投递结果。
  代码锚点：`apps/connect/internal/grpc/server.go`、`apps/connect/internal/manager/connection_manager.go`
- [ ] `P0(规划)` 端到端消息链路（JSON 上行 + Proto 下行）
  `Client(JSON) -> Gateway -> MsgService -> Kafka -> Connect -> Client(Binary Proto)`。
  文档/协议锚点：`doc/消息链路设计(JSON上行_Proto下行).md`、`proto/msg/msg_service.proto`、`proto/connect/connect.proto`
## 2. 状态机流转图（State Diagram）
- [ ] `P0` 设备连接状态机（Connect 视角）
  `Connecting -> Authenticated -> Online -> HeartbeatActive -> Disconnected -> Reconnecting/Closed`。
  代码锚点：`apps/connect/internal/handler/ws_handler.go`、`apps/connect/internal/svc/lifecycle.go`、`apps/connect/internal/manager/client.go`
- [ ] `P0` 设备业务状态机（User 视角）
  `Online(0) <-> Offline(1)`，以及 `LoggedOut(2)`、`Kicked(3)` 语义和触发源。
  代码锚点：`apps/user/internal/service/device_service.go`、`apps/user/internal/service/auth_service.go`、`apps/user/internal/repository/device_repository.go`
- [ ] `P0` 设备活跃/在线判定状态机
  `Active(within window) -> Stale(out of window) -> Offline`，含 `lastSeenAt` 读取规则。
  代码锚点：`pkg/deviceactive/cache.go`、`config/device_active.go`、`apps/user/internal/service/device_service.go`
- [ ] `P1` 好友申请状态机
  `Pending(0) -> Accepted(1) / Rejected(2)`，`is_read` 独立流转与红点清理。
  代码锚点：`apps/user/internal/service/friend_service.go`、`apps/user/internal/repository/apply_repository.go`
- [ ] `P1` 用户关系状态机
  `none -> friend(0) -> deleted(2) -> blacklist(1/3) -> recover`。
  代码锚点：`apps/user/internal/repository/friend_repository.go`、`apps/user/internal/repository/blacklist_repository.go`
- [ ] `P1` Redis 重试任务状态机
  `NewTask -> KafkaQueued -> Consuming -> Retry(n) -> Success/Discard(max_retries)`。
  代码锚点：`apps/user/mq/redis_task.go`、`apps/user/mq/redis_consumer.go`
- [ ] `P2` Gateway 到 User gRPC 熔断状态机
  `Closed -> Open -> HalfOpen -> Closed/Open`（含触发阈值）。
  代码锚点：`apps/gateway/internal/pb/client.go`
## 3. 数据流与存储拓扑图（Data Flow / Architecture Diagram）
- [ ] `P0` 设备在线体系总览图（最值得先画）
  `Gateway/Connect/User` 与 `MySQL(device_session)`、`Redis(user:devices, user:devices:active, auth:at/rt)` 的写读路径。
  代码锚点：`apps/gateway/cmd/main.go`、`apps/connect/cmd/main.go`、`apps/user/cmd/main.go`、`apps/user/internal/repository/device_repository.go`
- [ ] `P0` 活跃时间同步拓扑（双 map + 批量 RPC）
  节流 map、缓冲 map、flush、worker、批量 RPC、Redis Pipeline 的关系图。
  代码锚点：`pkg/deviceactive/cache.go`、`apps/gateway/internal/middleware/device_active.go`、`apps/connect/internal/svc/lifecycle.go`
- [ ] `P0` Redis 重试补偿拓扑
  `Repository -> Kafka Producer -> Topic -> Consumer -> Redis`，含失败再入队。
  代码锚点：`apps/user/cmd/main.go`、`apps/user/mq/*.go`
- [ ] `P1` 社交缓存拓扑（Friend/Blacklist/Apply）
  `Hash + ZSet + Counter` 三套缓存模型、空值占位、回源与增量更新策略。
  代码锚点：`apps/user/internal/repository/friend_repository.go`、`apps/user/internal/repository/blacklist_repository.go`、`apps/user/internal/repository/apply_repository.go`
- [ ] `P1` Gateway 请求治理链路拓扑
  `trace_id -> IP限流 -> JWT -> 用户限流 -> gRPC metadata透传 -> user服务`。
  代码锚点：`apps/gateway/internal/router/router.go`、`apps/gateway/internal/middleware/*.go`、`pkg/grpcx/metadata.go`
- [ ] `P1` Connect 推送能力拓扑
  `外部RPC调用方 -> Connect gRPC -> ConnectionManager(bucket)` 的单播/多播/踢线路径。
  代码锚点：`apps/connect/internal/grpc/server.go`、`apps/connect/internal/manager/connection_manager.go`
- [ ] `P0(规划)` 消息服务全链路拓扑
  `Gateway + Msg + Kafka + Connect + Client`，明确协议边界（JSON/Proto/Binary）。
  文档/协议锚点：`doc/消息链路设计(JSON上行_Proto下行).md`、`proto/msg/msg_service.proto`、`proto/connect/connect.proto`
## 4. 建议的出图顺序（最小闭环）
- [ ] 第 1 张：设备保活心跳流转时序图（P0）
- [ ] 第 2 张：设备在线体系数据流拓扑图（P0）
- [ ] 第 3 张：设备状态机图（P0）
- [ ] 第 4 张：Redis 重试补偿时序图（P0）
- [ ] 第 5 张：登录与握手鉴权全流程时序图（P0）
