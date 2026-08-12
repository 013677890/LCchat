package repository

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/model"
)

// AccountStatusItem 表示账号存在性与状态查询结果。
type AccountStatusItem struct {
	UserUUID string
	Exists   bool
	Status   int32
}

// IAuthRepository 定义认证与账号安全相关的数据访问能力。
type IAuthRepository interface {
	// GetByEmail 根据邮箱查询账号。
	GetByEmail(ctx context.Context, email string) (*model.UserAccount, error)
	// GetByUserUUID 根据用户 UUID 查询账号。
	GetByUserUUID(ctx context.Context, userUUID string) (*model.UserAccount, error)
	// ExistsByEmail 检查邮箱是否已被占用。
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	// Create 创建账号。
	Create(ctx context.Context, user *model.UserAccount) (*model.UserAccount, error)
	// CreateWithOutboxEvent 在创建账号的同一事务中追加 Outbox 事件。
	CreateWithOutboxEvent(ctx context.Context, user *model.UserAccount, eventType, payload string) (*model.UserAccount, error)
	// UpdatePassword 更新密码哈希。
	UpdatePassword(ctx context.Context, userUUID, password string) error
	// UpdateEmail 更新邮箱。
	UpdateEmail(ctx context.Context, userUUID, email string) error
	// UpdateLoginDisplay 更新登录态所需的昵称与头像冗余字段。
	UpdateLoginDisplay(ctx context.Context, userUUID, nickname, avatar string) error
	// Delete 软删除账号。
	Delete(ctx context.Context, userUUID string) error
	// DeleteWithOutboxEvent 在软删除账号的同一事务中追加 Outbox 事件。
	DeleteWithOutboxEvent(ctx context.Context, userUUID, eventType, payload string) error

	// VerifyVerifyCode 校验验证码是否匹配。
	VerifyVerifyCode(ctx context.Context, email, verifyCode string, codeType int32) (bool, error)
	// StoreVerifyCode 存储验证码。
	StoreVerifyCode(ctx context.Context, email, verifyCode string, codeType int32, expireDuration time.Duration) error
	// DeleteVerifyCode 删除验证码。
	DeleteVerifyCode(ctx context.Context, email string, codeType int32) error
	// CheckAndIncrementVerifyCodeRateLimit 原子校验验证码发送限流并占用一次计数。
	CheckAndIncrementVerifyCodeRateLimit(ctx context.Context, email, ip string) (bool, error)

	// BatchGetAccountStatus 批量查询账号存在性与状态。
	BatchGetAccountStatus(ctx context.Context, userUUIDs []string) ([]*AccountStatusItem, error)
}

// IDeviceRepository 定义设备会话与在线状态相关的数据访问能力。
type IDeviceRepository interface {
	// GetByUserUUID 获取用户的所有设备会话。
	GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error)
	// GetByDeviceID 根据设备 ID 获取设备会话。
	GetByDeviceID(ctx context.Context, userUUID, deviceID string) (*model.DeviceSession, error)
	// UpsertSession 创建或更新设备会话。
	UpsertSession(ctx context.Context, session *model.DeviceSession) error
	// TouchDeviceInfoTTL 续期设备信息缓存 TTL。
	TouchDeviceInfoTTL(ctx context.Context, userUUID string) error

	// BatchGetOnlineStatus 批量获取用户设备会话。
	BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error)

	// StoreRefreshToken 存储 RefreshToken。
	StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error
	// GetRefreshToken 获取 RefreshToken。
	GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error)
	// DeleteRefreshToken 撤销指定设备的续期凭据。
	DeleteRefreshToken(ctx context.Context, userUUID, deviceID string) error
	// DeleteByUserUUID 删除用户的全部设备登录态。
	DeleteByUserUUID(ctx context.Context, userUUID string) error
	// UpdateOnlineStatus 更新设备在线状态。
	UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error
}
