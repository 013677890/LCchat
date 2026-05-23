package repository

import (
	"context"
	"errors"
	"fmt"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// groupRepositoryImpl 是 group-service 仓储层的读写实现。
//
// 当前聚焦群资料与群成员的核心读写闭环，主要承接：
//  1. 群资料查询；
//  2. 群成员查询；
//  3. 当前用户所属群列表查询；
//  4. 群创建、资料更新、成员增删、解散；
//  5. 成员昵称头像补齐所需的用户资料批量查询。
type groupRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
	memberGroup singleflight.Group
}

const (
	groupStatusNormal         int8  = 0
	groupStatusDismissed      int8  = 2
	memberRoleMember          int8  = 0
	memberRoleAdmin           int8  = 1
	memberRoleOwner           int8  = 2
	memberStatusNormal        int8  = 0
	memberStatusQuit          int8  = 1
	memberStatusKicked        int8  = 2
	joinRequestStatusPending  int8  = 0
	joinRequestStatusApproved int8  = 1
	joinRequestStatusRejected int8  = 2
	joinRequestStatusCanceled int8  = 3
	maxGroupAdminCount        int64 = 10
)

// NewGroupRepository 创建 group 仓储实例。
//
// 当前仍保持薄构造：
//  1. 只接收 gorm.DB；
//  2. 不在构造阶段探测连通性；
//  3. 由上层 provider 负责基础设施初始化与失败处理。
func NewGroupRepository(db *gorm.DB, redisClient *goredis.Client) IGroupRepository {
	return &groupRepositoryImpl{db: db, redisClient: redisClient}
}

// CreateGroup 创建群与初始成员关系。
func (r *groupRepositoryImpl) CreateGroup(ctx context.Context, group *model.GroupInfo, members []*model.GroupMember) error {
	// repository 再做一次最小防御校验，避免 service 以外的调用方写入半成品聚合。
	if r == nil || r.db == nil || group == nil || group.Uuid == "" || group.OwnerUuid == "" || len(members) == 0 {
		return fmt.Errorf("%w: invalid create group payload", ErrDatabase)
	}
	if group.MemberCnt <= 0 {
		// member_cnt 以初始成员关系为准，避免上层漏填导致群人数与成员表不一致。
		group.MemberCnt = len(members)
	}
	// 群基础资料和初始成员必须同事务写入，否则会出现“有群无群主”的不可恢复状态。
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return WrapDBError(err)
		}
		if err := tx.Create(members).Error; err != nil {
			return WrapDBError(err)
		}
		// GORM 通常会在 Create 后回填时间字段，这里再做一次兜底，
		// 保证写入 outbox 的快照一定带有可用于排序的更新时间。
		if group.UpdatedAt.IsZero() {
			group.UpdatedAt = time.Now()
		}
		return r.insertGroupCreatedEvent(tx, group, members)
	})
	if err != nil {
		return err
	}
	// 创建群后缓存用全量重建，确保首批成员关系一次性成为缓存事实。
	r.rebuildGroupMembersCacheAsync(ctx, group.Uuid, members)
	return nil
}

// AddMembers 向群内添加成员。
func (r *groupRepositoryImpl) AddMembers(ctx context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || len(members) == 0 {
		return nil
	}
	// 同一批新增/恢复成员共享时间戳，避免成员列表排序因毫秒差异产生不稳定顺序。
	now := time.Now()
	// newMembers 用于缓存插入；restoredMembers 用于缓存 upsert，两类变更的 patch 语义不同。
	newMembers := make([]*model.GroupMember, 0, len(members))
	restoredMembers := make([]*model.GroupMember, 0, len(members))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先锁群记录，保证解散/禁用与成员写入之间不会并发穿插。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 添加成员只允许管理员及以上角色，权限判断必须在事务锁内读取最新角色。
		if err := r.ensureOperatorCanAddMembers(ctx, tx, groupUUID, operatorUUID); err != nil {
			return err
		}
		// Unscoped 查询可同时发现“已退出/被踢/软删”的历史成员，用于幂等恢复入群。
		existingMap, err := r.loadExistingMembersForUpdate(ctx, tx, groupUUID, collectWriteMemberUUIDs(members))
		if err != nil {
			return err
		}
		pendingCreates := make([]*model.GroupMember, 0, len(members))
		restoredCount := 0
		// 请求内重复 UUID 只处理一次，避免 member_cnt 和缓存 patch 被重复累加。
		seen := make(map[string]struct{}, len(members))
		for _, member := range members {
			if member == nil || member.UserUuid == "" {
				continue
			}
			if _, ok := seen[member.UserUuid]; ok {
				continue
			}
			seen[member.UserUuid] = struct{}{}
			existing, exists := existingMap[member.UserUuid]
			if !exists {
				// 新成员走批量 Create，具体角色/邀请人以当前操作者和业务默认值落库。
				created := &model.GroupMember{
					GroupUuid: groupUUID,
					UserUuid:  member.UserUuid,
					Role:      0,
					Status:    0,
					Inviter:   operatorUUID,
					JoinedAt:  now,
				}
				pendingCreates = append(pendingCreates, created)
				newMembers = append(newMembers, created)
				continue
			}
			if existing.Status == 0 && !existing.DeletedAt.Valid {
				// 已经是有效成员时按幂等成功处理，不重复增加 member_cnt。
				continue
			}
			// 历史成员重新入群要清空软删标记，并重置普通成员角色，避免旧管理员身份复活。
			if err := tx.Unscoped().Model(&model.GroupMember{}).
				Where("id = ?", existing.Id).
				Updates(map[string]interface{}{
					"status":       0,
					"role":         0,
					"inviter_uuid": operatorUUID,
					"joined_at":    now,
					"updated_at":   now,
					"deleted_at":   nil,
				}).Error; err != nil {
				return WrapDBError(err)
			}
			restored := *existing
			restored.Status = 0
			restored.Role = 0
			restored.Inviter = operatorUUID
			restored.JoinedAt = now
			restored.UpdatedAt = now
			restored.DeletedAt = gorm.DeletedAt{}
			restoredMembers = append(restoredMembers, &restored)
			restoredCount++
		}
		insertedCount := 0
		if len(pendingCreates) > 0 {
			// OnConflict 兜底处理极端并发重复插入，最终 member_cnt 只按实际影响行数增加。
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(pendingCreates)
			if result.Error != nil {
				return WrapDBError(result.Error)
			}
			insertedCount = int(result.RowsAffected)
		}
		delta := insertedCount + restoredCount
		if delta == 0 {
			// 本批全部为已在群成员时，不刷新群更新时间，也不触发缓存写入。
			return nil
		}
		// 群人数只在真实新增/恢复成员时递增，和 group_members 有效记录保持一致。
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("member_cnt", gorm.Expr("member_cnt + ?", delta)).Error; err != nil {
			return WrapDBError(err)
		}
		// 事件快照需要携带最新 member_cnt 和 updated_at，
		// 这样 projector 在 patch info / user_groups 时才能和 DB 排序保持一致。
		group.MemberCnt += delta
		group.UpdatedAt = now
		changedMembers := make([]*model.GroupMember, 0, len(newMembers)+len(restoredMembers))
		changedMembers = append(changedMembers, newMembers...)
		changedMembers = append(changedMembers, restoredMembers...)
		return r.insertMemberAddedEvent(tx, group, operatorUUID, changedMembers)
	})
	if err != nil {
		return err
	}
	if len(newMembers) > 0 {
		// 缓存存在时增量插入新 field；缓存缺失则留给读路径全量重建，避免局部集合污染缓存。
		r.insertGroupMembersCacheAsync(ctx, groupUUID, newMembers)
	}
	for _, member := range restoredMembers {
		// 恢复成员可能覆盖旧角色/时间/删除态，所以使用 upsert patch 更新完整字段。
		r.upsertGroupMemberCacheAsync(ctx, groupUUID, member)
	}
	return nil
}

