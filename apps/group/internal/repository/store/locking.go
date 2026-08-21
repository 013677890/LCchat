package store

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// loadGroupForUpdate 以行锁（SELECT ... FOR UPDATE）方式加载群记录。
//
// 事务并发边界设计：
//   - 该方法是所有群写操作（改资料、加人、踢人、角色流转、审批）的并发串行化入口；
//   - 同一个群的所有写操作在此排队，保证 `member_cnt` 计数、`cache_version` 版本递增与 Outbox 事件严格保序。
func (s *Store) loadGroupForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string) (*model.GroupInfo, error) {
	var group model.GroupInfo
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("uuid = ? AND deleted_at IS NULL", groupUUID).
		First(&group).Error
	if err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	return &group, nil
}

// loadWritableGroupForUpdate 加载可写的正常群记录（必须未解散且处于正常状态）。
//
// 区分错误类型：
//   - 若群已解散（Status == 2），返回 ErrGroupDismissed；
//   - 若群不存在或已被软删除，返回 ErrRecordNotFound。
func (s *Store) loadWritableGroupForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string) (*model.GroupInfo, error) {
	group, err := s.loadGroupForUpdate(ctx, tx, groupUUID)
	if err != nil {
		return nil, err
	}
	if group.Status == repository.GroupStatusDismissed {
		return nil, repository.ErrGroupDismissed
	}
	if group.Status != repository.GroupStatusNormal {
		return nil, repoerr.ErrRecordNotFound
	}
	return group, nil
}

// loadActiveMemberForUpdate 以行锁方式加载单个有效群成员关系（status=0 且未软删）。
func (s *Store) loadActiveMemberForUpdate(ctx context.Context, tx *gorm.DB, groupUUID, userUUID string) (*model.GroupMember, error) {
	var member model.GroupMember
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL",
			groupUUID,
			userUUID,
			repository.MemberStatusNormal,
		).
		Take(&member).Error
	if err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	return &member, nil
}

// ensureOperatorRoleAtLeast 在写事务内校验操作者是否达到指定最小角色等级（如 minRole=1 表示至少为管理员）。
func (s *Store) ensureOperatorRoleAtLeast(ctx context.Context, tx *gorm.DB, groupUUID, operatorUUID string, minRole int8) (*model.GroupMember, error) {
	member, err := s.loadActiveMemberForUpdate(ctx, tx, groupUUID, operatorUUID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return nil, repository.ErrNoPermission
		}
		return nil, err
	}
	if member.Role < minRole {
		return nil, repository.ErrNoPermission
	}
	return member, nil
}

// ensureOperatorCanAddMembers 校验操作者是否具备加人权限（至少为管理员）。
func (s *Store) ensureOperatorCanAddMembers(ctx context.Context, tx *gorm.DB, groupUUID, operatorUUID string) error {
	_, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleAdmin)
	return err
}

// loadExistingMembersForUpdate 在写事务内批量锁定目标成员集合（包含历史软删除成员）。
//
// 关键设计：
//  1. 必须使用 `Unscoped()`：重新入群时必须感知历史软删（deleted_at != NULL）的成员记录，
//     以便执行恢复（Restore）而不是重复 INSERT 触发唯一索引冲突；
//  2. 保证并发入群/踢人/转让群主操作在行锁内完全串行化。
func (s *Store) loadExistingMembersForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string, userUUIDs []string) (map[string]*model.GroupMember, error) {
	result := make(map[string]*model.GroupMember, len(userUUIDs))
	if len(userUUIDs) == 0 {
		return result, nil
	}
	var members []*model.GroupMember
	if err := tx.WithContext(ctx).
		Unscoped().
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_uuid = ? AND user_uuid IN ?", groupUUID, userUUIDs).
		Find(&members).Error; err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		result[member.UserUuid] = member
	}
	return result, nil
}

