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
//
// 当前 user-service 仅保留群成员只读查询，因此仓储也只暴露最小查询集合，
// 不再承载群管理写逻辑。
func NewGroupRepository(db *gorm.DB) IGroupRepository {
	return &groupRepositoryImpl{db: db}
}

// GetGroupMembers 获取群组有效成员列表。
//
// 查询分两步完成：
//  1. 先确认群本身存在且处于有效状态，避免把“群不存在”和“成员为空”混为一谈；
//  2. 再按角色优先、加入时间次序返回有效成员，满足 msg-service 的权限判断与展示需求。
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