// RemoveMember 移除或退出群成员。
func (r *groupRepositoryImpl) RemoveMember(ctx context.Context, groupUUID, operatorUUID, targetUUID string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || targetUUID == "" {
		return nil
	}
	// 只有事务内确认真实移除后才 patch 缓存，幂等空操作不需要打扰 Redis。
	removed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁群记录用于串行化“解散群”和“移除成员”两类写操作。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 操作者和目标成员一起加锁，避免角色变更/退群与本次移除判断交叉。
		memberMap, err := r.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{operatorUUID, targetUUID})
		if err != nil {
			return err
		}
		var operator *model.GroupMember
		if operatorUUID != targetUUID {
			// 非本人退群时，操作者必须仍是有效成员，否则没有任何管理权限。
			operator = memberMap[operatorUUID]
			if !isActiveGroupMember(operator) {
				return ErrNoPermission
			}
		}
		target := memberMap[targetUUID]
		if !isActiveGroupMember(target) {
			// 目标已经不在群时按幂等成功处理，但普通成员不能借此探测或操作他人。
			if operator != nil && operator.Role < memberRoleAdmin {
				return ErrNoPermission
			}
			return nil
		}
		if target.Role == memberRoleOwner {
			// 群主生命周期必须通过转让/解散处理，不能被踢出或直接退群。
			if operatorUUID == targetUUID {
				return ErrCannotQuitAsOwner
			}
			return ErrCannotKickOwner
		}
		if operator != nil {
			// 管理员只能踢普通成员，群主才能踢管理员，具体矩阵集中在 helper 内维护。
			if !canRemoveGroupMember(operator.Role, target.Role) {
				return ErrNoPermission
			}
		}
		now := time.Now()
		status := memberStatusQuit
		if operatorUUID != targetUUID {
			// 区分主动退群和被踢出，便于后续审计或重新入群策略扩展。
			status = memberStatusKicked
		}
		// 软删成员关系，让读路径天然只看到有效成员，同时保留历史记录供恢复/审计。
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", target.Id).
			Updates(map[string]interface{}{
				"status":     status,
				"updated_at": now,
				"deleted_at": now,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		// 使用 CASE 防御异常数据，避免成员数被并发/脏数据扣成负数。
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("member_cnt", gorm.Expr("CASE WHEN member_cnt > 0 THEN member_cnt - 1 ELSE 0 END")).Error; err != nil {
			return WrapDBError(err)
		}
		// 删除事件同样要带上最新群快照，
		// 否则缓存侧 member_count 与最近更新时间会落后于事实库。
		if group.MemberCnt > 0 {
			group.MemberCnt--
		}
		group.UpdatedAt = now
		removed = true
		return r.insertMemberRemovedEvent(tx, group, operatorUUID, targetUUID)
	})
	if err != nil {
		return err
	}
	if removed {
		// 只删除单个成员 field，其他成员缓存保持可用，降低写路径缓存成本。
		r.removeGroupMemberCacheAsync(ctx, groupUUID, targetUUID)
	}
	return nil
}

// DismissGroup 解散群。
func (r *groupRepositoryImpl) DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return nil
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 读取并锁定群记录，确保并发解散时只有一个事务执行状态变更。
		group, err := r.loadGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 重复解散也必须校验群主身份，避免非群主借幂等语义绕过权限。
		if group.OwnerUuid != operatorUUID {
			return ErrNoPermission
		}
		if group.Status == groupStatusDismissed {
			// 已解散视为幂等成功，不批量触碰成员表，避免大群长事务。
			return nil
		}
		if group.Status != groupStatusNormal {
			return ErrRecordNotFound
		}
		userUUIDs, err := r.loadActiveMemberUUIDs(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 解散只更新群状态；成员历史保留，读写路径统一通过群状态拦截。
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"status":     groupStatusDismissed,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		group.Status = groupStatusDismissed
		group.UpdatedAt = updatedAt
		return r.insertGroupDismissedEvent(tx, group, operatorUUID, userUUIDs)
	})
	if err != nil {
		return err
	}
	// 解散后成员缓存不能继续支撑权限判断，直接删除整份缓存比逐个 patch 更安全。
	r.deleteGroupMembersCacheAsync(ctx, groupUUID)
	return nil
}

// UpdateGroupInfo 更新群资料。
func (r *groupRepositoryImpl) UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, updates GroupInfoUpdates) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || updates.IsEmpty() {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁群记录保证资料更新不会和解散/禁用状态变更交叉提交。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// name/avatar 允许管理员及以上更新；add_mode 只能由群主修改。
		if updates.AddMode != nil && group.OwnerUuid != operatorUUID {
			return ErrNoPermission
		}
		// 角色仍在事务内读取，避免并发退群/转让群主后继续写入资料。
		if _, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin); err != nil {
			return err
		}
		updateMap := buildGroupInfoUpdateMap(group, updates)
		if len(updateMap) == 0 {
			// 无实际变化时不刷新 updated_at，保持读侧排序稳定。
			return nil
		}
		updatedAt := time.Now()
		updateMap["updated_at"] = updatedAt
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(updateMap).Error; err != nil {
			return WrapDBError(err)
		}
		applyGroupInfoUpdates(group, updates)
		group.UpdatedAt = updatedAt
		return r.insertGroupInfoUpdatedEvent(tx, group, operatorUUID)
	})
}

// UpdateGroupNotice 独立更新群公告。
func (r *groupRepositoryImpl) UpdateGroupNotice(ctx context.Context, groupUUID, operatorUUID, notice string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 公告更新与群状态、解散、其他资料写入共用同一个群维度串行边界。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 公告属于对全体成员可见的正式资料，仍要求管理员及以上角色维护。
		if _, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin); err != nil {
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
			return WrapDBError(err)
		}
		group.Notice = notice
		group.UpdatedAt = updatedAt
		return r.insertGroupInfoUpdatedEvent(tx, group, operatorUUID)
	})
}

