package repository

import (
	"context"
	"strconv"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type applyRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// NewApplyRepository 创建好友申请仓储实例。
func NewApplyRepository(db *gorm.DB, redisClient *goredis.Client) IApplyRepository {
	return &applyRepositoryImpl{db: db, redisClient: redisClient}
}

// Create 创建好友申请。
func (r *applyRepositoryImpl) Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
	// 先落库，再以“缓存存在才增量更新”的策略做 best-effort 缓存维护，避免把局部写入误当成完整列表。
	if err := r.db.WithContext(ctx).Create(apply).Error; err != nil {
		return nil, WrapDBError(err)
	}

	if r.redisClient == nil {
		return apply, nil
	}

	cacheKey := rediskey.ApplyPendingKey(apply.TargetUuid)
	luaScript := goredis.NewScript(luaAddPendingApplyIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.ApplyPendingTTL).Seconds())
	// 待处理申请列表只在 key 已存在时补丁写入；若缓存已过期，则交给读路径全量重建。
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		apply.CreatedAt.Unix(),
		apply.ApplicantUuid,
		expireSeconds,
	).Result()
	if err != nil && err != goredis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
	}

	if apply.ApplyType == 0 && apply.TargetUuid != "" {
		// 好友申请未读数仍沿用旧逻辑单独维护 Redis 计数，列表读取时再由上层清红点。
		notifyKey := rediskey.ApplyUnreadNotifyKey(apply.TargetUuid)
		pipe := r.redisClient.Pipeline()
		pipe.Incr(ctx, notifyKey)
		pipe.Expire(ctx, notifyKey, rediskey.ApplyUnreadNotifyTTL)
		if _, err := pipe.Exec(ctx); err != nil && err != goredis.Nil {
			LogRedisError(ctx, err)
		}
	}

	return apply, nil
}

// GetByID 根据主键查询一条好友申请。
func (r *applyRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	var apply model.ApplyRequest
	if err := r.db.WithContext(ctx).
		Where("id = ? AND apply_type = ? AND deleted_at IS NULL", id, 0).
		First(&apply).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return &apply, nil
}

// GetPendingList 获取收到的好友申请列表。
//
// 这里保留旧逻辑：待处理申请优先查 Redis ZSet，其余状态直接查 MySQL。
func (r *applyRepositoryImpl) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	page, pageSize, _ = normalizePage(page, pageSize)

	if status == 0 && r.redisClient != nil {
		applies, total, err := r.getPendingListFromCache(ctx, targetUUID, page, pageSize)
		if err == nil {
			return applies, total, nil
		}
		if err != goredis.Nil {
			LogRedisError(ctx, err)
		}
	}

	return r.getPendingListFromDB(ctx, targetUUID, status, page, pageSize)
}

func (r *applyRepositoryImpl) getPendingListFromCache(ctx context.Context, targetUUID string, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	if r.redisClient == nil {
		return nil, 0, goredis.Nil
	}

	cacheKey := rediskey.ApplyPendingKey(targetUUID)
	pipe := r.redisClient.Pipeline()
	totalCmd := pipe.ZCard(ctx, cacheKey)
	start := int64((page - 1) * pageSize)
	stop := start + int64(pageSize) - 1
	membersCmd := pipe.ZRevRange(ctx, cacheKey, start, stop)
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, 0, err
	}

	total := totalCmd.Val()
	applicantUUIDs := membersCmd.Val()
	// total=0 说明 key 不存在或已经被清空，此时让调用方回源数据库并触发全量重建。
	if total == 0 {
		return nil, 0, goredis.Nil
	}
	// __EMPTY__ 是空列表占位符，命中时直接返回空结果，避免高频空查询持续穿透 DB。
	if total == 1 && len(applicantUUIDs) == 1 && applicantUUIDs[0] == "__EMPTY__" {
		return []*model.ApplyRequest{}, 0, nil
	}

	// Redis 中只存 applicant_uuid，因此需要先过滤占位符，再批量回源查询完整申请记录。
	filteredUUIDs := make([]string, 0, len(applicantUUIDs))
	for _, uuid := range applicantUUIDs {
		if uuid != "__EMPTY__" {
			filteredUUIDs = append(filteredUUIDs, uuid)
		}
	}
	if len(filteredUUIDs) == 0 {
		return []*model.ApplyRequest{}, total, nil
	}

	var applies []*model.ApplyRequest
	if err := r.db.WithContext(ctx).
		Where("apply_type = ? AND target_uuid = ? AND status = ? AND applicant_uuid IN ? AND deleted_at IS NULL",
			0, targetUUID, 0, filteredUUIDs).
		Order("created_at DESC").
		Find(&applies).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	realTotal := total
	for _, uuid := range applicantUUIDs {
		if uuid == "__EMPTY__" {
			realTotal--
			break
		}
	}

	return applies, realTotal, nil
}

