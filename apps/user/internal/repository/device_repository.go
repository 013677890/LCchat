package repository

import (
	"ChatServer/model"
	"context"
	"time"

	"gorm.io/gorm"
)

// deviceRepositoryImpl 设备会话数据访问层实现
type deviceRepositoryImpl struct {
	db *gorm.DB
}

// NewDeviceRepository 创建设备会话仓储实例
func NewDeviceRepository(db *gorm.DB) IDeviceRepository {
	return &deviceRepositoryImpl{db: db}
}

// Create 创建设备会话
func (r *deviceRepositoryImpl) Create(ctx context.Context, session *model.DeviceSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// GetByUserUUID 获取用户的所有设备会话
func (r *deviceRepositoryImpl) GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	var sessions []*model.DeviceSession
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND status = 0", userUUID).
		Order("created_at DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
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

// UpdateOnlineStatus 更新在线状态
func (r *deviceRepositoryImpl) UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error {
	return r.db.WithContext(ctx).
		Model(&model.DeviceSession{}).
		Where("user_uuid = ? AND device_id = ?", userUUID, deviceID).
		Update("status", status).Error
}

// UpdateLastSeen 更新最后活跃时间
func (r *deviceRepositoryImpl) UpdateLastSeen(ctx context.Context, userUUID, deviceID string) error {
	return r.db.WithContext(ctx).
		Model(&model.DeviceSession{}).
		Where("user_uuid = ? AND device_id = ?", userUUID, deviceID).
		UpdateColumn("last_seen_at", gorm.Expr("NOW()")).Error
}

// Delete 删除设备会话
func (r *deviceRepositoryImpl) Delete(ctx context.Context, userUUID, deviceID string) error {
	return r.db.WithContext(ctx).
		Where("user_uuid = ? AND device_id = ?", userUUID, deviceID).
		Delete(&model.DeviceSession{}).Error
}

// GetOnlineDevices 获取在线设备列表
func (r *deviceRepositoryImpl) GetOnlineDevices(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	var sessions []*model.DeviceSession
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND status = 0", userUUID).
		Order("last_seen_at DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// BatchGetOnlineStatus 批量获取用户在线状态
func (r *deviceRepositoryImpl) BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
	if len(userUUIDs) == 0 {
		return make(map[string][]*model.DeviceSession), nil
	}

	var sessions []*model.DeviceSession
	err := r.db.WithContext(ctx).
		Where("user_uuid IN ? AND status = 0", userUUIDs).
		Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	// 按用户UUID分组
	result := make(map[string][]*model.DeviceSession)
	for _, session := range sessions {
		result[session.UserUuid] = append(result[session.UserUuid], session)
	}

	return result, nil
}

// UpdateToken 更新Token
func (r *deviceRepositoryImpl) UpdateToken(ctx context.Context, userUUID, deviceID, token, refreshToken string, expireAt *time.Time) error {
	updates := map[string]interface{}{
		"token":         token,
		"refresh_token": refreshToken,
	}

	if expireAt != nil {
		updates["expire_at"] = expireAt
	}

	return r.db.WithContext(ctx).
		Model(&model.DeviceSession{}).
		Where("user_uuid = ? AND device_id = ?", userUUID, deviceID).
		Updates(updates).Error
}

// DeleteByUserUUID 删除用户所有设备会话
func (r *deviceRepositoryImpl) DeleteByUserUUID(ctx context.Context, userUUID string) error {
	return r.db.WithContext(ctx).
		Where("user_uuid = ?", userUUID).
		Delete(&model.DeviceSession{}).Error
}
