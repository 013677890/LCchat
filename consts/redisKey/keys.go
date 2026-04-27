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
	// DeviceActiveTTL 设备活跃时间缓存 TTL
	DeviceActiveTTL = 7 * 24 * time.Hour

	// UserInfoTTL 用户信息缓存 TTL
	UserInfoTTL = 1 * time.Hour
	// UserInfoEmptyTTL 用户信息空值缓存 TTL
	UserInfoEmptyTTL = 5 * time.Minute

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

// AccessTokenKey 生成 AccessToken Key: auth:at:{user_uuid}:{device_id}
func AccessTokenKey(userUUID, deviceID string) string {
	return fmt.Sprintf("auth:at:%s:%s", userUUID, deviceID)
}

// RefreshTokenKey 生成 RefreshToken Key: auth:rt:{user_uuid}:{device_id}
func RefreshTokenKey(userUUID, deviceID string) string {
	return fmt.Sprintf("auth:rt:%s:%s", userUUID, deviceID)
}

// DeviceInfoKey 生成设备信息缓存 Key: user:devices:{user_uuid}
func DeviceInfoKey(userUUID string) string {
	return fmt.Sprintf("user:devices:%s", userUUID)
}

// DeviceActiveKey 生成设备活跃时间 Key: user:devices:active:{user_uuid}
func DeviceActiveKey(userUUID string) string {
	return fmt.Sprintf("user:devices:active:%s", userUUID)
}

// UserRoutingKey 生成用户设备路由 Key: user:routing:{user_uuid}
func UserRoutingKey(userUUID string) string {
	return fmt.Sprintf("user:routing:%s", userUUID)
}

// UserInfoKey 生成用户信息缓存 Key: user:info:{uuid}
func UserInfoKey(uuid string) string {
	return fmt.Sprintf("user:info:%s", uuid)
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
