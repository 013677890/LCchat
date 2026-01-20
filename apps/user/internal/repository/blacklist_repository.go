package repository

import (
	"ChatServer/model"
	"context"
	"time"

	"gorm.io/gorm"
)

// blacklistRepositoryImpl 黑名单数据访问层实现
type blacklistRepositoryImpl struct {
	db *gorm.DB
}

// NewBlacklistRepository 创建黑名单仓储实例
func NewBlacklistRepository(db *gorm.DB) IBlacklistRepository {
	return &blacklistRepositoryImpl{db: db}
}

// AddBlacklist 拉黑用户
func (r *blacklistRepositoryImpl) AddBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	// 先查询是否存在关系
	var relation model.UserRelation
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, targetUUID).
		First(&relation).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在关系，创建新的拉黑关系
		relation = model.UserRelation{
			UserUuid:  userUUID,
			PeerUuid:  targetUUID,
			Status:    1, // 拉黑状态
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		return r.db.WithContext(ctx).Create(&relation).Error
	}

	if err != nil {
		return err
	}

	// 存在关系，更新为拉黑状态
	return r.db.WithContext(ctx).
		Model(&relation).
		Update("status", 1).Error
}

// RemoveBlacklist 取消拉黑
func (r *blacklistRepositoryImpl) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = 1", userUUID, targetUUID).
		Update("status", 2).Error
}

// GetBlacklistList 获取黑名单列表
func (r *blacklistRepositoryImpl) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	var relations []*model.UserRelation
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status = 1", userUUID)

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Order("updated_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&relations).Error

	if err != nil {
		return nil, 0, err
	}

	return relations, total, nil
}

// IsBlocked 检查是否被拉黑
func (r *blacklistRepositoryImpl) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = 1", targetUUID, userUUID).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetBlacklistRelation 获取拉黑关系
func (r *blacklistRepositoryImpl) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ? AND status = 1", userUUID, targetUUID).
		First(&relation).Error
	if err != nil {
		return nil, err
	}
	return &relation, nil
}
