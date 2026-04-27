package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// applyRepositoryImpl 好友申请数据访问层实现
type applyRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewApplyRepository 创建好友申请仓储实例
func NewApplyRepository(db *gorm.DB, redisClient *redis.Client) IApplyRepository {
	return &applyRepositoryImpl{db: db, redisClient: redisClient}
}

// Create 创建好友申请
func (r *applyRepositoryImpl) Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
	err := r.db.WithContext(ctx).Create(apply).Error
	if err != nil {
		return nil, WrapDBError(err)
	}

	// 尽力而为地更新目标用户的待处理申请缓存。
	// 关键原则：只有 Key 存在时才增量添加，Key 不存在时不操作（让读接口负责全量加载）。
	// 这避免了 Key 过期后增量添加导致缓存数据不完整的问题。
	cacheKey := rediskey.ApplyPendingKey(apply.TargetUuid)

	// 使用 Lua 脚本原子性地：检查 Key 存在 -> 移除占位符 -> 添加新成员 -> 续期
	luaScript := redis.NewScript(luaAddPendingApplyIfExists)

	expireSeconds := int(getRandomExpireTime(rediskey.ApplyPendingTTL).Seconds())
	_, err = luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		apply.CreatedAt.Unix(),
		apply.ApplicantUuid,
		expireSeconds,
	).Result()

	if err != nil && err != redis.Nil {
		// Lua 脚本执行失败，记录日志但不阻塞主流程
		// 注意：Key 不存在返回 0 不是错误，读接口会负责全量加载
		LogRedisError(ctx, err)
	}

	// 更新好友申请未读数量（仅好友申请）
	if apply.ApplyType == 0 && apply.TargetUuid != "" {
		notifyKey := rediskey.ApplyUnreadNotifyKey(apply.TargetUuid)
		notifyTTL := rediskey.ApplyUnreadNotifyTTL
		pipe := r.redisClient.Pipeline()
		pipe.Incr(ctx, notifyKey)
		pipe.Expire(ctx, notifyKey, notifyTTL)
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			LogRedisError(ctx, err)
		}
	}

	return apply, nil
}

// GetByID 根据ID获取好友申请
func (r *applyRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	var apply model.ApplyRequest
	err := r.db.WithContext(ctx).
		Where("id = ? AND apply_type = ? AND deleted_at IS NULL", id, 0).
		First(&apply).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &apply, nil
}

// GetPendingList 获取待处理的好友申请列表
// 冷热分离策略：
//   - status=0 (待处理)：高热度数据，优先查 Redis ZSet，未命中回源 MySQL
//   - status=1/2 (已处理)：历史冷数据，直接查 MySQL
//   - status<0 (全部)：合并分页复杂，直接查 MySQL
func (r *applyRepositoryImpl) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	// 兜底分页参数
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 冷热分离：status=0 走 Redis 缓存
	if status == 0 {
		applies, total, err := r.getPendingListFromCache(ctx, targetUUID, page, pageSize)
		if err == nil {
			return applies, total, nil
		}
		// Redis 未命中或失败，降级走 MySQL 其中失败情况下打日志
		if err != redis.Nil {
			LogRedisError(ctx, err)
		}
	}

	// status=1/2 或 status<0 或缓存失败：直接查 MySQL
	return r.getPendingListFromDB(ctx, targetUUID, status, page, pageSize)
}

