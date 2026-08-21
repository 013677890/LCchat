package store

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"gorm.io/gorm"
)

// ApplyJoinGroup 按 add_mode 执行直加入群或创建待审批申请。
func (s *Store) ApplyJoinGroup(ctx context.Context, groupUUID, applicantUUID, reason string) (repository.ApplyJoinGroupResult, error) {
	result := repository.ApplyJoinGroupResult{}
	if s == nil || s.db == nil || groupUUID == "" || applicantUUID == "" {
		return result, nil
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		existingMap, err := s.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{applicantUUID})
		if err != nil {
			return err
		}
		if isActiveGroupMember(existingMap[applicantUUID]) {
			return repository.ErrAlreadyGroupMember
		}
		if group.AddMode == 0 {
			now := time.Now()
			existing := existingMap[applicantUUID]
			var changedMember *model.GroupMember
			if existing == nil {
				member := &model.GroupMember{
					GroupUuid: groupUUID,
					UserUuid:  applicantUUID,
					Role:      repository.MemberRoleMember,
					Status:    repository.MemberStatusNormal,
					Inviter:   applicantUUID,
					JoinedAt:  now,
				}
				if err := tx.Create(member).Error; err != nil {
					return repoerr.WrapDBError(err)
				}
				changedMember = member
			} else {
				if err := tx.Unscoped().Model(&model.GroupMember{}).
					Where("id = ?", existing.Id).
					Updates(map[string]interface{}{
						"status":       repository.MemberStatusNormal,
						"role":         repository.MemberRoleMember,
						"inviter_uuid": applicantUUID,
						"joined_at":    now,
						"updated_at":   now,
						"deleted_at":   nil,
					}).Error; err != nil {
					return repoerr.WrapDBError(err)
				}
				restored := *existing
				restored.Status = repository.MemberStatusNormal
				restored.Role = repository.MemberRoleMember
				restored.Inviter = applicantUUID
				restored.JoinedAt = now
				restored.UpdatedAt = now
				restored.DeletedAt = gorm.DeletedAt{}
				changedMember = &restored
			}
			if err := tx.Model(&model.GroupInfo{}).
				Where("id = ?", group.Id).
				Updates(map[string]interface{}{
					"member_cnt": gorm.Expr("member_cnt + 1"),
					"updated_at": now,
				}).Error; err != nil {
				return repoerr.WrapDBError(err)
			}
			group.MemberCnt++
			group.UpdatedAt = now
			result.JoinedDirectly = true
			return s.insertMemberAddedEvent(tx, group, applicantUUID, []*model.GroupMember{changedMember})
		}
		pendingRequest, err := s.loadPendingJoinRequestByApplicantForUpdate(ctx, tx, groupUUID, applicantUUID)
		if err != nil {
			return err
		}
		if pendingRequest != nil {
			return repository.ErrGroupApplyAlreadyExists
		}
		joinRequest := &model.GroupJoinRequest{
			GroupUuid:     groupUUID,
			ApplicantUuid: applicantUUID,
			Status:        repository.JoinRequestStatusPending,
			Reason:        reason,
		}
		if err := tx.Create(joinRequest).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		result.ApplyID = joinRequest.Id
		return s.insertJoinRequestCreatedEvent(tx, groupUUID, applicantUUID, joinRequest)
	})
	if err != nil {
		return repository.ApplyJoinGroupResult{}, err
	}
	return result, nil
}

// CancelJoinGroupApplication 撤销当前用户自己发起的待审批入群申请。
func (s *Store) CancelJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) error {
	if s == nil || s.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID); err != nil {
			return err
		}
		joinRequest, err := s.loadPendingJoinRequestByApplicantForUpdate(ctx, tx, groupUUID, applicantUUID)
		if err != nil {
			return err
		}
		if joinRequest == nil {
			return repository.ErrGroupApplyNotFound
		}
		now := time.Now()
		if err := tx.Model(&model.GroupJoinRequest{}).
			Where("id = ?", joinRequest.Id).
			Updates(map[string]interface{}{
				"status":     repository.JoinRequestStatusCanceled,
				"updated_at": now,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		joinRequest.Status = repository.JoinRequestStatusCanceled
		joinRequest.UpdatedAt = now
		return s.insertJoinRequestCanceledEvent(tx, groupUUID, applicantUUID, joinRequest)
	})
}

