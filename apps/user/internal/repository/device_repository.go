package repository

import (
	"ChatServer/model"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// deviceRepositoryImpl 设备会话数据访问层实现
type deviceRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewDeviceRepository 创建设备会话仓储实例
func NewDeviceRepository(db *gorm.DB, redisClient *redis.Client) IDeviceRepository {
	return &deviceRepositoryImpl{db: db, redisClient: redisClient}
}

// Redis Key 构造函数
func (r *deviceRepositoryImpl) accessTokenKey(userUUID, deviceID string) string {
	return fmt.Sprintf("auth:at:%s:%s", userUUID, deviceID)
}

func (r *deviceRepositoryImpl) refreshTokenKey(userUUID, deviceID string) string {
	return fmt.Sprintf("auth:rt:%s:%s", userUUID, deviceID)
}

// md5Hash 计算字符串的 MD5 哈希
func md5Hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// Create 创建设备会话
func (r *deviceRepositoryImpl) Create(ctx context.Context, session *model.DeviceSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// GetByUserUUID 获取用户的所有设备会话
func (r *deviceRepositoryImpl) GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	return nil, nil // TODO: 获取用户的所有设备会话
}

// GetByDeviceID 根据设备ID获取会话
func (r *deviceRepositoryImpl) GetByDeviceID(ctx context.Context, userUUID, deviceID string) (*model.DeviceSession, error) {
	var session model.DeviceSession
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND device_id = ?", userUUID, deviceID).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpsertSession 创建或更新设备会话（Upsert）
func (r *deviceRepositoryImpl) UpsertSession(ctx context.Context, session *model.DeviceSession) error {
	now := time.Now()
	
	// 直接执行 INSERT ... ON DUPLICATE KEY UPDATE
	// 当唯一索引冲突时（user_uuid + device_id 已存在），执行 UPDATE
	return r.db.WithContext(ctx).
		Exec(`
			INSERT INTO device_session (
				user_uuid, device_id, device_name, platform, 
				app_version, ip, user_agent, last_seen_at, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
			ON DUPLICATE KEY UPDATE
				device_name = VALUES(device_name),
				platform = VALUES(platform),
				app_version = VALUES(app_version),
				ip = VALUES(ip),
				user_agent = VALUES(user_agent),
				last_seen_at = VALUES(last_seen_at),
				status = 0,
				updated_at = VALUES(updated_at)
		`, 
			session.UserUuid, session.DeviceId, session.DeviceName, session.Platform,
			session.AppVersion, session.IP, session.UserAgent, now, now, now,
		).Error
}

// StoreAccessToken 将 AccessToken 存入 Redis
// userUUID: 用户 UUID
// deviceID: 设备 ID
// accessToken: 访问令牌（完整的 JWT 字符串）
// expireDuration: 过期时间
func (r *deviceRepositoryImpl) StoreAccessToken(ctx context.Context, userUUID, deviceID, accessToken string, expireDuration time.Duration) error {
	key := r.accessTokenKey(userUUID, deviceID)
	// 存储 MD5 哈希值以节省内存
	value := md5Hash(accessToken)
	return r.redisClient.Set(ctx, key, value, expireDuration).Err()
}

// StoreRefreshToken 将 RefreshToken 存入 Redis
// userUUID: 用户 UUID
// deviceID: 设备 ID
// refreshToken: 刷新令牌（UUID 字符串）
// expireDuration: 过期时间
func (r *deviceRepositoryImpl) StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error {
	key := r.refreshTokenKey(userUUID, deviceID)
	// RefreshToken 直接存储原始值
	return r.redisClient.Set(ctx, key, refreshToken, expireDuration).Err()
}

// VerifyAccessToken 验证 AccessToken 是否有效
// 返回 true 表示 Token 有效且未被踢出
func (r *deviceRepositoryImpl) VerifyAccessToken(ctx context.Context, userUUID, deviceID, accessToken string) (bool, error) {
	key := r.accessTokenKey(userUUID, deviceID)
	storedHash, err := r.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			// Key 不存在，说明 Token 已过期或被踢出
			return false, nil
		}
		return false, err
	}

	// 比对 MD5 哈希
	currentHash := md5Hash(accessToken)
	return storedHash == currentHash, nil
}

// GetRefreshToken 获取 RefreshToken
func (r *deviceRepositoryImpl) GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error) {
	key := r.refreshTokenKey(userUUID, deviceID)
	return r.redisClient.Get(ctx, key).Result()
}

// DeleteTokens 删除设备的所有 Token（用于踢出设备）
func (r *deviceRepositoryImpl) DeleteTokens(ctx context.Context, userUUID, deviceID string) error {
	atKey := r.accessTokenKey(userUUID, deviceID)
	rtKey := r.refreshTokenKey(userUUID, deviceID)

	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, atKey)
	pipe.Del(ctx, rtKey)
	_, err := pipe.Exec(ctx)
	return err
}

// UpdateOnlineStatus 更新在线状态
func (r *deviceRepositoryImpl) UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error {
	return nil // TODO: 更新在线状态
}

// UpdateLastSeen 更新最后活跃时间
func (r *deviceRepositoryImpl) UpdateLastSeen(ctx context.Context, userUUID, deviceID string) error {
	return nil // TODO: 更新最后活跃时间
}

// Delete 删除设备会话
func (r *deviceRepositoryImpl) Delete(ctx context.Context, userUUID, deviceID string) error {
	return nil // TODO: 删除设备会话
}

// GetOnlineDevices 获取在线设备列表
func (r *deviceRepositoryImpl) GetOnlineDevices(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	return nil, nil // TODO: 获取在线设备列表
}

// BatchGetOnlineStatus 批量获取用户在线状态
func (r *deviceRepositoryImpl) BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
	if len(userUUIDs) == 0 {
		return nil, nil // TODO: 批量获取用户在线状态
	}
	return nil, nil // TODO: 批量获取用户在线状态
}

// UpdateToken 更新Token
func (r *deviceRepositoryImpl) UpdateToken(ctx context.Context, userUUID, deviceID, token, refreshToken string, expireAt *time.Time) error {
	return nil // TODO: 更新Token
}

// DeleteByUserUUID 删除用户所有设备会话
func (r *deviceRepositoryImpl) DeleteByUserUUID(ctx context.Context, userUUID string) error {
	return nil // TODO: 删除用户所有设备会话
}