// TransferGroupOwner 转让群主。
func (r *groupRepositoryImpl) TransferGroupOwner(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	if operatorUUID == targetUserUUID {
		return nil
	}
	changedMembers := make([]*model.GroupMember, 0, 2)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 群主转让会同时改动群表与两条成员关系，必须放在同一事务里完成。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		if group.OwnerUuid != operatorUUID {
			return ErrNoPermission
		}
		memberMap, err := r.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{operatorUUID, targetUserUUID})
		if err != nil {
			return err
		}
		currentOwner := memberMap[operatorUUID]
		targetMember := memberMap[targetUserUUID]
		if !isActiveGroupMember(currentOwner) {
			return ErrNoPermission
		}
		if !isActiveGroupMember(targetMember) {
			return ErrGroupMemberNotFound
		}
		if targetMember.Role == memberRoleOwner {
			return nil
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"owner_uuid": targetUserUUID,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", currentOwner.Id).
			Updates(map[string]interface{}{
				"role":       memberRoleMember,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", targetMember.Id).
			Updates(map[string]interface{}{
				"role":       memberRoleOwner,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		demotedOwner := *currentOwner
		demotedOwner.Role = memberRoleMember
		demotedOwner.UpdatedAt = updatedAt
		promotedOwner := *targetMember
		promotedOwner.Role = memberRoleOwner
		promotedOwner.UpdatedAt = updatedAt
		changedMembers = []*model.GroupMember{&demotedOwner, &promotedOwner}
		group.OwnerUuid = targetUserUUID
		group.UpdatedAt = updatedAt
		return r.insertOwnerTransferredEvent(tx, group, operatorUUID, changedMembers)
	})
	if err != nil {
		return err
	}
	for _, member := range changedMembers {
		// 角色变更是高频权限链路依赖字段，提交后立刻 patch 成员缓存可缩短读侧不一致窗口。
		r.upsertGroupMemberCacheAsync(ctx, groupUUID, member)
	}
	return nil
}

// UpdateMemberRole 更新群成员角色。
func (r *groupRepositoryImpl) UpdateMemberRole(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, role int8) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || targetUserUUID == "" {
		return nil
	}
	var changedMember *model.GroupMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 成员角色会影响后续加人、踢人等权限判断，因此写入必须串行化到群维度事务里。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		if group.OwnerUuid != operatorUUID {
			return ErrNoPermission
		}
		operator, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleOwner)
		if err != nil {
			return err
		}
		targetMember, err := r.loadActiveMemberForUpdate(ctx, tx, groupUUID, targetUserUUID)
		if err != nil {
			if errors.Is(err, ErrRecordNotFound) {
				return ErrGroupMemberNotFound
			}
			return err
		}
		if !canUpdateGroupMemberRole(operator.Role, targetMember.Role, role) {
			return ErrNoPermission
		}
		if targetMember.Role == role {
			return nil
		}
		if role == memberRoleAdmin && targetMember.Role != memberRoleAdmin {
			adminCount, err := r.countGroupAdminsForUpdate(ctx, tx, groupUUID)
			if err != nil {
				return err
			}
			if adminCount >= maxGroupAdminCount {
				return ErrAdminLimitExceeded
			}
		}
		updatedAt := time.Now()
		if err := tx.Model(&model.GroupMember{}).
			Where("id = ?", targetMember.Id).
			Updates(map[string]interface{}{
				"role":       role,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Update("updated_at", updatedAt).Error; err != nil {
			return WrapDBError(err)
		}
		updatedMember := *targetMember
		updatedMember.Role = role
		updatedMember.UpdatedAt = updatedAt
		changedMember = &updatedMember
		group.UpdatedAt = updatedAt
		return r.insertMemberRoleUpdatedEvent(tx, group, operatorUUID, []*model.GroupMember{changedMember})
	})
	if err != nil {
		return err
	}
	if changedMember != nil {
		r.upsertGroupMemberCacheAsync(ctx, groupUUID, changedMember)
	}
	return nil
}

// ApplyJoinGroup 按 add_mode 执行直加入群或创建待审批申请。
func (r *groupRepositoryImpl) ApplyJoinGroup(ctx context.Context, groupUUID, applicantUUID, reason string) (ApplyJoinGroupResult, error) {
	result := ApplyJoinGroupResult{}
	if r == nil || r.db == nil || groupUUID == "" || applicantUUID == "" {
		return result, nil
	}
	var changedMember *model.GroupMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 申请入群要和资料变更、审批、直接加人共享同一群维度串行边界。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		existingMap, err := r.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{applicantUUID})
		if err != nil {
			return err
		}
		if isActiveGroupMember(existingMap[applicantUUID]) {
			return ErrAlreadyGroupMember
		}
		if group.AddMode == 0 {
			now := time.Now()
			existing := existingMap[applicantUUID]
			if existing == nil {
				member := &model.GroupMember{
					GroupUuid: groupUUID,
					UserUuid:  applicantUUID,
					Role:      memberRoleMember,
					Status:    memberStatusNormal,
					Inviter:   applicantUUID,
					JoinedAt:  now,
				}
				if err := tx.Create(member).Error; err != nil {
					return WrapDBError(err)
				}
				changedMember = member
			} else {
				if err := tx.Unscoped().Model(&model.GroupMember{}).
					Where("id = ?", existing.Id).
					Updates(map[string]interface{}{
						"status":       memberStatusNormal,
						"role":         memberRoleMember,
						"inviter_uuid": applicantUUID,
						"joined_at":    now,
						"updated_at":   now,
						"deleted_at":   nil,
					}).Error; err != nil {
					return WrapDBError(err)
				}
				restored := *existing
				restored.Status = memberStatusNormal
				restored.Role = memberRoleMember
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
				return WrapDBError(err)
			}
			group.MemberCnt++
			group.UpdatedAt = now
			result.JoinedDirectly = true
			return r.insertMemberAddedEvent(tx, group, applicantUUID, []*model.GroupMember{changedMember})
		}
		pendingRequest, err := r.loadPendingJoinRequestByApplicantForUpdate(ctx, tx, groupUUID, applicantUUID)
		if err != nil {
			return err
		}
		if pendingRequest != nil {
			return ErrGroupApplyAlreadyExists
		}
		joinRequest := &model.GroupJoinRequest{
			GroupUuid:     groupUUID,
			ApplicantUuid: applicantUUID,
			Status:        joinRequestStatusPending,
			Reason:        reason,
		}
		if err := tx.Create(joinRequest).Error; err != nil {
			return WrapDBError(err)
		}
		result.ApplyID = joinRequest.Id
		return r.insertJoinRequestCreatedEvent(tx, groupUUID, applicantUUID, joinRequest)
	})
	if err != nil {
		return ApplyJoinGroupResult{}, err
	}
	if changedMember != nil {
		// 直接入群后立刻 patch 成员缓存，缩短权限判断读到旧成员集的窗口。
		r.upsertGroupMemberCacheAsync(ctx, groupUUID, changedMember)
	}
	return result, nil
}

// CancelJoinGroupApplication 撤销当前用户自己发起的待审批入群申请。
func (r *groupRepositoryImpl) CancelJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) error {
	if r == nil || r.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID); err != nil {
			return err
		}
		joinRequest, err := r.loadPendingJoinRequestByApplicantForUpdate(ctx, tx, groupUUID, applicantUUID)
		if err != nil {
			return err
		}
		if joinRequest == nil {
			return ErrGroupApplyNotFound
		}
		now := time.Now()
		if err := tx.Model(&model.GroupJoinRequest{}).
			Where("id = ?", joinRequest.Id).
			Updates(map[string]interface{}{
				"status":     joinRequestStatusCanceled,
				"updated_at": now,
			}).Error; err != nil {
			return WrapDBError(err)
		}
		joinRequest.Status = joinRequestStatusCanceled
		joinRequest.UpdatedAt = now
		return r.insertJoinRequestCanceledEvent(tx, groupUUID, applicantUUID, joinRequest)
	})
}

// GetMyJoinGroupApplication 获取当前用户在指定群的最新入群申请状态。
func (r *groupRepositoryImpl) GetMyJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if r == nil || r.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return nil, err
	}
	return r.loadLatestJoinRequestByApplicant(ctx, groupUUID, applicantUUID)
}

