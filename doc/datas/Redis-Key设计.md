# Redis-Key 设计

本文整理当前项目 Redis Key、数据类型、TTL 和使用方。事实来源为 `consts/redisKey/keys.go` 与 `consts/redisKey/msg_ack.go`。

## 1. 总体约定

| 约定 | 说明 |
| --- | --- |
| Key 命名 | 使用模块前缀和业务实体分段，例如 `auth:rt:{user_uuid}:{device_id}`。 |
| Redis 定位 | 缓存、RefreshToken、验证码、限流、在线路由、幂等和短期 ACK 位点。AccessToken 不落 Redis。 |
| 权威事实 | 除 Token、验证码、在线路由等短期状态外，业务事实仍以 MySQL 为准。 |
| 空值缓存 | 资料、关系、群等读侧使用短 TTL 空值缓存防穿透。 |

## 2. TTL 常量

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `VerifyCodeMinuteTTL` | 1 分钟 | 单邮箱发送验证码 1 分钟限流。 |
| `VerifyCode24HTTL` | 24 小时 | 单邮箱 24 小时发送次数限制。 |
| `VerifyCodeIPTTL` | 1 小时 | 单 IP 验证码发送限流。 |
| `DeviceInfoTTL` | 60 天 | 设备信息缓存。 |
| `UserProfileTTL` | 1 小时 | 用户资料缓存。 |
| `UserProfileEmptyTTL` | 5 分钟 | 用户资料空值缓存。 |
| `FriendRelationTTL` | 24 小时 | 好友关系缓存。 |
| `FriendRelationEmptyTTL` | 5 分钟 | 好友关系空值缓存。 |
| `BlacklistTTL` | 24 小时 | 黑名单缓存。 |
| `BlacklistEmptyTTL` | 5 分钟 | 黑名单空值缓存。 |
| `ApplyPendingTTL` | 24 小时 | 好友申请待处理缓存。 |
| `ApplyPendingEmptyTTL` | 5 分钟 | 好友申请空值缓存。 |
| `ApplyUnreadNotifyTTL` | 7 天 | 好友申请未读计数。 |
| `QRCodeTTL` | 48 小时 | 用户二维码 token。 |
| `GroupInfoTTL` | 1 小时 | 群资料缓存。 |
| `GroupInfoEmptyTTL` | 5 分钟 | 群资料空值缓存。 |
| `GroupMembersTTL` | 24 小时 | 群成员缓存。 |
| `GroupJoinRequestTTL` | 24 小时 | 群待审批申请缓存。 |
| `UserGroupListTTL` | 24 小时 | 用户群列表缓存。 |
| `MsgIdempotentTTL` | 10 分钟 | 消息发送幂等缓存。 |
| `MsgDeviceAckTTL` | 30 天 | 设备消息 ACK 位点。 |

## 3. auth 与验证码 Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `user:verify_code:{email}:{type}` | String | 验证码有效期 | auth | 邮箱验证码。 |
| `user:verify_code:1m:{email}` | String/计数 | 1 分钟 | auth | 邮箱维度短期限流。 |
| `user:verify_code:24h:{email}` | String/计数 | 24 小时 | auth | 邮箱维度日限流。 |
| `user:verify_code:1h:{ip}` | String/计数 | 1 小时 | auth | IP 维度验证码限流。 |
| `auth:rt:{user_uuid}:{device_id}` | String | 7 天 | auth | 当前设备 RefreshToken；用于续期与主动撤销。 |
| `user:devices:{user_uuid}` | Hash/String | 60 天 | auth | 设备信息缓存；field 内 `loginAt` 为最后一次状态迁移时刻（登录/上线/下线），是离线设备 last_seen 的缓存来源。 |

## 4. user 资料 Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `user:profile:{uuid}` | String(JSON) | 1 小时/空值 5 分钟 | user/gateway | 用户资料缓存。 |
| `user:qrcode:token:{token}` | String | 48 小时 | user | 二维码 token 到用户 UUID 的映射。 |
| `user:qrcode:user:{user_uuid}` | String | 48 小时 | user | 用户 UUID 到二维码 token 的映射。 |

## 5. relation Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `user:relation:friend:{user_uuid}` | Set/Hash | 24 小时 | relation/msg | 好友关系缓存。 |
| `user:relation:blacklist:{user_uuid}` | Set/Hash | 24 小时 | relation/msg | 黑名单缓存。 |
| `user:apply:pending:{target_uuid}` | String/Set | 24 小时 | relation | 目标用户待处理好友申请缓存。 |
| `user:notify:friend_apply:unread:{uuid}` | String/计数 | 7 天 | relation/gateway | 好友申请未读数。 |

