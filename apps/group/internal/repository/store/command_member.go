package store

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LeaveGroup 处理当前用户主动退群（复用 RemoveMember 逻辑，操作者与目标均为本人）。
func (s *Store) LeaveGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	return s.RemoveMember(ctx, groupUUID, operatorUUID, operatorUUID)
}

// AddMembers 批量向群内添加/邀请成员。
//
// 关键设计与并发处理：
//  1. 权限校验：在事务行锁内读取操作者最新角色，必须至少为管理员（Admin）；
//  2. 识别恢复与全新成员：使用 Unscoped().FOR UPDATE 锁定目标成员集合，
//     - 若用户曾退群/被踢（存在软删除记录），执行 UPDATE 恢复为正常状态（status=0, role=0, deleted_at=nil, 刷新 joined_at 和 inviter_uuid），
//     强制重置角色为普通成员，防止旧管理员身份在重新入群后被意外复活；
//     - 若用户为全新成员，批量 INSERT（使用 ON CONFLICT DO NOTHING 防重）；
//  3. 人数原子累加：`UPDATE groups SET member_cnt = member_cnt + delta`；
//  4. 产生事件：递增 `cache_version` 并写入 `member_added` Outbox 事件，驱动 Redis 投影。
func (s *Store) AddMembers(ctx context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || len(members) == 0 {
		return nil
	}
	now := time.Now()
	newMembers := make([]*model.GroupMember, 0, len(members))
	restoredMembers := make([]*model.GroupMember, 0, len(members))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁群记录
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 2. 校验加人权限
		if err := s.ensureOperatorCanAddMembers(ctx, tx, groupUUID, operatorUUID); err != nil {
			return err
		}
		// 3. 锁定目标用户已有记录（包含历史退群/被踢成员）
		existingMap, err := s.loadExistingMembersForUpdate(ctx, tx, groupUUID, collectWriteMemberUUIDs(members))
		if err != nil {
			return err
		}
		pendingCreates := make([]*model.GroupMember, 0, len(members))
		restoredCount := 0
		seen := make(map[string]struct{}, len(members))
		for _, member := range members {
			if member == nil || member.UserUuid == "" {
				continue
			}
			if _, exists := seen[member.UserUuid]; exists {
				continue
			}
			seen[member.UserUuid] = struct{}{}
			existing, exists := existingMap[member.UserUuid]
			if !exists {
				// 全新成员：准备批量 INSERT
				created := &model.GroupMember{
					GroupUuid: groupUUID,
					UserUuid:  member.UserUuid,
					Role:      repository.MemberRoleMember,
					Status:    repository.MemberStatusNormal,
					Inviter:   operatorUUID,
					JoinedAt:  now,
				}
				pendingCreates = append(pendingCreates, created)
				newMembers = append(newMembers, created)
				continue
			}
			// 已是在群正常成员，幂等忽略
			if existing.Status == repository.MemberStatusNormal && !existing.DeletedAt.Valid {
				continue
			}
			// 历史退群/被踢成员：执行恢复（清空软删，强制重置为普通成员 role=0）
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
			restoredMembers = append(restoredMembers, &restored)
			restoredCount++
		}
		insertedCount := 0
		if len(pendingCreates) > 0 {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(pendingCreates)
			if result.Error != nil {
				return repoerr.WrapDBError(result.Error)
			}
			insertedCount = int(result.RowsAffected)
		}
		delta := insertedCount + restoredCount
		if delta == 0 {
			return nil
		}
		// 4. 原子更新群成员人数
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("member_cnt", gorm.Expr("member_cnt + ?", delta)).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		group.MemberCnt += delta
		group.UpdatedAt = now
		changedMembers := make([]*model.GroupMember, 0, len(newMembers)+len(restoredMembers))
		changedMembers = append(changedMembers, newMembers...)
		changedMembers = append(changedMembers, restoredMembers...)
		// 5. 递增版本并写入 Outbox
		return s.insertMemberAddedEvent(tx, group, operatorUUID, changedMembers)
	})
}