// ReviewJoinGroup 审批入群申请。
func (r *groupRepositoryImpl) ReviewJoinGroup(ctx context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || applyID <= 0 {
		return nil
	}
	var changedMember *model.GroupMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 入群审批属于管理动作，允许管理员及以上执行。
		if _, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin); err != nil {
			return err
		}
		joinRequest, err := r.loadPendingJoinRequestForUpdate(ctx, tx, groupUUID, applyID)
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
		if action == joinRequestStatusRejected {
			if err := tx.Model(&model.GroupJoinRequest{}).
				Where("id = ?", joinRequest.Id).
				Updates(requestUpdates).Error; err != nil {
				return WrapDBError(err)
			}
			return r.insertJoinRequestReviewedEvent(tx, groupUUID, operatorUUID, joinRequest)
		}
		existingMap, err := r.loadExistingMembersForUpdate(ctx, tx, groupUUID, []string{joinRequest.ApplicantUuid})
		if err != nil {
			return err
		}
		existing := existingMap[joinRequest.ApplicantUuid]
		if !isActiveGroupMember(existing) {
			if existing == nil {
				member := &model.GroupMember{
					GroupUuid: groupUUID,
					UserUuid:  joinRequest.ApplicantUuid,
					Role:      memberRoleMember,
					Status:    memberStatusNormal,
					Inviter:   operatorUUID,
					JoinedAt:  now,
				}
				if err := tx.Create(member).Error; err != nil {
					return WrapDBError(err)
				}
				changedMember = member
			} else {
				if err := tx.Unscoped().Model(&model.GroupMember{}).
					Where("id = ?", existing.Id).
					Updates(map[string]interface{}{
						"status":       memberStatusNormal,
						"role":         memberRoleMember,
						"inviter_uuid": operatorUUID,
						"joined_at":    now,
						"updated_at":   now,
						"deleted_at":   nil,
					}).Error; err != nil {
					return WrapDBError(err)
				}
				restored := *existing
				restored.Status = memberStatusNormal
				restored.Role = memberRoleMember
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
				return WrapDBError(err)
			}
			group.MemberCnt++
			group.UpdatedAt = now
		}
		if err := tx.Model(&model.GroupJoinRequest{}).
			Where("id = ?", joinRequest.Id).
			Updates(requestUpdates).Error; err != nil {
			return WrapDBError(err)
		}
		if err := r.insertJoinRequestReviewedEvent(tx, groupUUID, operatorUUID, joinRequest); err != nil {
			return err
		}
		if changedMember == nil {
			return nil
		}
		return r.insertMemberAddedEvent(tx, group, operatorUUID, []*model.GroupMember{changedMember})
	})
	if err != nil {
		return err
	}
	if changedMember != nil {
		r.upsertGroupMemberCacheAsync(ctx, groupUUID, changedMember)
	}
	return nil
}

// ListJoinRequests 获取群待审批入群申请列表。
func (r *groupRepositoryImpl) ListJoinRequests(ctx context.Context, groupUUID, operatorUUID string, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return nil, 0, err
	}
	var operator model.GroupMember
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
	cachedItems, cacheHit, cacheErr := r.getGroupJoinRequestsFromCache(ctx, groupUUID)
	if cacheErr != nil {
		LogRedisError(ctx, cacheErr)
	} else if cacheHit {
		total := int64(len(cachedItems))
		if total == 0 {
			return []*model.GroupJoinRequest{}, 0, nil
		}
		start := (page - 1) * pageSize
		if start >= len(cachedItems) {
			return []*model.GroupJoinRequest{}, total, nil
		}
		end := start + pageSize
		if end > len(cachedItems) {
			end = len(cachedItems)
		}
		return cachedItems[start:end], total, nil
	}
	query := r.db.WithContext(ctx).
		Model(&model.GroupJoinRequest{}).
		Where("group_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, joinRequestStatusPending)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	if total == 0 {
		if rebuildErr := r.rebuildGroupJoinRequestsCache(ctx, groupUUID, []*model.GroupJoinRequest{}); rebuildErr != nil {
			LogRedisError(ctx, rebuildErr)
		}
		return []*model.GroupJoinRequest{}, 0, nil
	}
	items := make([]*model.GroupJoinRequest, 0, total)
	if err := query.Order("created_at DESC, id DESC").Find(&items).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	if rebuildErr := r.rebuildGroupJoinRequestsCache(ctx, groupUUID, items); rebuildErr != nil {
		LogRedisError(ctx, rebuildErr)
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []*model.GroupJoinRequest{}, total, nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}

// buildGroupInfoUpdateMap 只提取真实变化字段，避免无意义更新打乱排序。
func buildGroupInfoUpdateMap(group *model.GroupInfo, updates GroupInfoUpdates) map[string]interface{} {
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
func applyGroupInfoUpdates(group *model.GroupInfo, updates GroupInfoUpdates) {
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

// countGroupAdminsForUpdate 在事务内统计当前群的有效管理员数量。
//
// 这里不把群主算进管理员上限，只约束 role=1 的管理员席位，
// 避免群主转让、群主唯一性与管理员配额耦在一起。
func (r *groupRepositoryImpl) countGroupAdminsForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string) (int64, error) {
	if tx == nil || groupUUID == "" {
		return 0, nil
	}
	var count int64
	err := tx.WithContext(ctx).
		Model(&model.GroupMember{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_uuid = ? AND role = ? AND status = ? AND deleted_at IS NULL", groupUUID, memberRoleAdmin, memberStatusNormal).
		Count(&count).Error
	if err != nil {
		return 0, WrapDBError(err)
	}
	return count, nil
}

// loadPendingJoinRequestForUpdate 以行锁方式加载单条待审批入群申请。
func (r *groupRepositoryImpl) loadPendingJoinRequestForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string, applyID int64) (*model.GroupJoinRequest, error) {
	var joinRequest model.GroupJoinRequest
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND group_uuid = ? AND status = ? AND deleted_at IS NULL", applyID, groupUUID, joinRequestStatusPending).
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupApplyNotFound
		}
		return nil, WrapDBError(err)
	}
	return &joinRequest, nil
}

// loadPendingJoinRequestByApplicantForUpdate 以行锁方式检查申请人是否已有待处理申请。
func (r *groupRepositoryImpl) loadPendingJoinRequestByApplicantForUpdate(ctx context.Context, tx *gorm.DB, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if tx == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	var joinRequest model.GroupJoinRequest
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_uuid = ? AND applicant_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, applicantUUID, joinRequestStatusPending).
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

// actionToJoinRequestStatus 把审批动作值映射成申请状态值。
func actionToJoinRequestStatus(action int8) int8 {
	if action == joinRequestStatusRejected {
		return joinRequestStatusRejected
	}
	return joinRequestStatusApproved
}