// getPendingListFromDB 从 MySQL 查询收到的好友申请列表，并在待处理场景下异步回填 Redis 缓存。
func (r *applyRepositoryImpl) getPendingListFromDB(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	_, _, offset := normalizePage(page, pageSize)

	// status<0 表示“全部状态”，其余值则保持和旧单体一致的单状态过滤语义。
	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND deleted_at IS NULL", 0, targetUUID)
	if status >= 0 {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status IN ?", []int{0, 1, 2})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	var applies []*model.ApplyRequest
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	if status == 0 && r.redisClient != nil {
		r.rebuildPendingCacheAsync(ctx, targetUUID)
	}

	return applies, total, nil
}

// rebuildPendingCacheAsync 异步从数据库重建某个用户的整份待处理申请缓存。
//
// 写路径只做补丁更新，真正的缓存真值重建统一放在读路径 miss 之后完成，保证集合完整性。
func (r *applyRepositoryImpl) rebuildPendingCacheAsync(ctx context.Context, targetUUID string) {
	if r.redisClient == nil || targetUUID == "" {
		return
	}

	cacheKey := rediskey.ApplyPendingKey(targetUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		// 先查全量待处理申请的申请人与时间戳，再整体覆盖 Redis ZSet，避免把单页结果误写成全集。
		var applies []model.ApplyRequest
		err := r.db.WithContext(runCtx).
			Select("applicant_uuid", "created_at").
			Where("apply_type = ? AND target_uuid = ? AND status = ? AND deleted_at IS NULL", 0, targetUUID, 0).
			Find(&applies).Error
		if err != nil {
			return
		}

		pipe := r.redisClient.Pipeline()
		pipe.Del(runCtx, cacheKey)
		// 空列表写入 __EMPTY__ 占位，非空列表则按创建时间倒序需求维护 score。
		if len(applies) == 0 {
			pipe.ZAdd(runCtx, cacheKey, goredis.Z{Score: 0, Member: "__EMPTY__"})
			pipe.Expire(runCtx, cacheKey, rediskey.ApplyPendingEmptyTTL)
		} else {
			zs := make([]goredis.Z, 0, len(applies))
			for _, apply := range applies {
				zs = append(zs, goredis.Z{
					Score:  float64(apply.CreatedAt.Unix()),
					Member: apply.ApplicantUuid,
				})
			}
			pipe.ZAdd(runCtx, cacheKey, zs...)
			pipe.Expire(runCtx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
		}

		if _, err := pipe.Exec(runCtx); err != nil && err != goredis.Nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisPipelineTimeout)
}

// GetSentList 获取当前用户发出的好友申请列表。
func (r *applyRepositoryImpl) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	_, pageSize, offset := normalizePage(page, pageSize)

	// 发出的申请列表不走缓存，保持与旧逻辑一致：统一直接查 MySQL。
	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND applicant_uuid = ? AND deleted_at IS NULL", 0, applicantUUID)
	if status >= 0 {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status IN ?", []int{0, 1, 2})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	var applies []*model.ApplyRequest
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	return applies, total, nil
}

// UpdateStatus 更新申请状态。
func (r *applyRepositoryImpl) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	updates := map[string]interface{}{"status": status}
	if remark != "" {
		updates["handle_remark"] = remark
	}

	// 通过 WHERE status=0 保持旧单体的 CAS 语义：只有待处理申请才允许流转到拒绝等终态。
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id = ? AND status = ?", id, 0).
		Updates(updates)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrApplyNotFound
	}

	return nil
}