// RemoveMember 移除群成员（主动退群或管理员踢人）。
//
// 权限与安全保护规则：
//  1. 群主保护：群主不能退群（需先转让群主），且任何人都不能踢群主；
//  2. 等级保护：管理员只能踢普通成员，不能踢其他管理员或群主；
//  3. 普通成员只能退自己，不能踢他人；
//  4. 软删除策略：设置 `deleted_at = now()`，区分 `status = quit (退群)` 与 `status = kicked (被踢)`；
//  5. 原子扣减人数：`member_cnt = CASE WHEN member_cnt > 0 THEN member_cnt - 1 ELSE 0 END`。
func (s *Store) RemoveMember(ctx context.Context, groupUUID, operatorUUID, targetUUID string) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || targetUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁群记录
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 2. 批量锁定操作者和目标成员
		memberMap, err := s.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{operatorUUID, targetUUID})
		if err != nil {
			return err
		}
		var operator *model.GroupMember
		if operatorUUID != targetUUID {
			operator = memberMap[operatorUUID]
			if !isActiveGroupMember(operator) {
				return repository.ErrNoPermission
			}
		}
		target := memberMap[targetUUID]
		if !isActiveGroupMember(target) {
			if operator != nil && operator.Role < repository.MemberRoleAdmin {
				return repository.ErrNoPermission
			}
			return nil
		}
		// 群主保护：不可退群或被踢
		if target.Role == repository.MemberRoleOwner {
			if operatorUUID == targetUUID {
				return repository.ErrCannotQuitAsOwner
			}
			return repository.ErrCannotKickOwner
		}
		// 权限矩阵判断
		if operator != nil && !canRemoveGroupMember(operator.Role, target.Role) {
			return repository.ErrNoPermission
		}
		now := time.Now()
		status := repository.MemberStatusQuit
		if operatorUUID != targetUUID {
			status = repository.MemberStatusKicked
		}
		// 3. 软删除该成员记录
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", target.Id).
			Updates(map[string]interface{}{
				"status":     status,
				"updated_at": now,
				"deleted_at": now,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		// 4. 人数防负数安全扣减
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("member_cnt", gorm.Expr("CASE WHEN member_cnt > 0 THEN member_cnt - 1 ELSE 0 END")).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		if group.MemberCnt > 0 {
			group.MemberCnt--
		}
		group.UpdatedAt = now
		// 5. 递增版本并落 Outbox 事件
		return s.insertMemberRemovedEvent(tx, group, operatorUUID, targetUUID)
	})
}

// UpdateMemberRole 更新群成员角色（设为管理员或取消管理员）。
//
// 业务约束：
//  1. 仅群主拥有调整成员角色的权限；
//  2. 提拔管理员时，在事务内加排他锁统计管理员总数（countGroupAdminsForUpdate），严格限制管理员上限为 10 人。
func (s *Store) UpdateMemberRole(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, role int8) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		if group.OwnerUuid != operatorUUID {
			return repository.ErrNoPermission
		}
		operator, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleOwner)
		if err != nil {
			return err
		}
		targetMember, err := s.loadActiveMemberForUpdate(ctx, tx, groupUUID, targetUserUUID)
		if err != nil {
			if errors.Is(err, repoerr.ErrRecordNotFound) {
				return repository.ErrGroupMemberNotFound
			}
			return err
		}
		if !canUpdateGroupMemberRole(operator.Role, targetMember.Role, role) {
			return repository.ErrNoPermission
		}
		if targetMember.Role == role {
			return nil
		}
		// 检查 10 位管理员配额
		if role == repository.MemberRoleAdmin && targetMember.Role != repository.MemberRoleAdmin {
			adminCount, err := s.countGroupAdminsForUpdate(ctx, tx, groupUUID)
			if err != nil {
				return err
			}
			if adminCount >= repository.MaxGroupAdminCount {
				return repository.ErrAdminLimitExceeded
			}
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", targetMember.Id).
			Updates(map[string]interface{}{
				"role":       role,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		updatedMember := *targetMember
		updatedMember.Role = role
		updatedMember.UpdatedAt = updatedAt
		group.UpdatedAt = updatedAt
		return s.insertMemberRoleUpdatedEvent(tx, group, operatorUUID, []*model.GroupMember{&updatedMember})
	})
}

