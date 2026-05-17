# Redis-Key 设计

本文整理当前项目 Redis Key、数据类型、TTL 和使用方。事实来源为 `consts/redisKey/keys.go` 与 `consts/redisKey/msg_ack.go`。

## 1. 总体约定

| 约定 | 说明 |
| --- | --- |
| Key 命名 | 使用模块前缀和业务实体分段，例如 `auth:at:{user_uuid}:{device_id}`。 |
| Redis 定位 | 缓存、Token、验证码、限流、在线路由、幂等和短期 ACK 位点。 |
| 权威事实 | 除 Token、验证码、在线路由等短期状态外，业务事实仍以 MySQL 为准。 |
| 空值缓存 | 资料、关系、群等读侧使用短 TTL 空值缓存防穿透。 |

## 2. TTL 常量

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `VerifyCodeMinuteTTL` | 1 分钟 | 单邮箱发送验证码 1 分钟限流。 |
| `VerifyCode24HTTL` | 24 小时 | 单邮箱 24 小时发送次数限制。 |
| `VerifyCodeIPTTL` | 1 小时 | 单 IP 验证码发送限流。 |
| `DeviceInfoTTL` | 60 天 | 设备信息缓存。 |
| `DeviceActiveTTL` | 7 天 | 设备活跃时间缓存。 |
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
| `auth:at:{user_uuid}:{device_id}` | String | Token 生命周期 | auth/connect | AccessToken MD5。WebSocket 握手强校验该值。 |
| `auth:rt:{user_uuid}:{device_id}` | String | Token 生命周期 | auth | RefreshToken MD5。 |
| `user:devices:{user_uuid}` | Hash/String | 60 天 | auth | 设备信息缓存。 |
| `user:devices:active:{user_uuid}` | Hash/String | 7 天 | auth/connect | 设备活跃时间缓存。 |

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
| `group:members:{group_uuid}` | Set/Hash | 24 小时 | group/message-push/msg | 群成员缓存，群消息扩散依赖。 |
| `group:join_requests:{group_uuid}` | List/Hash/String | 24 小时 | group | 群待审批申请缓存。 |
| `group:info:{group_uuid}` | String(JSON) | 1 小时/空值 5 分钟 | group/gateway/msg | 群资料缓存。 |
| `group:user_groups:{user_uuid}` | Set/List | 24 小时/空值 5 分钟 | group/gateway | 用户加入的群列表缓存。 |

group Key 由 `group.cache` Outbox 投影更新，写入成功和缓存可见之间存在短暂最终一致延迟。

## 7. gateway 限流 Key

| Key | 数据类型 | TTL | 使用方 | 说明 |
| --- | --- | --- | --- | --- |
| `gateway:blacklist:ips` | Set | 长期 | gateway | IP 黑名单。 |
| `gateway:rate:limit:user:{user_uuid}` | String/令牌桶 | 窗口期 | gateway | 登录用户限流。 |
| `rate:limit:ip:{ip}` | String/令牌桶 | 窗口期 | gateway | IP 限流。 |

限流 Redis 不可用时 gateway 当前采取 Fail-Open，避免基础设施短暂异常导致 API 全量不可用。

## 8. connect 路由与 ACK Key

### 8.1 在线路由

```text
key   = user:routing:{user_uuid}
field = {device_id}
value = {connectGrpcAddr}|{lastActiveMs}
TTL   = 约 180 秒
```

| 字段 | 说明 |
| --- | --- |
| `connectGrpcAddr` | 当前设备所在 connect 节点 gRPC 地址。 |
| `lastActiveMs` | 最近活跃时间，Unix 毫秒。 |

connect 在连接建立时写入、心跳时刷新、断开时删除。message-push 根据该路由定位在线设备。

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
3. Key value 格式变更必须同步更新消费方文档和兼容策略；当前项目倾向不向后兼容，通过同步重构收敛复杂度。
4. 所有 Token、验证码、ACK 等敏感或状态类 Key 禁止在日志中输出完整值。
