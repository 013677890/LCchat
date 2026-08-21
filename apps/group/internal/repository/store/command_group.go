package store

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"gorm.io/gorm"
)

// CreateGroup 创建群与初始成员关系。
//
// 关键设计考量：
//  1. 防穿透顺序（先写布隆，再开事务）：在 DB 事务前先执行 `EnsureGroupUUIDInBloom`（BF.ADD）。
//     若先写 DB 成功而布隆写失败，读路径会被布隆直接误判为“群不存在”（假阴性）；而先写布隆即使后续 DB 失败，
//     最多只产生一次可容忍的 DB 回源（假阳性），保证业务正确性；
//  2. 事务原子性：群基本信息、初始成员列表（群主 role=2）、自增 cache_version 和 Outbox 投影事件在同一个事务内提交。
func (s *Store) CreateGroup(ctx context.Context, group *model.GroupInfo, members []*model.GroupMember) error {
	if s == nil || s.db == nil || group == nil || group.Uuid == "" || group.OwnerUuid == "" || len(members) == 0 {
		return fmt.Errorf("%w: invalid create group payload", repoerr.ErrDatabase)
	}
	if group.MemberCnt <= 0 {
		group.MemberCnt = len(members)
	}
	// 1. 事务前先写布隆过滤器，防止假阴性
	if err := repository.EnsureGroupUUIDInBloom(ctx, s.redisClient, group.Uuid); err != nil {
		return repoerr.WrapRedisError(err)
	}
	// 2. 开启 MySQL 事务
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		if err := tx.Create(members).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		if group.UpdatedAt.IsZero() {
			group.UpdatedAt = time.Now()
		}
		// 3. 分配 cache_version 并写入 group_created Outbox 事件
		return s.insertGroupCreatedEvent(tx, group, members)
	})
}

// DismissGroup 解散群。
//
// 业务与一致性规则：
//  1. 重复解散幂等且安全：即使群已解散，也必须先校验群主身份，防止非群主用户借幂等绕过权限检查；
//  2. 软解散策略：只更新 `groups.status = 2 (Dismissed)`，不物理删除成员关系，保留历史审计数据；
//  3. 写入 group_dismissed Outbox 事件，通知 Redis 投影层将成员列表替换为空 Hash tombstone。
func (s *Store) DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 行锁锁定群记录
		group, err := s.loadGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 2. 权限校验（必须为群主）
		if group.OwnerUuid != operatorUUID {
			return repository.ErrNoPermission
		}
		// 3. 幂等处理
		if group.Status == repository.GroupStatusDismissed {
			return nil
		}
		if group.Status != repository.GroupStatusNormal {
			return repoerr.ErrRecordNotFound
		}
		// 4. 查询当前活跃成员 UUID 列表，以便在 Outbox 事件中通知清理各个成员的反向索引
		userUUIDs, err := s.loadActiveMemberUUIDs(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		updatedAt := time.Now()
		// 5. 更新状态为解散
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"status":     repository.GroupStatusDismissed,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		group.Status = repository.GroupStatusDismissed
		group.UpdatedAt = updatedAt
		// 6. 分配新版本并写入 Outbox
		return s.insertGroupDismissedEvent(tx, group, operatorUUID, userUUIDs)
	})
}

// UpdateGroupInfo 更新群资料（名称、头像、加群审批模式）。
//
// 权限分级矩阵：
//   - name / avatar：管理员及以上角色均可更新；
//   - add_mode（加群模式 0=直加, 1=需审批）：敏感加群策略，仅允许群主本人修改。
func (s *Store) UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, updates repository.GroupInfoUpdates) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || updates.IsEmpty() {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁群记录
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 2. 加群模式修改必须为群主
		if updates.AddMode != nil && group.OwnerUuid != operatorUUID {
			return repository.ErrNoPermission
		}
		// 3. 基础资料修改至少需要管理员权限
		if _, err := s.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, repository.MemberRoleAdmin); err != nil {
			return err
		}
		updateMap := buildGroupInfoUpdateMap(group, updates)
		if len(updateMap) == 0 {
			return nil
		}
		updatedAt := time.Now()
		updateMap["updated_at"] = updatedAt
		// 4. 更新数据库
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(updateMap).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		applyGroupInfoUpdates(group, updates)
		group.UpdatedAt = updatedAt
		// 5. 分配版本并落 Outbox 事件
		return s.insertGroupInfoUpdatedEvent(tx, group, operatorUUID)
	})
}

