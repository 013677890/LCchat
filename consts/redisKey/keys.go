package rediskey

import (
	"fmt"
	"time"
)

// ==================== TTL 常量 ====================
const (
	// VerifyCodeMinuteTTL 验证码 1 分钟限流 TTL
	VerifyCodeMinuteTTL = 1 * time.Minute
	// VerifyCode24HTTL 验证码 24 小时限流 TTL
	VerifyCode24HTTL = 24 * time.Hour
	// VerifyCodeIPTTL 验证码 IP 1 小时限流 TTL
	VerifyCodeIPTTL = 1 * time.Hour
	// DeviceInfoTTL 设备信息缓存 TTL
	DeviceInfoTTL = 60 * 24 * time.Hour
	// UserProfileTTL 用户资料缓存 TTL
	UserProfileTTL = 1 * time.Hour
	// UserProfileEmptyTTL 用户资料空值缓存 TTL
	UserProfileEmptyTTL = 5 * time.Minute
	// FriendRelationTTL 好友关系缓存 TTL
	FriendRelationTTL = 24 * time.Hour
	// FriendRelationEmptyTTL 好友关系空值缓存 TTL
	FriendRelationEmptyTTL = 5 * time.Minute
	// BlacklistTTL 黑名单缓存 TTL
	BlacklistTTL = 24 * time.Hour
	// BlacklistEmptyTTL 黑名单空值缓存 TTL
	BlacklistEmptyTTL = 5 * time.Minute
	// ApplyPendingTTL 好友申请待处理缓存 TTL
	ApplyPendingTTL = 24 * time.Hour
	// ApplyPendingEmptyTTL 好友申请空值缓存 TTL
	ApplyPendingEmptyTTL = 5 * time.Minute
	// ApplyUnreadNotifyTTL 好友申请未读计数 TTL
	ApplyUnreadNotifyTTL = 7 * 24 * time.Hour
	// QRCodeTTL 用户二维码缓存 TTL
	QRCodeTTL = 48 * time.Hour
	// GroupInfoTTL 群资料缓存 TTL
	GroupInfoTTL = 1 * time.Hour
	// GroupInfoEmptyTTL 群资料空值缓存 TTL
	GroupInfoEmptyTTL = 5 * time.Minute
	// GroupMembersTTL 群成员缓存 TTL
	GroupMembersTTL = 24 * time.Hour
	// GroupJoinRequestTTL 群待审批申请缓存 TTL
	GroupJoinRequestTTL = 24 * time.Hour
	// GroupJoinRequestEmptyTTL 群待审批申请空值缓存 TTL
	GroupJoinRequestEmptyTTL = 5 * time.Minute
	// UserGroupListTTL 用户群列表缓存 TTL
	UserGroupListTTL = 24 * time.Hour
	// UserGroupListEmptyTTL 用户群列表空值缓存 TTL
	UserGroupListEmptyTTL = 5 * time.Minute
	// UserGroupReconcileLeaseTTL 限制同一用户在缓存命中时触发权威对账的频率。
	//
	// 用户群反向索引即使结构和版本都合法，也可能因为人工误写或极端故障多出一个
	// DB 从未存在的关系；这种污染无法仅靠校验缓存格式发现。命中路径至多每小时
	// 触发一次后台对账，在修复可达性与 MySQL 负载之间取得平衡。
	UserGroupReconcileLeaseTTL = 1 * time.Hour
)

// ==================== Bloom Filter Key ====================
const (
	// UserUUIDBloomKey 用户 UUID 存在性 Bloom Filter。
	UserUUIDBloomKey = "bf:user_uuid"
	// GroupUUIDBloomKey 群 UUID 存在性 Bloom Filter。
	GroupUUIDBloomKey = "bf:group_uuid"
)

// ==================== Key 构造函数 ====================
// VerifyCodeKey 生成验证码 Key: user:verify_code:{email}:{type}
func VerifyCodeKey(email string, codeType int32) string {
	return fmt.Sprintf("user:verify_code:%s:%d", email, codeType)
}

// VerifyCodeMinuteKey 生成验证码 1 分钟限流 Key: user:verify_code:1m:{email}
func VerifyCodeMinuteKey(email string) string {
	return fmt.Sprintf("user:verify_code:1m:%s", email)
}

// VerifyCode24HKey 生成验证码 24 小时限流 Key: user:verify_code:24h:{email}
func VerifyCode24HKey(email string) string {
	return fmt.Sprintf("user:verify_code:24h:%s", email)
}

// VerifyCodeIPKey 生成验证码 IP 限流 Key: user:verify_code:1h:{ip}
func VerifyCodeIPKey(ip string) string {
	return fmt.Sprintf("user:verify_code:1h:%s", ip)
}

// RefreshTokenKey 生成 RefreshToken Key: auth:rt:{user_uuid}:{device_id}
func RefreshTokenKey(userUUID, deviceID string) string {
	return fmt.Sprintf("auth:rt:%s:%s", userUUID, deviceID)
}

// DeviceInfoKey 生成设备信息缓存 Key: user:devices:{user_uuid}
func DeviceInfoKey(userUUID string) string {
	return fmt.Sprintf("user:devices:%s", userUUID)
}

// UserRoutingKey 生成用户设备路由 Key: user:routing:{user_uuid}
func UserRoutingKey(userUUID string) string {
	return fmt.Sprintf("user:routing:%s", userUUID)
}