// getGroupJoinRequestsFromCache 读取群待审批入群申请缓存。
func (r *groupRepositoryImpl) getGroupJoinRequestsFromCache(ctx context.Context, groupUUID string) ([]*model.GroupJoinRequest, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.GroupJoinRequestPendingKey(groupUUID)
	values, err := r.redisClient.HGetAll(ctx, cacheKey).Result()
	if err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		return nil, false, WrapRedisError(err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}
	if len(values) == 1 {
		if raw, ok := values[groupJoinRequestsEmptyField]; ok && raw == groupJoinRequestsEmptyValue {
			return []*model.GroupJoinRequest{}, true, nil
		}
	}
	items := make([]*model.GroupJoinRequest, 0, len(values))
	for field, raw := range values {
		if field == groupJoinRequestsEmptyField {
			continue
		}
		entry, decodeErr := decodeGroupJoinRequestCacheValue(raw)
		if decodeErr != nil {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		if item := buildGroupJoinRequestFromCache(entry); item != nil {
			items = append(items, item)
		}
	}
	sortGroupJoinRequests(items)
	if getRandomBool(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupJoinRequestTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			LogRedisError(ctx, expireErr)
		}
	}
	return items, true, nil
}

// rebuildGroupJoinRequestsCache 全量重建群待审批入群申请缓存。
func (r *groupRepositoryImpl) rebuildGroupJoinRequestsCache(ctx context.Context, groupUUID string, items []*model.GroupJoinRequest) error {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil
	}
	cacheKey := rediskey.GroupJoinRequestPendingKey(groupUUID)
	fields := make(map[string]string, len(items))
	for _, item := range items {
		if item == nil || item.Id <= 0 {
			continue
		}
		fields[fmt.Sprintf("%d", item.Id)] = encodeGroupJoinRequestCacheValue(item)
	}
	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, cacheKey)
	if len(fields) == 0 {
		pipe.HSet(ctx, cacheKey, map[string]string{groupJoinRequestsEmptyField: groupJoinRequestsEmptyValue})
		pipe.Expire(ctx, cacheKey, rediskey.GroupJoinRequestEmptyTTL)
	} else {
		pipe.HSet(ctx, cacheKey, fields)
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupJoinRequestTTL))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// upsertGroupJoinRequestCacheIfExists 在待审批申请缓存已存在时同步写入单条申请。
func (r *groupRepositoryImpl) upsertGroupJoinRequestCacheIfExists(ctx context.Context, groupUUID string, request *model.GroupJoinRequest) error {
	if r == nil || r.redisClient == nil || groupUUID == "" || request == nil || request.Id <= 0 {
		return nil
	}
	cacheKey := rediskey.GroupJoinRequestPendingKey(groupUUID)
	luaScript := goredis.NewScript(luaUpsertGroupMemberIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.GroupJoinRequestTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		fmt.Sprintf("%d", request.Id),
		encodeGroupJoinRequestCacheValue(request),
		fmt.Sprintf("%d", expireSeconds),
	).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// removeGroupJoinRequestCacheIfExists 在待审批申请缓存已存在时删除单条申请。
func (r *groupRepositoryImpl) removeGroupJoinRequestCacheIfExists(ctx context.Context, groupUUID string, applyID int64) error {
	if r == nil || r.redisClient == nil || groupUUID == "" || applyID <= 0 {
		return nil
	}
	cacheKey := rediskey.GroupJoinRequestPendingKey(groupUUID)
	luaScript := goredis.NewScript(luaRemoveGroupMemberIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.GroupJoinRequestTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		fmt.Sprintf("%d", applyID),
		groupJoinRequestsEmptyValue,
		fmt.Sprintf("%d", expireSeconds),
	).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// loadGroupForUpdate 以行锁方式加载群记录。
//
// 该方法是所有群写操作的并发边界，保证同一群的多类写事实在事务内串行化裁决。
func (r *groupRepositoryImpl) loadGroupForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string) (*model.GroupInfo, error) {
	var group model.GroupInfo
	// 写路径统一通过行锁读取群记录，作为成员变更、解散和资料更新的并发边界。
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("uuid = ? AND deleted_at IS NULL", groupUUID).
		First(&group).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &group, nil
}

// loadWritableGroupForUpdate 加载可写的正常群记录。
//
// 这里把“已解散”和“其他异常状态”拆成不同错误，便于 service 映射稳定业务码。
func (r *groupRepositoryImpl) loadWritableGroupForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string) (*model.GroupInfo, error) {
	group, err := r.loadGroupForUpdate(ctx, tx, groupUUID)
	if err != nil {
		return nil, err
	}
	// 解散态单独返回，service 需要映射到明确的“群已解散”业务码。
	if group.Status == groupStatusDismissed {
		return nil, ErrGroupDismissed
	}
	// 非正常态不暴露内部状态细节，上层统一按群不存在处理。
	if group.Status != groupStatusNormal {
		return nil, ErrRecordNotFound
	}
	return group, nil
}

// ensureGroupNormal 确认群当前仍处于可读的正常状态。
//
// 高频读链路优先查缓存中的群状态，再按需回源数据库，避免每次权限校验都落到 MySQL。
func (r *groupRepositoryImpl) ensureGroupNormal(ctx context.Context, groupUUID string) error {
	if r == nil || r.db == nil || groupUUID == "" {
		return ErrRecordNotFound
	}
	groupInfo, cacheHit, err := r.getGroupInfoFromCache(ctx, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		if groupInfo == nil {
			return ErrRecordNotFound
		}
		if groupInfo.Status == groupStatusDismissed {
			return ErrGroupDismissed
		}
		if groupInfo.Status != groupStatusNormal {
			return ErrRecordNotFound
		}
		return nil
	}
	var group model.GroupInfo
	// 高频读路径只需要 status，避免为了权限检查读取整行群资料。
	err = r.db.WithContext(ctx).
		Select("status").
		Where("uuid = ? AND deleted_at IS NULL", groupUUID).
		Take(&group).Error
	if err != nil {
		return WrapDBError(err)
	}
	if group.Status == groupStatusDismissed {
		return ErrGroupDismissed
	}
	if group.Status != groupStatusNormal {
		return ErrRecordNotFound
	}
	return nil
}

// loadActiveMemberForUpdate 以行锁方式加载单个有效成员关系。
func (r *groupRepositoryImpl) loadActiveMemberForUpdate(ctx context.Context, tx *gorm.DB, groupUUID, userUUID string) (*model.GroupMember, error) {
	var member model.GroupMember
	// 成员角色是权限判断依据，必须加锁读取，防止并发角色变更造成越权。
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_uuid = ? AND user_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, userUUID, memberStatusNormal).
		Take(&member).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &member, nil
}

// ensureOperatorRoleAtLeast 校验操作者是否达到指定最小角色等级。
func (r *groupRepositoryImpl) ensureOperatorRoleAtLeast(ctx context.Context, tx *gorm.DB, groupUUID, operatorUUID string, minRole int8) (*model.GroupMember, error) {
	member, err := r.loadActiveMemberForUpdate(ctx, tx, groupUUID, operatorUUID)
	if err != nil {
		// 操作者不是有效成员时，对外统一表现为无权限，而不是泄露成员状态细节。
		if errors.Is(err, ErrRecordNotFound) {
			return nil, ErrNoPermission
		}
		return nil, err
	}
	if member.Role < minRole {
		return nil, ErrNoPermission
	}
	return member, nil
}

// ensureOperatorCanAddMembers 校验操作者是否具备加人权限。
func (r *groupRepositoryImpl) ensureOperatorCanAddMembers(ctx context.Context, tx *gorm.DB, groupUUID, operatorUUID string) error {
	_, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin)
	return err
}

// isActiveGroupMember 判断成员关系是否仍为有效成员。
func isActiveGroupMember(member *model.GroupMember) bool {
	return member != nil && member.Status == memberStatusNormal && !member.DeletedAt.Valid
}