// AcceptApplyAndCreateRelation 同意申请并建立双向好友关系。
func (r *applyRepositoryImpl) AcceptApplyAndCreateRelation(ctx context.Context, applyId int64, userUUID, friendUUID, remark string) (bool, error) {
	now := time.Now()
	var alreadyProcessed bool

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 第一步先用 status=0 做 CAS 守门，重复处理时直接按幂等成功返回。
		applyUpdates := map[string]interface{}{"status": 1}
		if remark != "" {
			applyUpdates["handle_remark"] = remark
		}

		result := tx.Model(&model.ApplyRequest{}).
			Where("id = ? AND status = ?", applyId, 0).
			Updates(applyUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			alreadyProcessed = true
			return nil
		}

		// 第二步写入当前处理人视角下的好友关系，并按旧逻辑允许覆盖当前侧备注。
		relationAB := &model.UserRelation{
			UserUuid:  userUUID,
			PeerUuid:  friendUUID,
			Status:    0,
			Remark:    remark,
			CreatedAt: now,
			UpdatedAt: now,
		}
		abUpdates := map[string]interface{}{
			"status":     0,
			"deleted_at": nil,
			"updated_at": now,
		}
		if remark != "" {
			abUpdates["remark"] = remark
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(abUpdates),
		}).Create(relationAB).Error; err != nil {
			return err
		}

		// 第三步恢复对向关系，但不覆盖对方可能已经维护过的 remark，保持旧单体的单向备注语义。
		relationBA := &model.UserRelation{
			UserUuid:  friendUUID,
			PeerUuid:  userUUID,
			Status:    0,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":     0,
				"deleted_at": nil,
				"updated_at": now,
			}),
		}).Create(relationBA).Error
	})
	if err != nil {
		return false, WrapDBError(err)
	}

	if !alreadyProcessed {
		r.invalidateFriendCacheAsync(ctx, userUUID, friendUUID, remark)
	}

	return alreadyProcessed, nil
}

// invalidateFriendCacheAsync 在好友申请同意后，按旧缓存模型补丁更新双方好友 Hash。
func (r *applyRepositoryImpl) invalidateFriendCacheAsync(ctx context.Context, userUUID, friendUUID, remark string) {
	if r.redisClient == nil {
		return
	}

	async.RunSafe(ctx, func(runCtx context.Context) {
		// A->B 方向允许写入 remark；B->A 方向只恢复好友存在性，不覆盖对方自己的备注。
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

		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		upsertScript := goredis.NewScript(luaUpsertFriendMetaIfExists)
		insertScript := goredis.NewScript(luaInsertFriendMetaIfExists)

		// 仍然遵循“key 存在才补丁写入”的缓存策略，避免在过期场景下把局部状态写成完整事实。
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
			if err != nil && err != goredis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, pair.userKey).Err()
					continue
				}
				LogRedisError(runCtx, err)
			}
		}
	}, async.AsyncRedisTimeout)
}

// MarkAsRead 标记指定申请已读。
func (r *applyRepositoryImpl) MarkAsRead(ctx context.Context, targetUUID string, ids []int64) (int64, error) {
	if len(ids) == 0 || targetUUID == "" {
		return 0, nil
	}

	// 仅更新当前用户收到的、仍未读的好友申请，避免越权或无效写放大。
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id IN ? AND target_uuid = ? AND apply_type = ? AND is_read = ? AND deleted_at IS NULL",
			ids, targetUUID, 0, false).
		Update("is_read", true)
	return result.RowsAffected, WrapDBError(result.Error)
}

// MarkAllAsRead 标记当前用户全部好友申请已读。
func (r *applyRepositoryImpl) MarkAllAsRead(ctx context.Context, targetUUID string) (int64, error) {
	if targetUUID == "" {
		return 0, nil
	}

	// “全部已读”只影响当前 targetUUID 收到的好友申请，不修改其他申请类型或其他用户的数据。
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND is_read = ? AND deleted_at IS NULL",
			0, targetUUID, false).
		Update("is_read", true)
	return result.RowsAffected, WrapDBError(result.Error)
}

