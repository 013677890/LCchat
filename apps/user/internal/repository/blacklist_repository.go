package repository

import (
	"ChatServer/model"
	"context"

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
	return nil // TODO: 拉黑用户
}

// RemoveBlacklist 取消拉黑
func (r *blacklistRepositoryImpl) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	return nil // TODO: 取消拉黑
}

// GetBlacklistList 获取黑名单列表
func (r *blacklistRepositoryImpl) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	return nil, 0, nil // TODO: 获取黑名单列表
}

// IsBlocked 检查是否被拉黑
func (r *blacklistRepositoryImpl) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	return false, nil // TODO: 检查是否被拉黑
}

// GetBlacklistRelation 获取拉黑关系
func (r *blacklistRepositoryImpl) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	return nil, nil // TODO: 获取拉黑关系
}
