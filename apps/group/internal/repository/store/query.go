package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"gorm.io/gorm"
)

// LoadGroupForRead 读取读路径需要的群状态与全员禁言开关。
//
// 与 LoadGroupInfoFromDB 的差异：
//  1. 这里不把解散群折叠成“不存在”，而是返回 ErrGroupDismissed；
//  2. 只选 uuid / status / mute_all，避免权限检查拖整行资料。
func (s *Store) LoadGroupForRead(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if s == nil || s.db == nil || groupUUID == "" {
		return nil, repoerr.ErrRecordNotFound
	}
	var group model.GroupInfo
	err := s.db.WithContext(ctx).
		Select("uuid", "status", "mute_all").
		Where("uuid = ? AND deleted_at IS NULL", groupUUID).
		Take(&group).Error
	if err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	if group.Status == repository.GroupStatusDismissed {
		return nil, repository.ErrGroupDismissed
	}
	if group.Status != repository.GroupStatusNormal {
		return nil, repoerr.ErrRecordNotFound
	}
	return &group, nil
}

// LoadGroupInfoFromDB 从 MySQL 读取有效群资料。
func (s *Store) LoadGroupInfoFromDB(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if s == nil || s.db == nil || groupUUID == "" {
		return nil, repoerr.ErrRecordNotFound
	}
	var groupInfo model.GroupInfo
	if err := s.db.WithContext(ctx).
		Where("uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, repository.GroupStatusNormal).
		First(&groupInfo).Error; err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	return &groupInfo, nil
}

// LoadGroupMembersFromDB 从 MySQL 读取群内有效成员列表。
func (s *Store) LoadGroupMembersFromDB(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if _, err := s.LoadGroupInfoFromDB(ctx, groupUUID); err != nil {
		return nil, err
	}
	var members []*model.GroupMember
	if err := s.db.WithContext(ctx).
		Table("group_members AS gm").
		Select("gm.*").
		Joins("JOIN `groups` AS g ON g.uuid = gm.group_uuid").
		Where("gm.group_uuid = ? AND gm.status = ? AND gm.deleted_at IS NULL", groupUUID, repository.MemberStatusNormal).
		Where("g.status = ? AND g.deleted_at IS NULL", repository.GroupStatusNormal).
		Order("gm.role DESC, gm.joined_at ASC, gm.id ASC").
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("查询群成员失败: %w", repoerr.WrapDBError(err))
	}
	return members, nil
}

// LoadActiveMemberFromDB 点查单个有效成员。
func (s *Store) LoadActiveMemberFromDB(ctx context.Context, groupUUID, userUUID string) (*model.GroupMember, error) {
	if s == nil || s.db == nil || groupUUID == "" || userUUID == "" {
		return nil, nil
	}
	var member model.GroupMember
	err := s.db.WithContext(ctx).
		Select("user_uuid", "role", "remark", "mute_until", "joined_at").
		Where(
			"group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL",
			groupUUID,
			userUUID,
			repository.MemberStatusNormal,
		).
		Take(&member).Error
	if err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, repoerr.WrapDBError(err)
	}
	return &member, nil
}

// LoadUserGroupsFromDB 从 MySQL 读取用户当前所在的有效群列表。
func (s *Store) LoadUserGroupsFromDB(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	if s == nil || s.db == nil || userUUID == "" {
		return []*model.GroupInfo{}, nil
	}
	var groups []*model.GroupInfo
	if err := s.db.WithContext(ctx).
		Table("`groups` AS g").
		Select("DISTINCT g.*").
		Joins("JOIN group_members AS gm ON gm.group_uuid = g.uuid").
		Where("gm.user_uuid = ? AND gm.status = ? AND gm.deleted_at IS NULL", userUUID, repository.MemberStatusNormal).
		Where("g.status = ? AND g.deleted_at IS NULL", repository.GroupStatusNormal).
		Order("g.updated_at DESC, g.id DESC").
		Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("查询用户群列表失败: %w", repoerr.WrapDBError(err))
	}
	return groups, nil
}

// LoadPendingJoinRequestsFromDB 从 MySQL 读取群待审批申请全量列表。
func (s *Store) LoadPendingJoinRequestsFromDB(ctx context.Context, groupUUID string) ([]*model.GroupJoinRequest, error) {
	if s == nil || s.db == nil || groupUUID == "" {
		return []*model.GroupJoinRequest{}, nil
	}
	var items []*model.GroupJoinRequest
	if err := s.db.WithContext(ctx).
		Where("group_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, repository.JoinRequestStatusPending).
		Order("created_at DESC, id DESC").
		Find(&items).Error; err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	return items, nil
}

// SearchGroupMembers 按用户 UUID、群名片或用户昵称搜索群成员。
func (s *Store) SearchGroupMembers(ctx context.Context, groupUUID, operatorUUID, keyword string, page, pageSize int) ([]*model.GroupMember, int64, error) {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" {
		return []*model.GroupMember{}, 0, nil
	}
	if err := s.ensureGroupNormalFromDB(ctx, groupUUID); err != nil {
		return nil, 0, err
	}
	if _, err := s.EnsureActiveMemberRole(ctx, groupUUID, operatorUUID, repository.MemberRoleMember); err != nil {
		return nil, 0, err
	}
	baseQuery := s.db.WithContext(ctx).
		Table("group_members AS gm").
		Joins("LEFT JOIN user_profile AS up ON up.user_uuid = gm.user_uuid").
		Where("gm.group_uuid = ? AND gm.status = ? AND gm.deleted_at IS NULL", groupUUID, repository.MemberStatusNormal)
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		baseQuery = baseQuery.Where("gm.user_uuid LIKE ? OR gm.remark LIKE ? OR up.nickname LIKE ?", like, like, like)
	}
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, repoerr.WrapDBError(err)
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
		return nil, 0, repoerr.WrapDBError(err)
	}
	return members, total, nil
}