// getPendingListFromCache 从 Redis ZSet 获取待处理申请列表（仅 status=0）
// 返回 error 表示缓存未命中或失败，调用方应降级到 MySQL
func (r *applyRepositoryImpl) getPendingListFromCache(ctx context.Context, targetUUID string, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	cacheKey := rediskey.ApplyPendingKey(targetUUID)

	// 1. Pipeline 查询：总数 + 分页成员
	pipe := r.redisClient.Pipeline()
	totalCmd := pipe.ZCard(ctx, cacheKey)
	start := int64((page - 1) * pageSize)
	stop := start + int64(pageSize) - 1
	membersCmd := pipe.ZRevRange(ctx, cacheKey, start, stop) // 按 score(created_at) 倒序

	// 概率续期：1% 概率续期避免热点 key 过期
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, 0, err
	}

	total := totalCmd.Val()
	applicantUUIDs := membersCmd.Val()

	// 2. 缓存未命中（key 不存在）
	if total == 0 {
		return nil, 0, redis.Nil
	}

	// 3. 空值占位符检查
	if total == 1 && len(applicantUUIDs) == 1 && applicantUUIDs[0] == "__EMPTY__" {
		return []*model.ApplyRequest{}, 0, nil
	}

	// 过滤掉可能的空值占位符
	filteredUUIDs := make([]string, 0, len(applicantUUIDs))
	for _, uuid := range applicantUUIDs {
		if uuid != "__EMPTY__" {
			filteredUUIDs = append(filteredUUIDs, uuid)
		}
	}

	if len(filteredUUIDs) == 0 {
		return []*model.ApplyRequest{}, total, nil
	}

	// 4. 根据 applicantUUIDs 批量查 MySQL 补全完整字段
	var applies []*model.ApplyRequest
	err = r.db.WithContext(ctx).
		Where("apply_type = ? AND target_uuid = ? AND status = ? AND applicant_uuid IN ? AND deleted_at IS NULL",
			0, targetUUID, 0, filteredUUIDs).
		Order("created_at DESC").
		Find(&applies).Error
	if err != nil {
		return nil, 0, WrapDBError(err)
	}

	// 5. 如果有占位符，总数需要减 1
	realTotal := total
	if total > 0 {
		// 检查是否包含占位符（通过原始列表）
		for _, uuid := range applicantUUIDs {
			if uuid == "__EMPTY__" {
				realTotal--
				break
			}
		}
	}

	return applies, realTotal, nil
}

// getPendingListFromDB 从 MySQL 查询好友申请列表
func (r *applyRepositoryImpl) getPendingListFromDB(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	offset := (page - 1) * pageSize

	// 基础条件：仅好友申请 + 指定目标用户 + 未删除
	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND deleted_at IS NULL", 0, targetUUID)

	// status >= 0 时按指定状态过滤；否则返回全部(0/1/2)状态
	if status >= 0 {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status IN ?", []int{0, 1, 2})
	}

	// 先查总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	// 再查列表，按创建时间倒序
	var applies []*model.ApplyRequest
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).
		Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	// status=0 且缓存未命中，异步重建 Redis 缓存（需要查全量数据）
	if status == 0 {
		r.rebuildPendingCacheAsync(ctx, targetUUID)
	}

	return applies, total, nil
}

// rebuildPendingCacheAsync 异步重建待处理申请的 Redis 缓存
// 注意：必须重新查询全量数据，不能使用分页数据
func (r *applyRepositoryImpl) rebuildPendingCacheAsync(ctx context.Context, targetUUID string) {
	cacheKey := rediskey.ApplyPendingKey(targetUUID)

	async.RunSafe(ctx, func(runCtx context.Context) {
		// 1. 重新查询全量待处理申请（只需要 applicant_uuid 和 created_at）
		var applies []model.ApplyRequest
		err := r.db.WithContext(runCtx).
			Select("applicant_uuid", "created_at").
			Where("apply_type = ? AND target_uuid = ? AND status = ? AND deleted_at IS NULL", 0, targetUUID, 0).
			Find(&applies).Error
		if err != nil {
			// 异步重建缓存失败静默忽略，不影响主流程
			return
		}

		// 2. 重建缓存
		pipe := r.redisClient.Pipeline()
		pipe.Del(runCtx, cacheKey)

		if len(applies) == 0 {
			// 空值占位，防止缓存穿透
			pipe.ZAdd(runCtx, cacheKey, redis.Z{
				Score:  0,
				Member: "__EMPTY__",
			})
			pipe.Expire(runCtx, cacheKey, rediskey.ApplyPendingEmptyTTL)
		} else {
			zs := make([]redis.Z, 0, len(applies))
			for _, apply := range applies {
				zs = append(zs, redis.Z{
					Score:  float64(apply.CreatedAt.Unix()),
					Member: apply.ApplicantUuid,
				})
			}
			pipe.ZAdd(runCtx, cacheKey, zs...)
			pipe.Expire(runCtx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
		}

		if _, err := pipe.Exec(runCtx); err != nil {
			LogRedisError(runCtx, err)
		}
	}, 0)
}

// GetSentList 获取发出的好友申请列表
func (r *applyRepositoryImpl) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	// 兜底分页参数
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// 基础条件：仅好友申请 + 指定申请人 + 未删除
	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND applicant_uuid = ? AND deleted_at IS NULL", 0, applicantUUID)

	// status >= 0 时按指定状态过滤；否则返回全部(0/1/2)状态
	if status >= 0 {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status IN ?", []int{0, 1, 2})
	}

	// 先查总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	// 再查列表，按创建时间倒序
	var applies []*model.ApplyRequest
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).
		Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	return applies, total, nil
}