// UpdateMyGroupNickname 更新当前用户本人的群名片。
func (s *Store) UpdateMyGroupNickname(ctx context.Context, groupUUID, userUUID, nickname string) error {
	if s == nil || s.db == nil || groupUUID == "" || userUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		member, err := s.loadActiveMemberForUpdate(ctx, tx, groupUUID, userUUID)
		if err != nil {
			if errors.Is(err, repoerr.ErrRecordNotFound) {
				return repository.ErrNoPermission
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
			return repoerr.WrapDBError(err)
		}
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		updatedMember := *member
		updatedMember.Remark = nickname
		updatedMember.UpdatedAt = updatedAt
		group.UpdatedAt = updatedAt
		return s.insertMemberProfileUpdatedEvent(tx, group, userUUID, []*model.GroupMember{&updatedMember})
	})
}

// UpdateGroupMemberNickname 管理员或群主修改指定成员的群名片。
//
// 权限矩阵：群主可修改所有人名片；管理员只能修改普通成员名片，不能改群主或其他管理员。
func (s *Store) UpdateGroupMemberNickname(ctx context.Context, groupUUID, operatorUUID, targetUserUUID, nickname string) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	if operatorUUID == targetUserUUID {
		return repository.ErrNoPermission
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		operator, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleAdmin)
		if err != nil {
			return err
		}
		target, err := s.loadActiveMemberForUpdate(ctx, tx, groupUUID, targetUserUUID)
		if err != nil {
			if errors.Is(err, repoerr.ErrRecordNotFound) {
				return repository.ErrGroupMemberNotFound
			}
			return err
		}
		if !canUpdateGroupMemberNickname(operator.Role, target.Role) {
			return repository.ErrNoPermission
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
			return repoerr.WrapDBError(err)
		}
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		updatedMember := *target
		updatedMember.Remark = nickname
		updatedMember.UpdatedAt = updatedAt
		group.UpdatedAt = updatedAt
		return s.insertMemberProfileUpdatedEvent(tx, group, operatorUUID, []*model.GroupMember{&updatedMember})
	})
}

// MuteGroupMember 设置或取消指定成员的单人禁言。
//
// 权限矩阵：群主可禁言管理员与普通成员；管理员只能禁言普通成员。
func (s *Store) MuteGroupMember(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, muteUntil *time.Time) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		operator, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleAdmin)
		if err != nil {
			return err
		}
		target, err := s.loadActiveMemberForUpdate(ctx, tx, groupUUID, targetUserUUID)
		if err != nil {
			if errors.Is(err, repoerr.ErrRecordNotFound) {
				return repository.ErrGroupMemberNotFound
			}
			return err
		}
		if !canMuteGroupMember(operator.Role, target.Role) {
			return repository.ErrNoPermission
		}
		if repository.MuteUntilEqual(target.MuteUntil, muteUntil) {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", target.Id).
			Updates(map[string]interface{}{
				"mute_until": muteUntil,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		updatedMember := *target
		updatedMember.MuteUntil = repository.CloneTimePtr(muteUntil)
		updatedMember.UpdatedAt = updatedAt
		group.UpdatedAt = updatedAt
		return s.insertMemberMutedEvent(tx, group, operatorUUID, []*model.GroupMember{&updatedMember})
	})
}

// UpdateGroupMuteSetting 更新群全员禁言开关（mute_all）。
//
// 需要管理员及以上权限。开启全员禁言后，普通成员（role=0）将被禁止发消息，管理员与群主不受限制。
func (s *Store) UpdateGroupMuteSetting(ctx context.Context, groupUUID, operatorUUID string, muteAll bool) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁群记录
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 2. 校验管理员权限
		if _, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleAdmin); err != nil {
			return err
		}
		if group.MuteAll == muteAll {
			return nil
		}
		updatedAt := time.Now()
		// 3. 更新全员禁言状态
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"mute_all":   muteAll,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		group.MuteAll = muteAll
		group.UpdatedAt = updatedAt
		// 4. 分配版本并落 Outbox
		return s.insertGroupMuteSettingUpdatedEvent(tx, group, operatorUUID)
	})
}