// SearchGroups 按群名模糊匹配或完整群号精确匹配正常群。
func (s *Store) SearchGroups(ctx context.Context, keyword string, page, pageSize int) ([]*model.GroupInfo, int64, error) {
	if s == nil || s.db == nil {
		return []*model.GroupInfo{}, 0, nil
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []*model.GroupInfo{}, 0, nil
	}
	likeKeyword := "%" + keyword + "%"
	baseQuery := s.db.WithContext(ctx).
		Model(&model.GroupInfo{}).
		Where("status = ? AND deleted_at IS NULL", repository.GroupStatusNormal).
		Where("uuid = ? OR name LIKE ?", keyword, likeKeyword)
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, repoerr.WrapDBError(err)
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
		return nil, 0, repoerr.WrapDBError(err)
	}
	return groups, total, nil
}

// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
func (s *Store) ListMyJoinGroupApplications(ctx context.Context, applicantUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if s == nil || s.db == nil || applicantUUID == "" {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	query := s.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("applicant_uuid = ? AND deleted_at IS NULL", applicantUUID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, repoerr.WrapDBError(err)
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
		return nil, 0, repoerr.WrapDBError(err)
	}
	return items, total, nil
}

// ListReviewedJoinRequests 获取群已审批入群申请列表。
func (s *Store) ListReviewedJoinRequests(ctx context.Context, groupUUID, operatorUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	if err := s.ensureGroupNormalFromDB(ctx, groupUUID); err != nil {
		return nil, 0, err
	}
	if _, err := s.EnsureActiveMemberRole(ctx, groupUUID, operatorUUID, repository.MemberRoleAdmin); err != nil {
		return nil, 0, err
	}
	statuses := []int8{repository.JoinRequestStatusApproved, repository.JoinRequestStatusRejected}
	if status != nil {
		statuses = []int8{*status}
	}
	query := s.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("group_uuid = ? AND status IN ? AND deleted_at IS NULL", groupUUID, statuses)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, repoerr.WrapDBError(err)
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
		return nil, 0, repoerr.WrapDBError(err)
	}
	return items, total, nil
}

// GetJoinRequestPendingCount 统计群待审批申请数量。
func (s *Store) GetJoinRequestPendingCount(ctx context.Context, groupUUID, operatorUUID string) (int64, error) {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" {
		return 0, nil
	}
	if err := s.ensureGroupNormalFromDB(ctx, groupUUID); err != nil {
		return 0, err
	}
	if _, err := s.EnsureActiveMemberRole(ctx, groupUUID, operatorUUID, repository.MemberRoleAdmin); err != nil {
		return 0, err
	}
	var count int64
	err := s.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("group_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, repository.JoinRequestStatusPending).
		Count(&count).Error
	if err != nil {
		return 0, repoerr.WrapDBError(err)
	}
	return count, nil
}

// GetUserProfiles 按用户 UUID 批量查询资料。
func (s *Store) GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
	result := make(map[string]*model.UserProfile)
	if s == nil || s.db == nil || len(userUUIDs) == 0 {
		return result, nil
	}
	var profiles []*model.UserProfile
	if err := s.db.WithContext(ctx).
		Where("user_uuid IN ?", userUUIDs).
		Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("批量查询用户资料失败: %w", repoerr.WrapDBError(err))
	}
	for _, profile := range profiles {
		if profile == nil || profile.UserUuid == "" {
			continue
		}
		result[profile.UserUuid] = profile
	}
	return result, nil
}

// GetGroupsByUUIDs 按群 UUID 批量查询群资料。
func (s *Store) GetGroupsByUUIDs(ctx context.Context, groupUUIDs []string) (map[string]*model.GroupInfo, error) {
	result := make(map[string]*model.GroupInfo, len(groupUUIDs))
	if s == nil || s.db == nil || len(groupUUIDs) == 0 {
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
	queryUUIDs := repository.FilterGroupUUIDsByBloom(ctx, s.redisClient, unique)
	if len(queryUUIDs) == 0 {
		return result, nil
	}
	var groups []*model.GroupInfo
	if err := s.db.WithContext(ctx).
		Where("uuid IN ? AND deleted_at IS NULL", queryUUIDs).
		Find(&groups).Error; err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	foundGroupUUIDs := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.Uuid == "" {
			continue
		}
		result[group.Uuid] = group
		foundGroupUUIDs = append(foundGroupUUIDs, group.Uuid)
	}
	repository.AddGroupUUIDsToBloomAsync(ctx, s.redisClient, foundGroupUUIDs)
	return result, nil
}