// canRemoveGroupMember 判断当前角色矩阵下是否允许移除目标成员。
func canRemoveGroupMember(operatorRole, targetRole int8) bool {
	switch operatorRole {
	case memberRoleOwner:
		return targetRole != memberRoleOwner
	case memberRoleAdmin:
		return targetRole == memberRoleMember
	default:
		return false
	}
}

// canUpdateGroupMemberRole 判断是否允许把目标成员调整为指定角色。
//
// 当前规则保持最小闭环：
//  1. 只有群主可以设置管理员；
//  2. 目标必须是当前有效的普通成员或管理员；
//  3. 不能通过该接口直接产生第二个群主。
//
// canUpdateGroupMemberRole 判断是否允许把目标成员调整为指定角色。
//
// 当前规则保持最小闭环：
//  1. 只有群主可以设置管理员；
//  2. 目标必须是当前有效的普通成员或管理员；
//  3. 不能通过该接口直接产生第二个群主。
func canUpdateGroupMemberRole(operatorRole, currentTargetRole, nextTargetRole int8) bool {
	if operatorRole != memberRoleOwner {
		return false
	}
	if currentTargetRole != memberRoleMember && currentTargetRole != memberRoleAdmin {
		return false
	}
	return nextTargetRole == memberRoleMember || nextTargetRole == memberRoleAdmin
}

// loadExistingMembersForUpdate 在事务内锁定目标成员集合。
//
// 这里使用 Unscoped 的原因是：
//  1. 重新入群需要感知历史软删成员；
//  2. 转让群主、角色变更、踢人等写操作都要和历史记录串行化；
//  3. 可以避免并发场景下重复插入同一个成员关系。
//
// loadExistingMembersForUpdate 在事务内锁定目标成员集合。
//
// 这里使用 Unscoped 的原因是：
//  1. 重新入群需要感知历史软删成员；
//  2. 转让群主、角色变更、踢人等写操作都要和历史记录串行化；
//  3. 可以避免并发场景下重复插入同一个成员关系。
func (r *groupRepositoryImpl) loadExistingMembersForUpdate(ctx context.Context, tx *gorm.DB, groupUUID string, userUUIDs []string) (map[string]*model.GroupMember, error) {
	result := make(map[string]*model.GroupMember, len(userUUIDs))
	if len(userUUIDs) == 0 {
		return result, nil
	}
	var members []*model.GroupMember
	// Unscoped 是为了把软删历史也锁住，支持重新入群恢复，并避免并发插入同一成员。
	if err := tx.WithContext(ctx).
		Unscoped().
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_uuid = ? AND user_uuid IN ?", groupUUID, userUUIDs).
		Find(&members).Error; err != nil {
		return nil, WrapDBError(err)
	}
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		result[member.UserUuid] = member
	}
	return result, nil
}
func (r *groupRepositoryImpl) rebuildGroupMembersCacheAsync(ctx context.Context, groupUUID string, members []*model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}
	// 异步任务使用克隆数据，避免调用方后续复用 slice 时影响缓存写入内容。
	cloned := cloneGroupMembers(members)
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.rebuildGroupMembersCache(runCtx, groupUUID, cloned); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisPipelineTimeout)
}

// getGroupInfoFromCache 读取 group:info:{group_uuid} 主缓存。
//
// 返回值语义：
//  1. (*GroupInfo, true, nil)：命中有效群资料缓存；
//  2. (nil, true, nil)：命中空值缓存，表示当前群不存在；
//  3. (nil, false, nil)：缓存未命中或脏 key 已被清理，调用方应回源 DB；
//  4. (_, _, err)：Redis 出现可观测错误，调用方通常记录告警后降级。
func (r *groupRepositoryImpl) getGroupInfoFromCache(ctx context.Context, groupUUID string) (*model.GroupInfo, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.GroupInfoKey(groupUUID)
	raw, err := r.redisClient.Get(ctx, cacheKey).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, false, nil
		}
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		return nil, false, WrapRedisError(err)
	}
	if raw == groupInfoEmptyValue {
		return nil, true, nil
	}
	entry, decodeErr := decodeGroupInfoCacheValue(raw)
	if decodeErr != nil {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return nil, false, nil
	}
	if getRandomBool(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupInfoTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			LogRedisError(ctx, expireErr)
		}
	}
	return buildGroupInfoFromCache(entry), true, nil
}

// setGroupInfoCacheAsync 异步回填群资料主缓存。
//
// 这里使用值拷贝而不是直接传原对象，避免调用方后续继续修改 group 指针时，
// 异步任务把“半旧半新”的内容写入 Redis。
func (r *groupRepositoryImpl) setGroupInfoCacheAsync(ctx context.Context, group *model.GroupInfo) {
	if r == nil || r.redisClient == nil || group == nil || group.Uuid == "" {
		return
	}
	copyGroup := *group
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.setGroupInfoCache(runCtx, &copyGroup); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// setGroupInfoCache 直接覆盖群资料主缓存。
//
// 该方法用于：
//  1. 读路径回源 DB 后的完整回填；
//  2. group_created projector 首次初始化主缓存。
//
// 对于增量写事实则应优先使用 patch-if-exists，避免写路径承担全量建缓存职责。
func (r *groupRepositoryImpl) setGroupInfoCache(ctx context.Context, group *model.GroupInfo) error {
	if r == nil || r.redisClient == nil || group == nil || group.Uuid == "" {
		return nil
	}
	cacheKey := rediskey.GroupInfoKey(group.Uuid)
	if err := r.redisClient.Set(ctx, cacheKey, encodeGroupInfoCacheValue(group), getRandomExpireTime(rediskey.GroupInfoTTL)).Err(); err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// setGroupInfoEmptyCacheAsync 异步写入群资料空值缓存。
//
// 这样“查无此群”的结果也会被短 TTL 缓存，
// 可以显著降低恶意探测和热点 miss 对数据库的压力。
func (r *groupRepositoryImpl) setGroupInfoEmptyCacheAsync(ctx context.Context, groupUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		cacheKey := rediskey.GroupInfoKey(groupUUID)
		if err := r.redisClient.Set(runCtx, cacheKey, groupInfoEmptyValue, rediskey.GroupInfoEmptyTTL).Err(); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// getUserGroupsFromCache 读取用户群列表反向索引缓存。
//
// 这里采用“两级缓存拼装”策略：
//  1. 先从 ZSet 取出 group_uuid 列表；
//  2. 再逐个读取 group:info 主缓存；
//  3. 只要任一主缓存缺失，就整体视为未命中并回源 DB，避免返回不完整列表。
func (r *groupRepositoryImpl) getUserGroupsFromCache(ctx context.Context, userUUID string) ([]*model.GroupInfo, bool, error) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.UserGroupListKey(userUUID)
	values, err := r.redisClient.ZRevRange(ctx, cacheKey, 0, -1).Result()
	if err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		return nil, false, WrapRedisError(err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}
	if len(values) == 1 && values[0] == userGroupsEmptyValue {
		return []*model.GroupInfo{}, true, nil
	}
	groups := make([]*model.GroupInfo, 0, len(values))
	missing := false
	for _, groupUUID := range values {
		if groupUUID == "" || groupUUID == userGroupsEmptyValue {
			continue
		}
		group, cacheHit, cacheErr := r.getGroupInfoFromCache(ctx, groupUUID)
		if cacheErr != nil {
			return nil, false, cacheErr
		}
		if !cacheHit || group == nil || group.Status != groupStatusNormal {
			missing = true
			break
		}
		groups = append(groups, group)
	}
	if missing {
		return nil, false, nil
	}
	if getRandomBool(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.UserGroupListTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			LogRedisError(ctx, expireErr)
		}
	}
	return groups, true, nil
}

// rebuildUserGroupsCacheAsync 异步全量重建单个用户的群列表缓存。
//
// 该方法只在读路径 miss 后调用；
// 写路径和 projector 仍然遵循 patch-if-exists，避免缓存重建职责向写侧扩散。
func (r *groupRepositoryImpl) rebuildUserGroupsCacheAsync(ctx context.Context, userUUID string, groups []*model.GroupInfo) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return
	}
	cloned := cloneGroupInfos(groups)
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.rebuildUserGroupsCache(runCtx, userUUID, cloned); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisPipelineTimeout)
}

