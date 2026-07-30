package repository

import (
	"context"
	"errors"
	"github.com/013677890/LCchat-Backend/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

const (
	groupSendDenyNotMember   = "not_group_member"
	groupSendDenyGroupMuted  = "group_muted"
	groupSendDenyMemberMuted = "member_muted"
)

// LeaveGroup 处理当前用户主动退群。
//
// 这里复用 RemoveMember 的事务与事件逻辑，但用独立方法表达“本人退群”业务语义，
// 让 service / proto 不必继续把退群伪装成“移除自己”。
func (r *groupRepositoryImpl) LeaveGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	return r.RemoveMember(ctx, groupUUID, operatorUUID, operatorUUID)
}

// SearchGroupMembers 按用户 UUID、群名片或用户昵称搜索群成员。
//
// 搜索前会确认操作者仍在群内，避免非成员通过成员搜索接口枚举群内用户；
// 展示资料只做左连接过滤，不把 user_profile 作为成员关系的权威来源。
func (r *groupRepositoryImpl) SearchGroupMembers(ctx context.Context, groupUUID, operatorUUID, keyword string, page, pageSize int) ([]*model.GroupMember, int64, error) {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return []*model.GroupMember{}, 0, nil
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return nil, 0, err
	}
	if _, err := r.ensureActiveMemberRole(ctx, groupUUID, operatorUUID, memberRoleMember); err != nil {
		return nil, 0, err
	}
	baseQuery := r.db.WithContext(ctx).
		Table("group_members AS gm").
		Joins("LEFT JOIN user_profile AS up ON up.user_uuid = gm.user_uuid").
		Where("gm.group_uuid = ? AND gm.status = ? AND gm.deleted_at IS NULL", groupUUID, memberStatusNormal)
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		baseQuery = baseQuery.Where("gm.user_uuid LIKE ? OR gm.remark LIKE ? OR up.nickname LIKE ?", like, like, like)
	}
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	if total == 0 {
		return []*model.GroupMember{}, 0, nil
	}
	members := make([]*model.GroupMember, 0, pageSize)
	if err := baseQuery.Session(&gorm.Session{}).
		Select("gm.*").
		Order("gm.role DESC, gm.joined_at ASC, gm.id ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&members).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	return members, total, nil
}

// SearchGroups 按群名模糊匹配或完整群号精确匹配正常群。
//
// 空关键字直接返回空结果，避免搜索接口退化成全量群枚举；
// 查询只暴露正常群，已解散或软删除群不会进入公开搜索结果。
func (r *groupRepositoryImpl) SearchGroups(ctx context.Context, keyword string, page, pageSize int) ([]*model.GroupInfo, int64, error) {
	if r == nil || r.db == nil {
		return []*model.GroupInfo{}, 0, nil
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []*model.GroupInfo{}, 0, nil
	}
	likeKeyword := "%" + keyword + "%"
	baseQuery := r.db.WithContext(ctx).
		Model(&model.GroupInfo{}).
		Where("status = ? AND deleted_at IS NULL", groupStatusNormal).
		Where("uuid = ? OR name LIKE ?", keyword, likeKeyword)
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	if total == 0 {
		return []*model.GroupInfo{}, 0, nil
	}
	groups := make([]*model.GroupInfo, 0, pageSize)
	if err := baseQuery.Session(&gorm.Session{}).
		Order("updated_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&groups).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	return groups, total, nil
}

// UpdateMyGroupNickname 更新当前用户自己的群名片。
//
// 群名片是成员维度资料，写入 group_members.remark，并通过成员快照事件同步缓存；
// 群表 updated_at 也会同步刷新，便于用户群列表按最近群资料变更排序。
func (r *groupRepositoryImpl) UpdateMyGroupNickname(ctx context.Context, groupUUID, userUUID, nickname string) error {
	if r == nil || r.db == nil || groupUUID == "" || userUUID == "" {
		return nil
	}
	var changedMember *model.GroupMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		member, err := r.loadActiveMemberForUpdate(ctx, tx, groupUUID, userUUID)
		if err != nil {
			if errors.Is(err, ErrRecordNotFound) {
				return ErrNoPermission
			}
			return err
		}

		if member.Remark == nickname {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", member.Id).
			Updates(map[string]interface{}{
				"remark":     nickname,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}

		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return WrapDBError(err)
		}
		updatedMember := *member
		updatedMember.Remark = nickname
		updatedMember.UpdatedAt = updatedAt
		changedMember = &updatedMember
		group.UpdatedAt = updatedAt
		return r.insertMemberProfileUpdatedEvent(tx, group, userUUID, []*model.GroupMember{changedMember})
	})

	if err != nil {
		return err
	}
	return nil
}