// ReviewJoinGroup 审批入群申请。
//
// 拒绝只更新申请状态；通过时若申请人还不是有效成员，则创建或恢复成员关系，
// 并在同一事务内连续写入 join_request_reviewed 与 member_added 两条版本化事件。
func (s *Store) ReviewJoinGroup(ctx context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || applyID <= 0 {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		if _, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleAdmin); err != nil {
			return err
		}
		joinRequest, err := s.loadPendingJoinRequestForUpdate(ctx, tx, groupUUID, applyID)
		if err != nil {
			return err
		}
		now := time.Now()
		requestUpdates := map[string]interface{}{
			"status":        actionToJoinRequestStatus(action),
			"reviewer_uuid": operatorUUID,
			"review_remark": remark,
			"reviewed_at":   now,
			"updated_at":    now,
		}
		if action == repository.JoinRequestStatusRejected {
			if err := tx.Model(&model.GroupJoinRequest{}).
				Where("id = ?", joinRequest.Id).
				Updates(requestUpdates).Error; err != nil {
				return repoerr.WrapDBError(err)
			}
			return s.insertJoinRequestReviewedEvent(tx, groupUUID, operatorUUID, joinRequest)
		}
		existingMap, err := s.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{joinRequest.ApplicantUuid})
		if err != nil {
			return err
		}
		existing := existingMap[joinRequest.ApplicantUuid]
		var changedMember *model.GroupMember
		if !isActiveGroupMember(existing) {
			if existing == nil {
				member := &model.GroupMember{
					GroupUuid: groupUUID,
					UserUuid:  joinRequest.ApplicantUuid,
					Role:      repository.MemberRoleMember,
					Status:    repository.MemberStatusNormal,
					Inviter:   operatorUUID,
					JoinedAt:  now,
				}
				if err := tx.Create(member).Error; err != nil {
					return repoerr.WrapDBError(err)
				}
				changedMember = member
			} else {
				if err := tx.Unscoped().Model(&model.GroupMember{}).
					Where("id = ?", existing.Id).
					Updates(map[string]interface{}{
						"status":       repository.MemberStatusNormal,
						"role":         repository.MemberRoleMember,
						"inviter_uuid": operatorUUID,
						"joined_at":    now,
						"updated_at":   now,
						"deleted_at":   nil,
					}).Error; err != nil {
					return repoerr.WrapDBError(err)
				}
				restored := *existing
				restored.Status = repository.MemberStatusNormal
				restored.Role = repository.MemberRoleMember
				restored.Inviter = operatorUUID
				restored.JoinedAt = now
				restored.UpdatedAt = now
				restored.DeletedAt = gorm.DeletedAt{}
				changedMember = &restored
			}
			if err := tx.Model(&model.GroupInfo{}).
				Where("id = ?", group.Id).
				Updates(map[string]interface{}{
					"member_cnt": gorm.Expr("member_cnt + 1"),
					"updated_at": now,
				}).Error; err != nil {
				return repoerr.WrapDBError(err)
			}
			group.MemberCnt++
			group.UpdatedAt = now
		}
		if err := tx.Model(&model.GroupJoinRequest{}).
			Where("id = ?", joinRequest.Id).
			Updates(requestUpdates).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		if err := s.insertJoinRequestReviewedEvent(tx, groupUUID, operatorUUID, joinRequest); err != nil {
			return err
		}
		if changedMember == nil {
			return nil
		}
		return s.insertMemberAddedEvent(tx, group, operatorUUID, []*model.GroupMember{changedMember})
	})
}

// GetMyJoinGroupApplication 获取当前用户在指定群的最新入群申请状态。
func (s *Store) GetMyJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if s == nil || s.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	if err := s.ensureGroupNormalFromDB(ctx, groupUUID); err != nil {
		return nil, err
	}
	return s.loadLatestJoinRequestByApplicant(ctx, groupUUID, applicantUUID)
}

// loadLatestJoinRequestByApplicant 读取当前用户在指定群的最新申请记录。
func (s *Store) loadLatestJoinRequestByApplicant(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if s == nil || s.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	var joinRequest model.GroupJoinRequest
	err := s.db.WithContext(ctx).
		Where("group_uuid = ? AND applicant_uuid = ? AND deleted_at IS NULL", groupUUID, applicantUUID).
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

// GetJoinRequestApplicant 获取指定申请对应的申请人 UUID。
func (s *Store) GetJoinRequestApplicant(ctx context.Context, groupUUID string, applyID int64) (string, error) {
	if s == nil || s.db == nil || groupUUID == "" || applyID <= 0 {
		return "", nil
	}
	var joinRequest model.GroupJoinRequest
	err := s.db.WithContext(ctx).
		Select("applicant_uuid").
		Where("id = ? AND group_uuid = ? AND deleted_at IS NULL", applyID, groupUUID).
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return "", nil
		}
		return "", repoerr.WrapDBError(err)
	}
	return joinRequest.ApplicantUuid, nil
}