// rebuildUserGroupsCache 全量重建用户群列表 ZSet。
//
// 这里直接删除旧 key 再写入新集合，是因为读路径已经拿到了完整事实集，
// 此时全量重建比增量 patch 更简单，也更不容易把历史脏 member 留在缓存中。
func (r *groupRepositoryImpl) rebuildUserGroupsCache(ctx context.Context, userUUID string, groups []*model.GroupInfo) error {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return nil
	}
	cacheKey := rediskey.UserGroupListKey(userUUID)
	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, cacheKey)
	if len(groups) == 0 {
		pipe.ZAdd(ctx, cacheKey, goredis.Z{Score: 0, Member: userGroupsEmptyValue})
		pipe.Expire(ctx, cacheKey, rediskey.UserGroupListEmptyTTL)
	} else {
		items := make([]goredis.Z, 0, len(groups))
		for _, group := range groups {
			if group == nil || group.Uuid == "" {
				continue
			}
			items = append(items, goredis.Z{Score: float64(group.UpdatedAt.Unix()), Member: group.Uuid})
		}
		if len(items) > 0 {
			pipe.ZAdd(ctx, cacheKey, items...)
		}
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.UserGroupListTTL))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}
func collectWriteMemberUUIDs(members []*model.GroupMember) []string {
	if len(members) == 0 {
		return []string{}
	}
	userUUIDs := make([]string, 0, len(members))
	// 事务内只锁每个目标用户一次，减少 IN 条件大小和重复行锁竞争。
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		if _, ok := seen[member.UserUuid]; ok {
			continue
		}
		seen[member.UserUuid] = struct{}{}
		userUUIDs = append(userUUIDs, member.UserUuid)
	}
	return userUUIDs
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
	groupInfo, cacheHit, err := r.getGroupInfoFromCache(ctx, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		if groupInfo == nil || groupInfo.Status != groupStatusNormal {
			return nil, ErrRecordNotFound
		}
		return groupInfo, nil
	}
	groupInfo, err = r.loadGroupInfoFromDB(ctx, groupUUID)
	if err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			r.setGroupInfoEmptyCacheAsync(ctx, groupUUID)
		}
		return nil, err
	}
	r.setGroupInfoCacheAsync(ctx, groupInfo)
	return groupInfo, nil
}

// loadGroupInfoFromDB 从 MySQL 读取群资料权威事实。
//
// 这里只返回“有效群”，因此 status!=0、已软删、已解散都统一视为不存在，
// 上层不需要了解 groups 表的内部状态枚举细节。
func (r *groupRepositoryImpl) loadGroupInfoFromDB(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
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
	members, cacheHit, err := r.getGroupMembersFromCache(ctx, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		return members, nil
	}
	return r.fetchGroupMembersWithSingleflight(ctx, groupUUID)
}
func (r *groupRepositoryImpl) loadGroupMembersFromDB(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if _, err := r.GetGroupInfo(ctx, groupUUID); err != nil {
		return nil, err
	}
	var members []*model.GroupMember
	if err := r.db.WithContext(ctx).
		Table("group_members AS gm").
		Select("gm.*").
		// groups 是 MySQL 8 保留关键字，手写 JOIN 时必须显式转义，避免线上 SQL 语法错误。
		Joins("JOIN `groups` AS g ON g.uuid = gm.group_uuid").
		Where("gm.group_uuid = ? AND gm.status = 0 AND gm.deleted_at IS NULL", groupUUID).
		Where("g.status = 0 AND g.deleted_at IS NULL").
		Order("gm.role DESC, gm.joined_at ASC, gm.id ASC").
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("查询群成员失败: %w", WrapDBError(err))
	}
	return members, nil
}
func (r *groupRepositoryImpl) fetchGroupMembersWithSingleflight(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	value, err, _ := r.memberGroup.Do("group_members:"+groupUUID, func() (interface{}, error) {
		members, loadErr := r.loadGroupMembersFromDB(ctx, groupUUID)
		if loadErr != nil {
			return nil, loadErr
		}
		// singleflight 合并后的首个回源协程负责尝试回填缓存；
		// 即便 Redis 回填失败，也不能影响本次已经查到的 MySQL 结果返回。
		if rebuildErr := r.rebuildGroupMembersCache(ctx, groupUUID, members); rebuildErr != nil {
			LogRedisError(ctx, rebuildErr)
		}
		return cloneGroupMembers(members), nil
	})
	if err != nil {
		return nil, err
	}
	members, ok := value.([]*model.GroupMember)
	if !ok {
		return nil, fmt.Errorf("群成员 singleflight 返回类型错误")
	}
	return cloneGroupMembers(members), nil
}
func (r *groupRepositoryImpl) getGroupMembersFromCache(ctx context.Context, groupUUID string) ([]*model.GroupMember, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	values, err := r.redisClient.HGetAll(ctx, cacheKey).Result()
	if err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		return nil, false, WrapRedisError(err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}
	members := make([]*model.GroupMember, 0, len(values))
	for userUUID, raw := range values {
		entry, decodeErr := decodeGroupMemberCacheValue(raw)
		if decodeErr != nil {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		if member := buildGroupMemberFromCache(userUUID, entry); member != nil {
			members = append(members, member)
		}
	}
	sortGroupMembers(members)
	if getRandomBool(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			LogRedisError(ctx, expireErr)
		}
	}
	return members, true, nil
}

// rebuildGroupMembersCache 全量重建群成员 Hash 缓存。
//
// 该方法主要服务于读路径 miss 和 group_created 首次建缓存：
//  1. 输入已经是一份完整成员事实集；
//  2. 因此直接 Del + HSet 比逐个 patch 更可靠；
//  3. 返回 error 让调用方自行决定是降级记录还是重试。
func (r *groupRepositoryImpl) rebuildGroupMembersCache(ctx context.Context, groupUUID string, members []*model.GroupMember) error {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	fields := make(map[string]string, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		fields[member.UserUuid] = encodeGroupMemberCacheValue(member)
	}
	if len(fields) == 0 {
		return r.deleteGroupMembersCache(ctx, groupUUID)
	}
	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, cacheKey)
	pipe.HSet(ctx, cacheKey, fields)
	pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL))
	if _, err := pipe.Exec(ctx); err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// insertGroupMemberCacheIfExists 在群成员 Hash 已存在时同步插入新成员 field。