// UpdateGroupMemberNickname 管理员或群主修改指定成员群名片。
//
// 该接口承载管理场景：群主可修改管理员/普通成员，管理员只能修改普通成员；
// 操作者修改自己的名片应走自助接口，避免管理入口和个人资料入口语义混用。
func (r *groupRepositoryImpl) UpdateGroupMemberNickname(ctx context.Context, groupUUID, operatorUUID, targetUserUUID, nickname string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	if operatorUUID == targetUserUUID {
		return ErrNoPermission
	}
	var changedMember *model.GroupMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		operator, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin)
		if err != nil {
			return err
		}
		target, err := r.loadActiveMemberForUpdate(ctx, tx, groupUUID, targetUserUUID)
		if err != nil {
			if errors.Is(err, ErrRecordNotFound) {
				return ErrGroupMemberNotFound
			}
			return err
		}

		if !canUpdateGroupMemberNickname(operator.Role, target.Role) {
			return ErrNoPermission
		}
		if target.Remark == nickname {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", target.Id).
			Updates(map[string]interface{}{
				"remark":     nickname,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}

		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return WrapDBError(err)
		}
		updatedMember := *target
		updatedMember.Remark = nickname
		updatedMember.UpdatedAt = updatedAt
		changedMember = &updatedMember
		group.UpdatedAt = updatedAt
		return r.insertMemberProfileUpdatedEvent(tx, group, operatorUUID, []*model.GroupMember{changedMember})
	})

	if err != nil {
		return err
	}
	return nil
}

// MuteGroupMember 设置或取消指定成员的单人禁言。
//
// 权限矩阵集中在 canMuteGroupMember：群主可处理管理员/普通成员，管理员只能处理普通成员；
// 群主自身不能被单人禁言，避免管理通道被错误锁死。
func (r *groupRepositoryImpl) MuteGroupMember(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, muteUntil *time.Time) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	var changedMember *model.GroupMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		operator, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin)
		if err != nil {
			return err
		}
		target, err := r.loadActiveMemberForUpdate(ctx, tx, groupUUID, targetUserUUID)
		if err != nil {
			if errors.Is(err, ErrRecordNotFound) {
				return ErrGroupMemberNotFound
			}
			return err
		}

		if !canMuteGroupMember(operator.Role, target.Role) {
			return ErrNoPermission
		}
		if muteUntilEqual(target.MuteUntil, muteUntil) {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", target.Id).
			Updates(map[string]interface{}{
				"mute_until": muteUntil,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}

		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return WrapDBError(err)
		}
		updatedMember := *target
		updatedMember.MuteUntil = cloneTimePtr(muteUntil)
		updatedMember.UpdatedAt = updatedAt
		changedMember = &updatedMember
		group.UpdatedAt = updatedAt
		return r.insertMemberMutedEvent(tx, group, operatorUUID, []*model.GroupMember{changedMember})
	})

	if err != nil {
		return err
	}
	return nil
}

// UpdateGroupMuteSetting 更新群全员禁言开关。
//
// 该开关是群级策略，因此只更新 groups.mute_all；单个成员是否被禁言仍由
// group_members.mute_until 独立表达，两个条件会在发送权限检查时同时生效。
func (r *groupRepositoryImpl) UpdateGroupMuteSetting(ctx context.Context, groupUUID, operatorUUID string, muteAll bool) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		if _, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin); err != nil {
			return err
		}
		if group.MuteAll == muteAll {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"mute_all":   muteAll,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		group.MuteAll = muteAll
		group.UpdatedAt = updatedAt
		return r.insertGroupMuteSettingUpdatedEvent(tx, group, operatorUUID)
	})
}

// CheckGroupSendPermission 检查指定用户在群内是否允许发送消息。
//
// 该方法面向消息写链路，返回“允许/拒绝 + 原因”而不是直接把非成员当错误抛出，
// 便于 msg-service 用统一结果拒绝发送并给客户端稳定提示。
func (r *groupRepositoryImpl) CheckGroupSendPermission(ctx context.Context, groupUUID, userUUID string) (CheckGroupSendPermissionResult, error) {
	result := CheckGroupSendPermissionResult{CanSend: false, Role: -1}
	if r == nil || r.db == nil || groupUUID == "" || userUUID == "" {
		result.Reason = groupSendDenyNotMember
		return result, nil
	}
	group, err := r.loadReadableGroupInfo(ctx, groupUUID)
	if err != nil {
		return result, err
	}
	member, err := r.loadActiveMemberForRead(ctx, groupUUID, userUUID)
	if err != nil {
		return result, err
	}
	if member == nil {
		result.Reason = groupSendDenyNotMember
		return result, nil
	}
	result.Role = member.Role
	result.MuteAll = group.MuteAll
	if group.MuteAll && member.Role == memberRoleMember {
		result.Reason = groupSendDenyGroupMuted
		return result, nil
	}
	if member.MuteUntil != nil && member.MuteUntil.After(time.Now()) {
		result.Reason = groupSendDenyMemberMuted
		result.MuteUntil = member.MuteUntil.UnixMilli()
		return result, nil
	}
	result.CanSend = true
	return result, nil
}