// UpdateStatus 更新申请状态
func (r *applyRepositoryImpl) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if remark != "" {
		updates["handle_remark"] = remark
	}

	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id = ? AND status = ?", id, 0). // 只能处理待处理状态的申请
		Updates(updates)

	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	// 如果没有更新任何行，说明申请不存在或已处理
	if result.RowsAffected == 0 {
		return ErrApplyNotFound
	}

	return nil
}

// AcceptApplyAndCreateRelation 同意申请并创建好友关系（事务 + CAS幂等）
// 在同一事务中执行：
//  1. CAS 更新申请状态（WHERE status=0 守门员）
//  2. 创建双向好友关系（拆分 Upsert 处理 remark）
//     - A→B：更新 remark（用户同意时给对方的备注）
//     - B→A：不覆盖 remark（保留对方原有备注）
//
// 返回值:
//   - alreadyProcessed=true: 申请已被处理（幂等成功，不是错误）
//   - err: 真正的数据库错误
func (r *applyRepositoryImpl) AcceptApplyAndCreateRelation(ctx context.Context, applyId int64, userUUID, friendUUID, remark string) (bool, error) {
	now := time.Now()
	var alreadyProcessed bool

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. CAS 更新申请状态（WHERE status=0 作为守门员）
		applyUpdates := map[string]interface{}{
			"status": 1, // 同意
		}
		if remark != "" {
			applyUpdates["handle_remark"] = remark
		}

		result := tx.Model(&model.ApplyRequest{}).
			Where("id = ? AND status = ?", applyId, 0).
			Updates(applyUpdates)

		if result.Error != nil {
			return result.Error
		}

		// 幂等判断：RowsAffected=0 表示已被处理
		if result.RowsAffected == 0 {
			alreadyProcessed = true
			return nil // 不触发回滚，幂等成功
		}

		// 2. 创建 A→B 关系（更新 remark）
		// 用户 A（userUUID）同意 B（friendUUID）的申请，A 给 B 设置备注
		relationAB := &model.UserRelation{
			UserUuid:  userUUID,
			PeerUuid:  friendUUID,
			Status:    0,
			Remark:    remark, // A 给 B 的备注
			CreatedAt: now,
			UpdatedAt: now,
		}

		// A→B 的更新策略：覆盖 remark
		abUpdates := map[string]interface{}{
			"status":     0,
			"deleted_at": nil,
			"updated_at": now,
		}
		if remark != "" {
			abUpdates["remark"] = remark // 更新 A 给 B 的备注
		}

		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(abUpdates),
		}).Create(relationAB).Error
		if err != nil {
			return err
		}

		// 3. 创建 B→A 关系（不覆盖 remark，保留 B 原有的对 A 的备注）
		relationBA := &model.UserRelation{
			UserUuid:  friendUUID,
			PeerUuid:  userUUID,
			Status:    0,
			CreatedAt: now,
			UpdatedAt: now,
		}

		// B→A 的更新策略：只恢复状态，不覆盖 remark
		err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":     0,
				"deleted_at": nil,
				"updated_at": now,
				// 注意：不更新 remark，保留 B 原有对 A 的备注
			}),
		}).Create(relationBA).Error

		return err
	})

	if err != nil {
		return false, WrapDBError(err)
	}

	// 4. 事务成功后异步更新 Redis 好友缓存
	if !alreadyProcessed {
		r.invalidateFriendCacheAsync(ctx, userUUID, friendUUID, remark)
	}

	return alreadyProcessed, nil
}