//
// 该方法严格遵守 patch-if-exists：
//  1. key 存在才插入；
//  2. key 不存在直接跳过，留给读路径全量重建；
//  3. field 已存在时不覆盖，保证“新增成员”语义稳定。
func (r *groupRepositoryImpl) insertGroupMemberCacheIfExists(ctx context.Context, groupUUID string, member *model.GroupMember) error {
	if r == nil || r.redisClient == nil || groupUUID == "" || member == nil || member.UserUuid == "" {
		return nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	luaScript := goredis.NewScript(luaInsertGroupMemberIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		member.UserUuid,
		encodeGroupMemberCacheValue(member),
		expireSeconds,
	).Result()
	if err != nil && err != goredis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// upsertGroupMemberCacheIfExists 在群成员 Hash 已存在时同步覆盖单个成员 field。
//
// 该方法适用于恢复入群、角色变更等需要覆盖旧快照的场景。
func (r *groupRepositoryImpl) upsertGroupMemberCacheIfExists(ctx context.Context, groupUUID string, member *model.GroupMember) error {
	if r == nil || r.redisClient == nil || groupUUID == "" || member == nil || member.UserUuid == "" {
		return nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	luaScript := goredis.NewScript(luaUpsertGroupMemberIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		member.UserUuid,
		encodeGroupMemberCacheValue(member),
		expireSeconds,
	).Result()
	if err != nil && err != goredis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// removeGroupMemberCacheIfExists 在群成员 Hash 已存在时同步删除单个成员 field。
//
// 若删除后集合为空，Lua 脚本会自动写回空集合占位，避免后续重复穿透 DB。
func (r *groupRepositoryImpl) removeGroupMemberCacheIfExists(ctx context.Context, groupUUID, userUUID string) error {
	if r == nil || r.redisClient == nil || groupUUID == "" || userUUID == "" {
		return nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	luaScript := goredis.NewScript(luaRemoveGroupMemberIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		userUUID,
		groupMembersEmptyValue,
		expireSeconds,
	).Result()
	if err != nil && err != goredis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// deleteGroupMembersCache 直接删除整份群成员缓存。
//
// 该方法用于 group_dismissed 等强失效场景，
// 此时“整 key 删除”比逐个成员 patch 更安全也更便宜。
func (r *groupRepositoryImpl) deleteGroupMembersCache(ctx context.Context, groupUUID string) error {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	if err := r.redisClient.Del(ctx, cacheKey).Err(); err != nil && err != goredis.Nil {
		return WrapRedisError(err)
	}
	return nil
}

// insertGroupMembersCacheAsync 在群成员 Hash 已存在时，异步增量插入新成员 field。
//
// 该方法供后续 CreateGroup / AddMember 写路径复用；如果 key 已过期，则交给下一次读路径
// 统一全量重建，避免把局部成员集合误写成完整事实。
func (r *groupRepositoryImpl) insertGroupMembersCacheAsync(ctx context.Context, groupUUID string, members []*model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" || len(members) == 0 {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		seen := make(map[string]struct{}, len(members))
		for _, member := range members {
			if member == nil || member.UserUuid == "" {
				continue
			}
			if _, ok := seen[member.UserUuid]; ok {
				continue
			}
			seen[member.UserUuid] = struct{}{}
			if err := r.insertGroupMemberCacheIfExists(runCtx, groupUUID, member); err != nil {
				LogRedisError(runCtx, err)
			}
		}
	}, async.AsyncRedisTimeout)
}

// upsertGroupMemberCacheAsync 在群成员 Hash 已存在时，异步更新单个成员 field。
//
// 该方法供后续角色调整、重新入群等写路径复用；若缓存整体缺失，则继续让读路径全量重建。
func (r *groupRepositoryImpl) upsertGroupMemberCacheAsync(ctx context.Context, groupUUID string, member *model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" || member == nil || member.UserUuid == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.upsertGroupMemberCacheIfExists(runCtx, groupUUID, member); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// removeGroupMemberCacheAsync 在群成员 Hash 已存在时，异步删除单个成员 field。
//
// 若移除后集合为空，则脚本会补一个 __EMPTY__ 占位，避免极端空集合场景持续穿透数据库。
func (r *groupRepositoryImpl) removeGroupMemberCacheAsync(ctx context.Context, groupUUID, userUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" || userUUID == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.removeGroupMemberCacheIfExists(runCtx, groupUUID, userUUID); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// deleteGroupMembersCacheAsync 异步删除整份群成员缓存。
//
// 该方法供后续 DismissGroup 等强失效写路径复用，此类场景直接删 key 比逐个 patch 更安全。
func (r *groupRepositoryImpl) deleteGroupMembersCacheAsync(ctx context.Context, groupUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.deleteGroupMembersCache(runCtx, groupUUID); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// CheckGroupMember 检查指定用户是否仍是群内有效成员，并返回角色。
//
// 这里先确认群状态，再走成员缓存/DB fallback，
// 避免群已解散后仅凭 group_members 残留缓存读出“幽灵成员”结论。
func (r *groupRepositoryImpl) CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error) {
	if r == nil || r.db == nil || groupUUID == "" || userUUID == "" {
		return false, -1, ErrRecordNotFound
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return false, -1, err
	}
	cacheHit, isMember, role, err := r.checkGroupMemberFromCache(ctx, groupUUID, userUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		return isMember, role, nil
	}
	members, err := r.fetchGroupMembersWithSingleflight(ctx, groupUUID)
	if err != nil {
		return false, -1, err
	}
	for _, member := range members {
		if member != nil && member.UserUuid == userUUID {
			return true, member.Role, nil
		}
	}
	return false, -1, nil
}
func (r *groupRepositoryImpl) checkGroupMemberFromCache(ctx context.Context, groupUUID, userUUID string) (bool, bool, int8, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" || userUUID == "" {
		return false, false, -1, nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	memberCmd := pipe.HGet(ctx, cacheKey, userUUID)
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL))
	}
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return false, false, -1, nil
		}
		return false, false, -1, WrapRedisError(err)
	}
	if existsCmd.Val() == 0 {
		return false, false, -1, nil
	}
	if memberCmd.Err() == nil {
		entry, decodeErr := decodeGroupMemberCacheValue(memberCmd.Val())
		if decodeErr != nil {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return false, false, -1, nil
		}
		return true, true, entry.Role, nil
	}
	if errors.Is(memberCmd.Err(), goredis.Nil) {
		return true, false, -1, nil
	}
	if isRedisWrongType(memberCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, false, -1, nil
	}
	return false, false, -1, WrapRedisError(memberCmd.Err())
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
	groups, cacheHit, err := r.getUserGroupsFromCache(ctx, userUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		return groups, nil
	}
	groups, err = r.loadUserGroupsFromDB(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	r.rebuildUserGroupsCacheAsync(ctx, userUUID, groups)
	for _, group := range groups {
		r.setGroupInfoCacheAsync(ctx, group)
	}
	return groups, nil
}

// loadUserGroupsFromDB 从 MySQL 读取用户当前所在的有效群列表。
//
// 查询里同时约束 group_members 和 groups 两张表状态，
// 避免“成员记录仍在，但群已解散/删除”的脏关系被返回给上层。
func (r *groupRepositoryImpl) loadUserGroupsFromDB(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	var groups []*model.GroupInfo
	if err := r.db.WithContext(ctx).
		// groups 是 MySQL 8 保留关键字，作为主表名拼接 SQL 时同样需要显式转义。
		Table("`groups` AS g").
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
