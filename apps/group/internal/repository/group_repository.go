package repository

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/model"
	"gorm.io/gorm"
)

// groupRepositoryImpl 是 group-service 仓储层的只读实现。
//
// 当前先聚焦“群资料/成员查询”这条闭环，因此这里主要承接：
//  1. 群资料查询；
//  2. 群成员查询；
//  3. 当前用户所属群列表查询；
//  4. 成员昵称头像补齐所需的用户资料批量查询。
//
// 等后续真正开始群管理写逻辑时，再在同一实现上继续扩展写方法即可。
type groupRepositoryImpl struct {
	db *gorm.DB
}

// NewGroupRepository 创建 group 仓储实例。
//
// 当前仍保持薄构造：
//  1. 只接收 gorm.DB；
//  2. 不在构造阶段探测连通性；
//  3. 由上层 provider 负责基础设施初始化与失败处理。
func NewGroupRepository(db *gorm.DB) IGroupRepository {
	return &groupRepositoryImpl{db: db}
}

// GetGroupInfo 按群 UUID 查询有效群资料。
//
// 约束：
//  1. 只返回 status=0 且未软删的群；
//  2. 群不存在、已解散、已删除都统一映射为“记录不存在”；
//  3. 上层无需感知 groups 表的存储细节。
func (r *groupRepositoryImpl) GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if r == nil || r.db == nil || groupUUID == "" {
		return nil, ErrRecordNotFound
	}

	var groupInfo model.GroupInfo
	if err := r.db.WithContext(ctx).
		Where("uuid = ? AND status = 0 AND deleted_at IS NULL", groupUUID).
		First(&groupInfo).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return &groupInfo, nil
}

// GetGroupMembers 获取群内有效成员列表。
//
// 查询分两步：
//  1. 先确认群本身存在，避免把“群不存在”和“群存在但成员为空”混淆；
//  2. 再按角色优先、入群时间次序返回有效成员，方便上层直接展示与做权限判断。
func (r *groupRepositoryImpl) GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if r == nil || r.db == nil || groupUUID == "" {
		return []*model.GroupMember{}, nil
	}

	if _, err := r.GetGroupInfo(ctx, groupUUID); err != nil {
		return nil, err
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

// ListUserGroups 获取当前用户所属的有效群列表。
//
// 这里使用 join 一次性筛出：
//  1. 群成员记录有效；
//  2. 群本身有效；
//  3. 结果按最近更新时间倒序，便于上层默认展示最近活跃/最近变更的群。
func (r *groupRepositoryImpl) ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	if r == nil || r.db == nil || userUUID == "" {
		return []*model.GroupInfo{}, nil
	}

	var groups []*model.GroupInfo
	if err := r.db.WithContext(ctx).
		Table("groups AS g").
		Select("DISTINCT g.*").
		Joins("JOIN group_members AS gm ON gm.group_uuid = g.uuid").
		Where("gm.user_uuid = ? AND gm.status = 0 AND gm.deleted_at IS NULL", userUUID).
		Where("g.status = 0 AND g.deleted_at IS NULL").
		Order("g.updated_at DESC, g.id DESC").
		Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("查询用户群列表失败: %w", WrapDBError(err))
	}
	return groups, nil
}

// GetUserProfiles 按用户 UUID 批量查询资料。
//
// 返回 map 而不是切片，目的是让 service 层在组装成员列表时能 O(1) 命中昵称头像，
// 避免再做额外的二次索引构建。
func (r *groupRepositoryImpl) GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
	result := make(map[string]*model.UserProfile)
	if r == nil || r.db == nil || len(userUUIDs) == 0 {
		return result, nil
	}

	var profiles []*model.UserProfile
	if err := r.db.WithContext(ctx).
		Where("user_uuid IN ?", userUUIDs).
		Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("批量查询用户资料失败: %w", WrapDBError(err))
	}

	for _, profile := range profiles {
		if profile == nil || profile.UserUuid == "" {
			continue
		}
		result[profile.UserUuid] = profile
	}
	return result, nil
}