## 6. group Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `group:members:<group_uuid>` | Hash | 24 小时 | group/message-push/msg | `__SCHEMA__=2`、`__VERSION__`、`__COMPLETE__=1` 加成员 field；版本 Lua 原子更新。 |
| `group:join_requests:<group_uuid>` | Hash | 24 小时 | group | `__SCHEMA__=2`、`__VERSION__`、`__COMPLETE__=1` 加待审批 `apply_id` field。 |
| `group:info:<group_uuid>` | String | 1 小时/负缓存 5 分钟 | group/gateway/msg | 固定格式 `2\|<projection_version>\|<json>`；负缓存为 `2\|0\|__NOT_FOUND__`。 |
| `group:user_groups:{<user_uuid>}` | ZSet | 24 小时 | group/gateway | member 为群 UUID、score 为群更新时间；花括号是 Redis Cluster hash tag。 |
| `group:user_group_versions:{<user_uuid>}` | Hash | 24 小时 | group | 逐群版本 tombstone；`__READY__=1` 才表示用户群列表完整。 |
| `group:user_groups_reconcile_lease:{<user_uuid>}` | String | 1 小时 | group | READY 命中后的用户级权威对账限频租约；不承载业务事实。 |

group Key 由 `group.cache` Outbox projector 或带 `groups.cache_version` 的 DB 全量对账写入。所有状态写都通过 Lua 比较版本并原子修改；Kafka 增量只接受更高版本，权威对账可用相同版本修复损坏值但仍拒绝更低版本。成员点查与用户群 ZSet/版本 Hash 联读也使用 Lua 获取原子快照。Hash 的 `__COMPLETE__=1` 是完整重建凭证，增量 patch 只在 schema、version、complete 都合法时执行。旧格式或缺少任一必需元数据的缓存直接失效，不做兼容读取。

## 7. gateway 限流 Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `gateway:blacklist:ips` | Set | 长期 | gateway | IP 黑名单。 |
| `gateway:rate:limit:user:{user_uuid}` | String/令牌桶 | 窗口期 | gateway | 登录用户限流。 |
| `rate:limit:ip:{ip}` | String/令牌桶 | 窗口期 | gateway | IP 限流。 |

限流 Redis 不可用时 gateway 当前采取 Fail-Open，避免基础设施短暂异常导致 API 全量不可用。

## 8. connect 路由与 ACK Key

### 8.1 在线路由（presence 契约）

```text
key   = user:routing:{user_uuid}
field = {device_id}
value = {connectGrpcAddr}|{lastActiveMs}
TTL   = CONNECT_ROUTE_TTL_SECONDS（默认 360 秒）
```

| 字段 | 说明 |
| --- | --- |
| `connectGrpcAddr` | 当前设备所在 connect 节点 gRPC 地址。 |
| `lastActiveMs` | 最近活跃时间，Unix 毫秒；新鲜度约等于客户端心跳周期。 |

该 Key 是"设备是否可达"的唯一事实源（presence 契约）：

- **唯一写方 connect**：连接建立无条件写入、**每个应用层心跳无条件刷新**、断开 CAS 删除、节点退出全量清理。
- **消费方**（统一经 `pkg/presence` 读取，按各自窗口过滤 `lastActiveMs`）：
  - message-push：推送寻址，窗口 `MESSAGE_PUSH_ROUTE_TTL_SECONDS`（默认 360s，宁可多尝试投递）；
  - auth：在线状态聚合，窗口 `PRESENCE_ONLINE_WINDOW_SECONDS`（默认 120s，连续丢约 4 个心跳判离线）。
- 参数约束：在线窗口 ≤ 推送窗口 ≤ 路由 TTL。

### 8.2 设备 ACK

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `msg:ack:{user_uuid}:{device_id}:{conv_id}` | String | 30 天 | connect | 设备对会话的最大 ACK seq。 |

ACK 使用 Lua 脚本单调合并，只允许位点前进，不允许旧 ACK 覆盖新 ACK。

## 9. msg Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `msg:seq:{conv_id}` | String | 无 | msg | Redis INCR 分配会话内 seq。 |
| `msg:idempotent:{from_uuid}:{device_id}:{client_msg_id}` | String(JSON) | 10 分钟 | msg | 消息发送幂等结果缓存。 |

## 10. 维护规则

1. 新增 Redis Key 必须在 `consts/redisKey` 中封装构造函数，禁止散落字符串拼接。
2. 修改 TTL 必须同步评估业务语义、缓存穿透风险和前端体验。
3. Key value 格式变更必须同步提高 schema 并协调切换；当前 group v2 明确禁止兼容读取旧格式，也禁止新旧服务混跑。
4. 所有 Token、验证码、ACK 等敏感或状态类 Key 禁止在日志中输出完整值。