// invalidateFriendCacheAsync 异步更新双方的好友缓存
func (r *applyRepositoryImpl) invalidateFriendCacheAsync(ctx context.Context, userUUID, friendUUID, remark string) {
	async.RunSafe(ctx, func(runCtx context.Context) {
		// 处理两个用户的缓存
		pairs := []struct {
			userKey   string
			newFriend string
			metaJSON  string
			upsert    bool
		}{
			{
				userKey:   rediskey.FriendRelationKey(userUUID),
				newFriend: friendUUID,
				metaJSON:  buildFriendMetaJSON(remark, "", "", time.Now().UnixMilli()),
				upsert:    true,
			},
			{
				userKey:   rediskey.FriendRelationKey(friendUUID),
				newFriend: userUUID,
				metaJSON:  buildFriendMetaJSON("", "", "", time.Now().UnixMilli()),
				upsert:    false,
			},
		}

		expireSeconds := int(getRandomExpireTime(24 * time.Hour).Seconds())
		upsertScript := redis.NewScript(luaUpsertFriendMetaIfExists)
		insertScript := redis.NewScript(luaInsertFriendMetaIfExists)

		for _, pair := range pairs {
			script := insertScript
			if pair.upsert {
				script = upsertScript
			}
			_, err := script.Run(runCtx, r.redisClient,
				[]string{pair.userKey},
				pair.newFriend,
				pair.metaJSON,
				expireSeconds,
			).Result()
			if err != nil && err != redis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, pair.userKey).Err()
					continue
				}
				logger.Error(runCtx, "Redis 脚本执行失败", logger.ErrorField("error", err))
			}
		}
	}, 0)
}

// MarkAsRead 标记申请已读（同步）
func (r *applyRepositoryImpl) MarkAsRead(ctx context.Context, targetUUID string, ids []int64) (int64, error) {
	if len(ids) == 0 || targetUUID == "" {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id IN ? AND target_uuid = ? AND apply_type = ? AND is_read = ? AND deleted_at IS NULL",
			ids, targetUUID, 0, false).
		Update("is_read", true)
	return result.RowsAffected, WrapDBError(result.Error)
}

// MarkAllAsRead 标记当前用户所有好友申请已读（同步）
func (r *applyRepositoryImpl) MarkAllAsRead(ctx context.Context, targetUUID string) (int64, error) {
	if targetUUID == "" {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND is_read = ? AND deleted_at IS NULL",
			0, targetUUID, false).
		Update("is_read", true)
	return result.RowsAffected, WrapDBError(result.Error)
}

// MarkAsReadAsync 异步标记申请已读（不阻塞主请求）
// 批量更新，仅更新 is_read=false 的记录避免无效写入
func (r *applyRepositoryImpl) MarkAsReadAsync(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}

	// 使用 async.RunSafe 异步执行，自带 panic recover 和超时控制
	async.RunSafe(ctx, func(runCtx context.Context) {
		err := r.db.WithContext(runCtx).
			Model(&model.ApplyRequest{}).
			Where("id IN ? AND apply_type = ? AND is_read = ? AND deleted_at IS NULL", ids, 0, false).
			Update("is_read", true).Error
		if err != nil {
			// 异步更新失败只记录日志，不影响主流程
			logger.Error(runCtx, "异步标记申请已读失败", logger.ErrorField("error", err))
		}
	}, 0) // timeout=0 使用默认 1 分钟超时
}