// UpdateGroupNotice 独立更新群公告。
//
// 至少需要管理员权限；若公告内容未发生变化，则短路返回，避免无意义事务开销和 updated_at 刷新。
func (s *Store) UpdateGroupNotice(ctx context.Context, groupUUID, operatorUUID, notice string) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" {
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
		if notice == group.Notice {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"notice":     notice,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		group.Notice = notice
		group.UpdatedAt = updatedAt
		return s.insertGroupInfoUpdatedEvent(tx, group, operatorUUID)
	})
}

// TransferGroupOwner 转让群主身份给指定群成员。
//
// 事务与状态变更原子性：
//  1. 锁 groups 表与两条 group_members 表记录（原群主与新群主）；
//  2. 确认当前操作者是原群主，且新群主是该群的有效成员；
//  3. 同一事务内完成三项修改：`groups.owner_uuid = target`，原群主 `role = 0 (Member)`，新群主 `role = 2 (Owner)`；
//  4. 递增 `cache_version` 并写入 `owner_transferred` Outbox 事件，下游投影层在单次 Lua 中原子更新这两个成员。
func (s *Store) TransferGroupOwner(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string) error {
	if s == nil || s.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	if operatorUUID == targetUserUUID {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁群记录并校验群主身份
		group, err := s.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		if group.OwnerUuid != operatorUUID {
			return repository.ErrNoPermission
		}
		// 2. 批量锁定涉及的两条成员记录
		memberMap, err := s.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{operatorUUID, targetUserUUID})
		if err != nil {
			return err
		}
		currentOwner := memberMap[operatorUUID]
		targetMember := memberMap[targetUserUUID]
		if !isActiveGroupMember(currentOwner) {
			return repository.ErrNoPermission
		}
		if !isActiveGroupMember(targetMember) {
			return repository.ErrGroupMemberNotFound
		}
		if targetMember.Role == repository.MemberRoleOwner {
			return nil
		}
		updatedAt := time.Now()
		// 3. 更新群主 UUID
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"owner_uuid": targetUserUUID,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		// 4. 原群主降级为普通成员
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", currentOwner.Id).
			Updates(map[string]interface{}{
				"role":       repository.MemberRoleMember,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		// 5. 目标成员升级为群主
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", targetMember.Id).
			Updates(map[string]interface{}{
				"role":       repository.MemberRoleOwner,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return repoerr.WrapDBError(err)
		}
		demotedOwner := *currentOwner
		demotedOwner.Role = repository.MemberRoleMember
		demotedOwner.UpdatedAt = updatedAt
		promotedOwner := *targetMember
		promotedOwner.Role = repository.MemberRoleOwner
		promotedOwner.UpdatedAt = updatedAt
		group.OwnerUuid = targetUserUUID
		group.UpdatedAt = updatedAt
		// 6. 分配版本并写入 Outbox
		return s.insertOwnerTransferredEvent(tx, group, operatorUUID, []*model.GroupMember{&demotedOwner, &promotedOwner})
	})
}

// buildGroupInfoUpdateMap 只提取真实变化字段，避免无意义更新打乱排序。
func buildGroupInfoUpdateMap(group *model.GroupInfo, updates repository.GroupInfoUpdates) map[string]interface{} {
	updateMap := make(map[string]interface{}, 3)
	if group == nil {
		return updateMap
	}
	if updates.Name != nil && *updates.Name != group.Name {
		updateMap["name"] = *updates.Name
	}
	if updates.Avatar != nil && *updates.Avatar != group.Avatar {
		updateMap["avatar"] = *updates.Avatar
	}
	if updates.AddMode != nil && *updates.AddMode != group.AddMode {
		updateMap["add_mode"] = *updates.AddMode
	}
	return updateMap
}

// applyGroupInfoUpdates 把已经提交成功的变更同步回内存对象，供事件快照复用。
func applyGroupInfoUpdates(group *model.GroupInfo, updates repository.GroupInfoUpdates) {
	if group == nil {
		return
	}
	if updates.Name != nil {
		group.Name = *updates.Name
	}
	if updates.Avatar != nil {
		group.Avatar = *updates.Avatar
	}
	if updates.AddMode != nil {
		group.AddMode = *updates.AddMode
	}
}