// countGroupAdminsForUpdate 在写事务内锁定并统计当前群的有效管理员数量。
//
// 并发配额防护：
//   - 通过 `Clauses(clause.Locking{Strength: "UPDATE"})` 锁定管理员行，彻底避免并发提拔管理员导致突破 10 人上限。
func (s *Store) countGroupAdminsForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string) (int64, error) {
	if tx == nil || groupUUID == "" {
		return 0, nil
	}
	var count int64
	err := tx.WithContext(ctx).
		Model(&model.GroupMember{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"group_uuid = ? AND role = ? AND status = ? AND deleted_at IS NULL",
			groupUUID,
			repository.MemberRoleAdmin,
			repository.MemberStatusNormal,
		).
		Count(&count).Error
	if err != nil {
		return 0, repoerr.WrapDBError(err)
	}
	return count, nil
}

// loadPendingJoinRequestForUpdate 以行锁方式加载单条待审批入群申请。
//
// 避免并发审批同一条申请或审批时被申请人撤销的竞态。
func (s *Store) loadPendingJoinRequestForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string, applyID int64) (*model.GroupJoinRequest, error) {
	var joinRequest model.GroupJoinRequest
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"id = ? AND group_uuid = ? AND status = ? AND deleted_at IS NULL",
			applyID,
			groupUUID,
			repository.JoinRequestStatusPending,
		).
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return nil, repository.ErrGroupApplyNotFound
		}
		return nil, repoerr.WrapDBError(err)
	}
	return &joinRequest, nil
}

// loadPendingJoinRequestByApplicantForUpdate 以行锁方式检查申请人是否已有待处理的申请。
//
// 用于防止同一个用户针对同一个群并发重复提交多条待审批申请。
func (s *Store) loadPendingJoinRequestByApplicantForUpdate(ctx context.Context, tx *gorm.DB, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if tx == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	var joinRequest model.GroupJoinRequest
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"group_uuid = ? AND applicant_uuid = ? AND status = ? AND deleted_at IS NULL",
			groupUUID,
			applicantUUID,
			repository.JoinRequestStatusPending,
		).
		Order("id DESC").
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, repoerr.WrapDBError(err)
	}
	return &joinRequest, nil
}

// loadActiveMemberUUIDs 在事务内加载当前群的所有活跃成员 UUID。
func (s *Store) loadActiveMemberUUIDs(ctx context.Context, tx *gorm.DB, groupUUID string) ([]string, error) {
	if tx == nil || groupUUID == "" {
		return []string{}, nil
	}
	var members []*model.GroupMember
	if err := tx.WithContext(ctx).
		Select("user_uuid").
		Where("group_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, repository.MemberStatusNormal).
		Find(&members).Error; err != nil {
		return nil, repoerr.WrapDBError(err)
	}
	userUUIDs := make([]string, 0, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		userUUIDs = append(userUUIDs, member.UserUuid)
	}
	return userUUIDs, nil
}

// EnsureActiveMemberRole 权威校验用户是有效成员且角色达到要求（点查 MySQL）。
//
// 读侧管理接口（如待审批列表、待办计数）在 cache 确认群状态后，仍用此方法做操作者角色校验：
// 管理权限必须强一致权威校验，防止依赖最终一致缓存导致权限收回滞后产生越权访问。
func (s *Store) EnsureActiveMemberRole(ctx context.Context, groupUUID, userUUID string, minRole int8) (*model.GroupMember, error) {
	var member model.GroupMember
	err := s.db.WithContext(ctx).
		Select("role").
		Where(
			"group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL",
			groupUUID,
			userUUID,
			repository.MemberStatusNormal,
		).
		Take(&member).Error
	if err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return nil, repository.ErrNoPermission
		}
		return nil, repoerr.WrapDBError(err)
	}
	if member.Role < minRole {
		return nil, repository.ErrNoPermission
	}
	return &member, nil
}

// ensureGroupNormalFromDB 仅从 MySQL 确认群仍处于可读正常状态。
//
// 纯 MySQL 查询使用该方法，避免 store 反向依赖 cache。
func (s *Store) ensureGroupNormalFromDB(ctx context.Context, groupUUID string) error {
	if s == nil || s.db == nil || groupUUID == "" {
		return repoerr.ErrRecordNotFound
	}
	var group model.GroupInfo
	err := s.db.WithContext(ctx).
		Select("status").
		Where("uuid = ? AND deleted_at IS NULL", groupUUID).
		Take(&group).Error
	if err != nil {
		return repoerr.WrapDBError(err)
	}
	if group.Status == repository.GroupStatusDismissed {
		return repository.ErrGroupDismissed
	}
	if group.Status != repository.GroupStatusNormal {
		return repoerr.ErrRecordNotFound
	}
	return nil
}
