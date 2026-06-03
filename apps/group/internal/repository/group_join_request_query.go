package repository

import (
	"context"
	"errors"
	"github.com/013677890/LCchat-Backend/model"
	"gorm.io/gorm"
)

// loadLatestJoinRequestByApplicant 读取当前用户在指定群的最新申请记录。
//
// 这里不只看 pending，而是保留最近一次申请的终态，
// 让“我的申请状态”接口可以直接展示待审批、已通过、已拒绝、已撤销四类结果。
func (r *groupRepositoryImpl) loadLatestJoinRequestByApplicant(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if r == nil || r.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	var joinRequest model.GroupJoinRequest
	err := r.db.WithContext(ctx).
		Where("group_uuid = ? AND applicant_uuid = ? AND deleted_at IS NULL", groupUUID, applicantUUID).
		Order("id DESC").
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, WrapDBError(err)
	}
	return &joinRequest, nil
}

// GetJoinRequestApplicant 获取指定申请对应的申请人 UUID。
func (r *groupRepositoryImpl) GetJoinRequestApplicant(ctx context.Context, groupUUID string, applyID int64) (string, error) {
	if r == nil || r.db == nil || groupUUID == "" || applyID <= 0 {
		return "", nil
	}
	var joinRequest model.GroupJoinRequest
	err := r.db.WithContext(ctx).
		Select("applicant_uuid").
		Where("id = ? AND group_uuid = ? AND deleted_at IS NULL", applyID, groupUUID).
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", WrapDBError(err)
	}
	return joinRequest.ApplicantUuid, nil
}

// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
//
// 这里保留全部终态而不是只看待审批记录，原因是：
//  1. 用户侧需要回看最近申请是否已通过、已拒绝或已撤销；
//  2. 申请列表是低频读，直接回源 MySQL 比额外维护复杂缓存更划算；
//  3. 结果集按创建时间倒序输出，更符合用户查看最近申请进度的习惯。
func (r *groupRepositoryImpl) ListMyJoinGroupApplications(ctx context.Context, applicantUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if r == nil || r.db == nil || applicantUUID == "" {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	query := r.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("applicant_uuid = ? AND deleted_at IS NULL", applicantUUID)
	if status != nil {
		// 状态筛选只追加到查询条件，不改变默认“查看全部终态”的产品语义。
		query = query.Where("status = ?", *status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	if total == 0 {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	items := make([]*model.GroupJoinRequest, 0, pageSize)
	if err := query.
		Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	return items, total, nil
}

// ListReviewedJoinRequests 获取群已审批入群申请列表。
//
// 这里把“审批记录”定义为管理员已经处理过的申请，因此只返回：
//  1. 已通过；
//  2. 已拒绝；
//
// 不包含申请人自己撤销的记录，避免把用户自助动作和管理审批历史混在一起。
func (r *groupRepositoryImpl) ListReviewedJoinRequests(ctx context.Context, groupUUID, operatorUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return nil, 0, err
	}
	var operator model.GroupMember
	// 审批记录属于管理视角数据，只允许管理员及以上角色读取，避免普通成员窥视审批历史。
	if err := r.db.WithContext(ctx).
		Select("role").
		Where("group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, operatorUUID, memberStatusNormal).
		Take(&operator).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrNoPermission
		}
		return nil, 0, WrapDBError(err)
	}
	if operator.Role < memberRoleAdmin {
		return nil, 0, ErrNoPermission
	}
	// 这里显式排除“申请人主动撤销”，保证审批记录只保留管理员真正处理过的历史。
	statuses := []int8{joinRequestStatusApproved, joinRequestStatusRejected}
	if status != nil {
		// service 层已经保证只会传入已通过或已拒绝，仓储层只负责按最终条件查询。
		statuses = []int8{*status}
	}
	query := r.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("group_uuid = ? AND status IN ? AND deleted_at IS NULL", groupUUID, statuses)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	if total == 0 {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	items := make([]*model.GroupJoinRequest, 0, pageSize)
	if err := query.
		Order("reviewed_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	return items, total, nil
}

// GetGroupsByUUIDs 按群 UUID 批量查询群资料。
//
// 该方法主要给“我的申请列表”补齐群名称与头像展示字段，
// 因此只返回当前仍能查到的群资料，缺失群资料时由上层按空展示降级。
func (r *groupRepositoryImpl) GetGroupsByUUIDs(ctx context.Context, groupUUIDs []string) (map[string]*model.GroupInfo, error) {
	result := make(map[string]*model.GroupInfo, len(groupUUIDs))
	if r == nil || r.db == nil || len(groupUUIDs) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(groupUUIDs))
	seen := make(map[string]struct{}, len(groupUUIDs))
	for _, groupUUID := range groupUUIDs {
		if groupUUID == "" {
			continue
		}
		if _, exists := seen[groupUUID]; exists {
			continue
		}
		seen[groupUUID] = struct{}{}
		unique = append(unique, groupUUID)
	}
	if len(unique) == 0 {
		return result, nil
	}
	// 批量补展示字段前先过滤确定不存在的 group_uuid，避免申请列表里脏 UUID 持续穿透 DB。
	queryUUIDs := r.filterGroupUUIDsByBloom(ctx, unique)
	if len(queryUUIDs) == 0 {
		return result, nil
	}
	var groups []*model.GroupInfo
	if err := r.db.WithContext(ctx).
		Where("uuid IN ? AND deleted_at IS NULL", queryUUIDs).
		Find(&groups).Error; err != nil {
		return nil, WrapDBError(err)
	}
	foundGroupUUIDs := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.Uuid == "" {
			continue
		}
		result[group.Uuid] = group
		foundGroupUUIDs = append(foundGroupUUIDs, group.Uuid)
	}
	r.addGroupUUIDsToBloomAsync(ctx, foundGroupUUIDs)
	return result, nil
}