// MarkAsReadAsync 异步标记申请已读。
func (r *applyRepositoryImpl) MarkAsReadAsync(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}

	// 列表读取后的已读回写属于 best-effort 行为，失败只记录 warn，不影响主流程返回。
	async.RunSafe(ctx, func(runCtx context.Context) {
		err := r.db.WithContext(runCtx).
			Model(&model.ApplyRequest{}).
			Where("id IN ? AND apply_type = ? AND is_read = ? AND deleted_at IS NULL", ids, 0, false).
			Update("is_read", true).Error
		if err != nil {
			logger.Warn(runCtx, "异步标记申请已读失败", logger.ErrorField("error", err))
		}
	}, async.AsyncDBTimeout)
}

// GetUnreadCount 获取未读申请数量。
func (r *applyRepositoryImpl) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	if targetUUID == "" {
		return 0, nil
	}

	if r.redisClient == nil {
		return r.countUnreadFromDB(ctx, targetUUID)
	}

	notifyKey := rediskey.ApplyUnreadNotifyKey(targetUUID)
	val, err := r.redisClient.Get(ctx, notifyKey).Result()
	if err != nil {
		if err == goredis.Nil {
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

	if err := r.redisClient.Expire(ctx, notifyKey, rediskey.ApplyUnreadNotifyTTL).Err(); err != nil && err != goredis.Nil {
		LogRedisError(ctx, err)
	}

	return count, nil
}

// countUnreadFromDB 从数据库兜底统计未读好友申请数量。
func (r *applyRepositoryImpl) countUnreadFromDB(ctx context.Context, targetUUID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND is_read = ? AND deleted_at IS NULL", 0, targetUUID, false).
		Count(&count).Error; err != nil {
		return 0, WrapDBError(err)
	}
	return count, nil
}

// ClearUnreadCount 清除未读申请红点。
func (r *applyRepositoryImpl) ClearUnreadCount(ctx context.Context, targetUUID string) error {
	if targetUUID == "" || r.redisClient == nil {
		return nil
	}

	// 红点清除只删除计数 key；申请记录本身的已读状态仍由 MarkAsRead / MarkAllAsRead 控制。
	notifyKey := rediskey.ApplyUnreadNotifyKey(targetUUID)
	if err := r.redisClient.Del(ctx, notifyKey).Err(); err != nil && err != goredis.Nil {
		return WrapRedisError(err)
	}
	return nil
}

// ExistsPendingRequest 判断当前是否已有待处理申请。
func (r *applyRepositoryImpl) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	if r.redisClient != nil {
		cacheKey := rediskey.ApplyPendingKey(targetUUID)
		// 先用缓存判断目标用户当前的待处理集合里是否已经有该申请人，减少重复发起时的数据库压力。
		pipe := r.redisClient.Pipeline()
		existsCmd := pipe.Exists(ctx, cacheKey)
		scoreCmd := pipe.ZScore(ctx, cacheKey, applicantUUID)
		if getRandomBool(0.01) {
			pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.ApplyPendingTTL))
		}

		_, err := pipe.Exec(ctx)
		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(ctx, cacheKey).Err()
			} else {
				LogRedisError(ctx, err)
			}
		} else if err == nil && existsCmd.Val() > 0 {
			if scoreCmd.Err() == nil {
				return true, nil
			}
			if scoreCmd.Err() == goredis.Nil {
				return false, nil
			}
			if isRedisWrongType(scoreCmd.Err()) {
				_ = r.redisClient.Del(ctx, cacheKey).Err()
			} else {
				LogRedisError(ctx, scoreCmd.Err())
			}
		}
	}

	// 缓存未命中时回源全量待处理申请，再异步重建 ZSet，保持和旧单体一致的 Cache-Aside 语义。
	var applies []model.ApplyRequest
	if err := r.db.WithContext(ctx).
		Where("apply_type = ? AND target_uuid = ? AND status = ? AND deleted_at IS NULL", 0, targetUUID, 0).
		Find(&applies).Error; err != nil {
		return false, WrapDBError(err)
	}

	if r.redisClient != nil {
		r.rebuildPendingCacheAsync(ctx, targetUUID)
	}

	for _, apply := range applies {
		if apply.ApplicantUuid == applicantUUID {
			return true, nil
		}
	}
	return false, nil
}

// GetByIDWithInfo 当前阶段仅返回申请记录本身。
func (r *applyRepositoryImpl) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	return r.GetByID(ctx, id)
}