// GetJoinRequestPendingCount 统计群待审批申请数量。
//
// 待办数量属于管理视角数据，读取前必须确认操作者仍是管理员或群主，
// 避免普通成员通过红点接口推测申请活跃度。
func (r *groupRepositoryImpl) GetJoinRequestPendingCount(ctx context.Context, groupUUID, operatorUUID string) (int64, error) {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return 0, nil
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return 0, err
	}
	if _, err := r.ensureActiveMemberRole(ctx, groupUUID, operatorUUID, memberRoleAdmin); err != nil {
		return 0, err
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("group_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, joinRequestStatusPending).
		Count(&count).Error
	if err != nil {
		return 0, WrapDBError(err)
	}
	return count, nil
}

// ensureActiveMemberRole 确认用户是有效成员且角色达到要求。
func (r *groupRepositoryImpl) ensureActiveMemberRole(ctx context.Context, groupUUID, userUUID string, minRole int8) (*model.GroupMember, error) {
	var member model.GroupMember
	err := r.db.WithContext(ctx).
		Select("role").
		Where("group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, userUUID, memberStatusNormal).
		Take(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoPermission
		}
		return nil, WrapDBError(err)
	}
	if member.Role < minRole {
		return nil, ErrNoPermission
	}
	return &member, nil
}

// loadReadableGroupInfo 读取发送权限检查需要的群状态和全员禁言开关。
func (r *groupRepositoryImpl) loadReadableGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	groupInfo, cacheHit, err := r.getGroupInfoFromCache(ctx, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		if groupInfo == nil {
			return nil, ErrRecordNotFound
		}
		if groupInfo.Status == groupStatusDismissed {
			return nil, ErrGroupDismissed
		}
		if groupInfo.Status != groupStatusNormal {
			return nil, ErrRecordNotFound
		}
		return groupInfo, nil
	}

	// 发送权限检查是高频链路，缓存 miss 后先用 Bloom 拦截明显不存在的 group_uuid。
	if !r.groupUUIDMayExist(ctx, groupUUID) {
		return nil, ErrRecordNotFound
	}
	var group model.GroupInfo
	err = r.db.WithContext(ctx).
		Select("uuid", "status", "mute_all").
		Where("uuid = ? AND deleted_at IS NULL", groupUUID).
		Take(&group).Error

	if err != nil {
		return nil, WrapDBError(err)
	}
	r.addGroupUUIDToBloomBestEffort(ctx, groupUUID)
	if group.Status == groupStatusDismissed {
		return nil, ErrGroupDismissed
	}
	if group.Status != groupStatusNormal {
		return nil, ErrRecordNotFound
	}
	return &group, nil
}

// loadActiveMemberForRead 优先从成员缓存读取发送权限所需的成员快照。
func (r *groupRepositoryImpl) loadActiveMemberForRead(ctx context.Context, groupUUID, userUUID string) (*model.GroupMember, error) {
	members, cacheHit, err := r.getGroupMembersFromCache(ctx, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		for _, member := range members {
			if member != nil && member.UserUuid == userUUID {
				return member, nil
			}
		}

		return nil, nil
	}
	var member model.GroupMember
	err = r.db.WithContext(ctx).
		Select("user_uuid", "role", "remark", "mute_until", "joined_at").
		Where("group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, userUUID, memberStatusNormal).
		Take(&member).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, WrapDBError(err)
	}

	return &member, nil
}

// canMuteGroupMember 判断操作者是否可以调整目标成员禁言状态。
func canMuteGroupMember(operatorRole, targetRole int8) bool {
	switch operatorRole {
	case memberRoleOwner:
		return targetRole == memberRoleAdmin || targetRole == memberRoleMember
	case memberRoleAdmin:
		return targetRole == memberRoleMember
	default:
		return false
	}
}

// canUpdateGroupMemberNickname 判断操作者是否可以代改目标成员群名片。
func canUpdateGroupMemberNickname(operatorRole, targetRole int8) bool {
	switch operatorRole {
	case memberRoleOwner:
		return targetRole == memberRoleAdmin || targetRole == memberRoleMember
	case memberRoleAdmin:
		return targetRole == memberRoleMember
	default:
		return false
	}
}

// muteUntilEqual 判断两份禁言截止时间是否代表同一个业务状态。
func muteUntilEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

// cloneTimePtr 复制时间指针，避免调用方后续修改同一地址影响异步缓存 patch。
func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
