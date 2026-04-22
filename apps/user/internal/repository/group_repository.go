package repository

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/model"
	"gorm.io/gorm"
)

// groupRepositoryImpl 群组数据访问实现。
type groupRepositoryImpl struct {
	db *gorm.DB
}

// NewGroupRepository 创建群组仓储实例。
func NewGroupRepository(db *gorm.DB) IGroupRepository {
	return &groupRepositoryImpl{db: db}
}

// GetGroupMembers 获取群组有效成员列表。
func (r *groupRepositoryImpl) GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if r == nil || r.db == nil || groupUUID == "" {
		return []*model.GroupMember{}, nil
	}

	var groupInfo model.GroupInfo
	if err := r.db.WithContext(ctx).
		Where("uuid = ? AND status = 0 AND deleted_at IS NULL", groupUUID).
		First(&groupInfo).Error; err != nil {
		return nil, WrapDBError(err)
	}

	var members []*model.GroupMember
	if err := r.db.WithContext(ctx).
		Where("group_uuid = ? AND status = 0 AND deleted_at IS NULL", groupUUID).
		Order("role DESC, joined_at ASC, id ASC").
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("查询群成员失败: %w", WrapDBError(err))
	}

	return members, nil
}