// GetUnreadCount 获取未读申请数量
func (r *applyRepositoryImpl) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	if targetUUID == "" {
		return 0, nil
	}

	notifyKey := rediskey.ApplyUnreadNotifyKey(targetUUID)
	notifyTTL := rediskey.ApplyUnreadNotifyTTL
	val, err := r.redisClient.Get(ctx, notifyKey).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, WrapRedisError(err)
	}

	count, convErr := strconv.ParseInt(val, 10, 64)
	if convErr != nil {
		logger.Warn(ctx, "未读数量解析失败",
			logger.String("value", val),
			logger.ErrorField("error", convErr),
		)
		return 0, nil
	}
	if count < 0 {
		count = 0
	}

	// 续期（尽力而为）
	if err := r.redisClient.Expire(ctx, notifyKey, notifyTTL).Err(); err != nil && err != redis.Nil {
		LogRedisError(ctx, err)
	}

	return count, nil
}

// ClearUnreadCount 清除未读申请数量（红点清除）
func (r *applyRepositoryImpl) ClearUnreadCount(ctx context.Context, targetUUID string) error {
	if targetUUID == "" {
		return nil
	}
	notifyKey := rediskey.ApplyUnreadNotifyKey(targetUUID)
	if err := r.redisClient.Del(ctx, notifyKey).Err(); err != nil && err != redis.Nil {
		return WrapRedisError(err)
	}
	return nil
}

// ExistsPendingRequest 检查是否存在待处理的申请
// 采用 Cache-Aside Pattern：优先查 Redis ZSet，未命中则回源 MySQL 并缓存
// 使用 ZSet 存储目标用户的待处理申请，以申请时间戳为 score
func (r *applyRepositoryImpl) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	cacheKey := rediskey.ApplyPendingKey(targetUUID)

	// ==================== 1. 组合查询 Redis (Pipeline) ====================
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	scoreCmd := pipe.ZScore(ctx, cacheKey, applicantUUID)

	// 概率续期优化：1% 的概率在读取时顺便续期
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		LogRedisError(ctx, err)
	} else if err == nil {
		if existsCmd.Val() > 0 {
			// Cache hit: if member exists, it has a score.
			if scoreCmd.Err() == nil {
				return true, nil
			}
			if scoreCmd.Err() == redis.Nil {
				return false, nil
			}
			LogRedisError(ctx, scoreCmd.Err())
		}
	}

	// ==================== 2. 缓存未命中，回源查询 MySQL ====================
	var applies []model.ApplyRequest
	err = r.db.WithContext(ctx).
		Where("apply_type = ? AND target_uuid = ? AND status = ? AND deleted_at IS NULL", 0, targetUUID, 0).
		Find(&applies).Error
	if err != nil {
		return false, WrapDBError(err)
	}

	// ==================== 3. 重建缓存 (ZSet) ====================
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		pipe.Del(runCtx, cacheKey)
		if len(applies) == 0 {
			pipe.ZAdd(runCtx, cacheKey, redis.Z{
				Score:  0,
				Member: "__EMPTY__",
			})
			pipe.Expire(runCtx, cacheKey, rediskey.ApplyPendingEmptyTTL)
		} else {
			zs := make([]redis.Z, 0, len(applies))
			for _, apply := range applies {
				zs = append(zs, redis.Z{
					Score:  float64(apply.CreatedAt.Unix()),
					Member: apply.ApplicantUuid,
				})
			}
			pipe.ZAdd(runCtx, cacheKey, zs...)
			pipe.Expire(runCtx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
		}
		if _, err := pipe.Exec(runCtx); err != nil {
			LogRedisError(runCtx, err)
		}
	}, 0)

	// ==================== 4. 根据回源结果判断 ====================
	for _, apply := range applies {
		if apply.ApplicantUuid == applicantUUID {
			return true, nil
		}
	}
	return false, nil
}

// GetByIDWithInfo 根据ID获取好友申请（仅申请记录）
func (r *applyRepositoryImpl) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	return r.GetByID(ctx, id)
}