// UserProfileKey 生成用户资料缓存 Key: user:profile:{uuid}
func UserProfileKey(uuid string) string {
	return fmt.Sprintf("user:profile:%s", uuid)
}

// QRCodeTokenKey 生成二维码 token Key: user:qrcode:token:{token}
func QRCodeTokenKey(token string) string {
	return fmt.Sprintf("user:qrcode:token:%s", token)
}

// QRCodeUserKey 生成二维码 user Key: user:qrcode:user:{user_uuid}
func QRCodeUserKey(userUUID string) string {
	return fmt.Sprintf("user:qrcode:user:%s", userUUID)
}

// FriendRelationKey 生成好友关系 Key: user:relation:friend:{user_uuid}
func FriendRelationKey(userUUID string) string {
	return fmt.Sprintf("user:relation:friend:%s", userUUID)
}

// BlacklistRelationKey 生成黑名单 Key: user:relation:blacklist:{user_uuid}
func BlacklistRelationKey(userUUID string) string {
	return fmt.Sprintf("user:relation:blacklist:%s", userUUID)
}

// ApplyPendingKey 生成好友申请待处理 Key: user:apply:pending:{target_uuid}
func ApplyPendingKey(targetUUID string) string {
	return fmt.Sprintf("user:apply:pending:%s", targetUUID)
}

// ApplyUnreadNotifyKey 生成好友申请未读计数 Key: user:notify:friend_apply:unread:{uuid}
func ApplyUnreadNotifyKey(targetUUID string) string {
	return fmt.Sprintf("user:notify:friend_apply:unread:%s", targetUUID)
}

// GroupMembersKey 生成群成员缓存 Key: group:members:{group_uuid}
func GroupMembersKey(groupUUID string) string {
	return fmt.Sprintf("group:members:%s", groupUUID)
}

// GroupJoinRequestPendingKey 生成群待审批申请缓存 Key: group:join_requests:{group_uuid}
func GroupJoinRequestPendingKey(groupUUID string) string {
	return fmt.Sprintf("group:join_requests:%s", groupUUID)
}

// GroupInfoKey 生成群资料缓存 Key: group:info:{group_uuid}
func GroupInfoKey(groupUUID string) string {
	return fmt.Sprintf("group:info:%s", groupUUID)
}

// UserGroupListKey 生成用户群列表缓存 Key: group:user_groups:{user_uuid}。
//
// UUID 外层花括号是 Redis Cluster hash tag；它保证该 ZSet 与
// UserGroupVersionKey 的版本 Hash 落在同一 slot，二者才能由一段 Lua 原子更新。
func UserGroupListKey(userUUID string) string {
	return fmt.Sprintf("group:user_groups:{%s}", userUUID)
}

// UserGroupVersionKey 生成用户群反向索引的逐群版本 Key。
//
// Hash field 为 group_uuid，值为该群最后一次影响此用户成员关系的 projection_version；
// 删除成员时仍保留版本 tombstone，防止更旧的 member_added 事件把群重新加回来。
func UserGroupVersionKey(userUUID string) string {
	return fmt.Sprintf("group:user_group_versions:{%s}", userUUID)
}

// UserGroupReconcileLeaseKey 生成用户群列表命中后对账的租约 Key。
//
// 该 String 只负责限频，不参与投影原子更新；沿用 userUUID hash tag 便于 Redis Cluster
// 运维时把同一用户的相关 Key 归到同一 slot。
func UserGroupReconcileLeaseKey(userUUID string) string {
	return fmt.Sprintf("group:user_groups_reconcile_lease:{%s}", userUUID)
}

// ==================== Gateway Key 构造函数 ====================
// GatewayIPBlacklistKey 网关 IP 黑名单 Key: gateway:blacklist:ips
func GatewayIPBlacklistKey() string {
	return "gateway:blacklist:ips"
}

// GatewayUserRateLimitKey 网关用户限流 Key: gateway:rate:limit:user:{user_uuid}
func GatewayUserRateLimitKey(userUUID string) string {
	return fmt.Sprintf("gateway:rate:limit:user:%s", userUUID)
}

// GatewayIPRateLimitKey 网关 IP 限流 Key: rate:limit:ip:{ip}
func GatewayIPRateLimitKey(ip string) string {
	return fmt.Sprintf("rate:limit:ip:%s", ip)
}

// ==================== Msg-Service Key 构造函数 ====================
const (
	// MsgIdempotentTTL 消息幂等缓存 TTL（防止弱网重发）
	MsgIdempotentTTL = 10 * time.Minute
)

// MsgSeqKey 会话内 seq 分配 Key: msg:seq:{conv_id}
// 数据类型: String (Redis INCR)
// TTL: 无（永久存在，随会话生命周期）
func MsgSeqKey(convId string) string {
	return fmt.Sprintf("msg:seq:%s", convId)
}

// MsgIdempotentKey 消息幂等缓存 Key: msg:idempotent:{from_uuid}:{device_id}:{client_msg_id}
// 数据类型: String (JSON, 首次发送结果)
// TTL: 10min
func MsgIdempotentKey(fromUuid, deviceId, clientMsgId string) string {
	return fmt.Sprintf("msg:idempotent:%s:%s:%s", fromUuid, deviceId, clientMsgId)
}
