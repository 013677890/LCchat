package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	groupStatusNormal    int8 = 0
	groupStatusDismissed int8 = 2

	memberRoleMember int8 = 0
	memberRoleAdmin  int8 = 1
	memberRoleOwner  int8 = 2

	memberStatusNormal int8 = 0
	memberStatusQuit   int8 = 1
	memberStatusKicked int8 = 2
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
		return nil
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
		return nil
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
		removed = true
		return nil
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

		// 解散只更新群状态；成员历史保留，读写路径统一通过群状态拦截。
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(map[string]interface{}{
				"status":     groupStatusDismissed,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return WrapDBError(err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 解散后成员缓存不能继续支撑权限判断，直接删除整份缓存比逐个 patch 更安全。
	r.deleteGroupMembersCacheAsync(ctx, groupUUID)
	return nil
}

// UpdateGroupInfo 更新群资料。
func (r *groupRepositoryImpl) UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, name, avatar *string) error {
	if r == nil || r.db == nil || groupUUID == "" || operatorUUID == "" || (name == nil && avatar == nil) {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁群记录保证资料更新不会和解散/禁用状态变更交叉提交。
		group, err := r.loadWritableGroupForUpdate(ctx, tx, groupUUID)
		if err != nil {
			return err
		}
		// 群资料可由管理员及群主维护，角色必须在事务内读取最新有效成员关系。
		if _, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin); err != nil {
			return err
		}

		updates := make(map[string]interface{}, 3)
		if name != nil && *name != group.Name {
			// 只写真实变化字段，避免无意义更新导致群列表排序抖动。
			updates["name"] = *name
		}
		if avatar != nil && *avatar != group.Avatar {
			updates["avatar"] = *avatar
		}
		if len(updates) == 0 {
			// 无实际变化时不刷新 updated_at，保持读侧排序稳定。
			return nil
		}

		updates["updated_at"] = time.Now()
		if err := tx.Model(&model.GroupInfo{}).
			Where("id = ?", group.Id).
			Updates(updates).Error; err != nil {
			return WrapDBError(err)
		}
		return nil
	})
}

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

func (r *groupRepositoryImpl) ensureGroupNormal(ctx context.Context, groupUUID string) error {
	var group model.GroupInfo
	// 高频读路径只需要 status，避免为了权限检查读取整行群资料。
	err := r.db.WithContext(ctx).
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

func (r *groupRepositoryImpl) ensureOperatorCanAddMembers(ctx context.Context, tx *gorm.DB, groupUUID, operatorUUID string) error {
	_, err := r.ensureOperatorRoleAtLeast(ctx, tx, groupUUID, operatorUUID, memberRoleAdmin)
	return err
}

func isActiveGroupMember(member *model.GroupMember) bool {
	return member != nil && member.Status == memberStatusNormal && !member.DeletedAt.Valid
}

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
		r.rebuildGroupMembersCache(runCtx, groupUUID, cloned)
	}, async.AsyncRedisPipelineTimeout)
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
		Joins("JOIN groups AS g ON g.uuid = gm.group_uuid").
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
		r.rebuildGroupMembersCache(ctx, groupUUID, members)
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

func (r *groupRepositoryImpl) rebuildGroupMembersCache(ctx context.Context, groupUUID string, members []*model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
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
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return
	}

	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, cacheKey)
	pipe.HSet(ctx, cacheKey, fields)
	pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL))
	if _, err := pipe.Exec(ctx); err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return
		}
		LogRedisError(ctx, err)
	}
}

// insertGroupMembersCacheAsync 在群成员 Hash 已存在时，异步增量插入新成员 field。
//
// 该方法供后续 CreateGroup / AddMember 写路径复用；如果 key 已过期，则交给下一次读路径
// 统一全量重建，避免把局部成员集合误写成完整事实。
func (r *groupRepositoryImpl) insertGroupMembersCacheAsync(ctx context.Context, groupUUID string, members []*model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" || len(members) == 0 {
		return
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaInsertGroupMemberIfExists)
		expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
		seen := make(map[string]struct{}, len(members))
		for _, member := range members {
			if member == nil || member.UserUuid == "" {
				continue
			}
			if _, ok := seen[member.UserUuid]; ok {
				continue
			}
			seen[member.UserUuid] = struct{}{}

			_, err := luaScript.Run(runCtx, r.redisClient,
				[]string{cacheKey},
				member.UserUuid,
				encodeGroupMemberCacheValue(member),
				expireSeconds,
			).Result()
			if err != nil && err != goredis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, cacheKey).Err()
					return
				}
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

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaUpsertGroupMemberIfExists)
		expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())

		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			member.UserUuid,
			encodeGroupMemberCacheValue(member),
			expireSeconds,
		).Result()
		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
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

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaRemoveGroupMemberIfExists)
		expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())

		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			userUUID,
			groupMembersEmptyValue,
			expireSeconds,
		).Result()
		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
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

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.redisClient.Del(runCtx, cacheKey).Err(); err != nil && err != goredis.Nil {
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
