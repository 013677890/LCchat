package repository

import (
	"github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// friendRepositoryImpl 好友关系数据访问层实现
type friendRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewFriendRepository 创建好友关系仓储实例
func NewFriendRepository(db *gorm.DB, redisClient *redis.Client) IFriendRepository {
	return &friendRepositoryImpl{db: db, redisClient: redisClient}
}

// GetFriendList 获取好友列表
func (r *friendRepositoryImpl) GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, int64, error) {
	// 兜底分页参数
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// 基础条件：仅好友关系 + 指定用户 + 未删除
	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, 0)
	if groupTag != "" {
		query = query.Where("group_tag = ?", groupTag)
	}

	var total int64
	var version int64

	// 只在第一页计算 total 和 version，减少数据库开销
	if page == 1 {
		// 先查总数
		if err := query.Count(&total).Error; err != nil {
			return nil, 0, 0, WrapDBError(err)
		}

		// 全量初始化版本号取当前服务器时间
		version = time.Now().UnixMilli()
	}

	// 再查列表，按创建时间倒序（加二级排序保证稳定性）
	var relations []*model.UserRelation
	if err := query.
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&relations).
		Error; err != nil {
		return nil, 0, 0, WrapDBError(err)
	}

	return relations, total, version, nil
}

// GetFriendRelation 获取好友关系
func (r *friendRepositoryImpl) GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error) {
	return nil, nil // TODO: 实现获取好友关系
}

// CreateFriendRelation 创建好友关系（双向）
// 使用 Upsert (INSERT ON DUPLICATE KEY UPDATE) 策略：
//   - 原子性：不存在"查不到然后插入报错"的时间差
//   - 性能：2 条 SELECT + 2 条 INSERT 变成 1 条 INSERT
//   - 稳健：正确处理软删除记录恢复场景
func (r *friendRepositoryImpl) CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	now := time.Now()

	// 1. 构建双向关系
	relations := []*model.UserRelation{
		{
			UserUuid:  userUUID,
			PeerUuid:  friendUUID,
			Status:    0, // 正常状态
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			UserUuid:  friendUUID,
			PeerUuid:  userUUID,
			Status:    0, // 正常状态
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	// 2. 批量 Upsert (Insert On Duplicate Key Update)
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		// 指定冲突列（必须是数据库的唯一索引列）
		Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
		// 冲突时执行更新操作
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":     0,   // 恢复正常状态
			"deleted_at": nil, // 【关键】恢复软删除
			"updated_at": now, // 更新时间
		}),
	}).Create(&relations).Error

	if err != nil {
		return WrapDBError(err)
	}

	// 3. 异步更新 Redis 好友列表缓存（合并为一个调用减少协程开销）
	r.invalidateFriendCacheAsync(ctx, userUUID, friendUUID)

	return nil
}

// DeleteFriendRelation 删除好友关系（单向）
func (r *friendRepositoryImpl) DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	now := time.Now()

	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{
			"status":     2,
			"deleted_at": gorm.DeletedAt{Time: now, Valid: true},
			"updated_at": now,
		})

	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	// 异步增量更新缓存（仅更新当前用户侧）
	r.removeFriendCacheAsync(ctx, userUUID, friendUUID)

	return nil
}

// SetFriendRemark 设置好友备注
func (r *friendRepositoryImpl) SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{
			"remark":     remark,
			"updated_at": now,
		})

	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	r.updateFriendRemarkCacheAsync(ctx, userUUID, friendUUID, remark, now.UnixMilli())

	return nil
}

// SetFriendTag 设置好友标签
func (r *friendRepositoryImpl) SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{
			"group_tag":  groupTag,
			"updated_at": now,
		})

	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	r.updateFriendTagCacheAsync(ctx, userUUID, friendUUID, groupTag, now.UnixMilli())

	return nil
}

// GetTagList 获取标签列表
func (r *friendRepositoryImpl) GetTagList(ctx context.Context, userUUID string) ([]string, error) {
	return nil, nil // TODO: 获取标签列表
}

// IsFriend 检查是否是好友
// 采用 Cache-Aside Pattern：优先查 Redis Hash，未命中则回源 MySQL 并缓存
func (r *friendRepositoryImpl) IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error) {
	cacheKey := rediskey.FriendRelationKey(userUUID)

	// ==================== 1. 组合查询 Redis (Pipeline) ====================
	// 使用 Pipeline 一次性发送命令，减少网络 RTT
	pipe := r.redisClient.Pipeline()

	// 命令1: 检查 Key 是否存在 (区分缓存命中/未命中)
	existsCmd := pipe.Exists(ctx, cacheKey)
	// 命令2: 读取好友元数据 (只有 Key 存在时此结果才有效)
	metaCmd := pipe.HGet(ctx, cacheKey, friendUUID)

	// 概率续期优化：1% 的概率在读取时顺便续期
	// 无论 Key 是否存在，Expire 都是安全的 (不存在则返回0)
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
	}

	_, err := pipe.Exec(ctx)

	if err != nil && err != redis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			// Redis 挂了，记录日志，降级去查 DB
			LogRedisError(ctx, err)
		}
	} else if err == nil {
		// Redis 正常返回
		// 核心逻辑：先看 Key 在不在
		if existsCmd.Val() > 0 {
			// Case A: 缓存命中 (Hit)
			if metaCmd.Err() == nil {
				_, _ = parseFriendMetaJSON(metaCmd.Val())
				return true, nil
			}
			if metaCmd.Err() == redis.Nil {
				return false, nil
			}
			if isRedisWrongType(metaCmd.Err()) {
				_ = r.redisClient.Del(ctx, cacheKey).Err()
			} else {
				LogRedisError(ctx, metaCmd.Err())
			}
		}
		// Case B: 缓存未命中 (Miss) -> Exists 返回 0
		// 代码继续往下走，去查数据库
	}

	// ==================== 2. 缓存未命中，回源查询 MySQL ====================
	var relations []model.UserRelation
	err = r.db.WithContext(ctx).
		Where("user_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, 0).
		Find(&relations).Error

	if err != nil {
		return false, WrapDBError(err)
	}

	// ==================== 3. 重建缓存 (Hash) ====================
	r.rebuildFriendCacheAsync(ctx, userUUID, relations)

	// 计算结果
	isFriendFound := false
	for _, relation := range relations {
		if relation.PeerUuid == friendUUID {
			isFriendFound = true
			break
		}
	}

	return isFriendFound, nil
}

// checkFriendCache 检查单侧缓存命中情况
// 返回值: cacheHit(该用户缓存是否存在), isFriend(是否包含对方)
func (r *friendRepositoryImpl) checkFriendCache(ctx context.Context, userUUID, friendUUID string) (bool, bool) {
	if userUUID == "" || friendUUID == "" {
		return false, false
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	metaCmd := pipe.HGet(ctx, cacheKey, friendUUID)

	// 概率续期优化：1% 的概率在读取时顺便续期
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
		return false, false
	}

	if existsCmd.Val() == 0 {
		return false, false
	}

	if metaCmd.Err() == nil {
		_, _ = parseFriendMetaJSON(metaCmd.Val())
		return true, true
	}
	if metaCmd.Err() == redis.Nil {
		return true, false
	}
	if isRedisWrongType(metaCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, false
	}

	LogRedisError(ctx, metaCmd.Err())
	return false, false
}

// getFriendMetaCache 获取好友元数据缓存
// 返回值: cacheHit(该用户缓存是否存在), meta(好友元数据), isFriend(是否包含对方)
func (r *friendRepositoryImpl) getFriendMetaCache(ctx context.Context, userUUID, friendUUID string) (bool, *friendMeta, bool) {
	if userUUID == "" || friendUUID == "" {
		return false, nil, false
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	metaCmd := pipe.HGet(ctx, cacheKey, friendUUID)

	// 概率续期优化：1% 的概率在读取时顺便续期
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
		return false, nil, false
	}

	if existsCmd.Val() == 0 {
		return false, nil, false
	}

	if metaCmd.Err() == nil {
		meta, err := parseFriendMetaJSON(metaCmd.Val())
		if err != nil {
			return true, nil, true
		}
		return true, meta, true
	}
	if metaCmd.Err() == redis.Nil {
		return true, nil, false
	}
	if isRedisWrongType(metaCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, nil, false
	}

	LogRedisError(ctx, metaCmd.Err())
	return false, nil, false
}

// checkBlacklistCache 检查黑名单缓存命中情况
// 返回值: cacheHit(该用户缓存是否存在), isBlocked(是否包含对方)
func (r *friendRepositoryImpl) checkBlacklistCache(ctx context.Context, userUUID, peerUUID string) (bool, bool) {
	if userUUID == "" || peerUUID == "" {
		return false, false
	}

	cacheKey := rediskey.BlacklistRelationKey(userUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	memberCmd := pipe.SIsMember(ctx, cacheKey, peerUUID)

	// 概率续期优化：1% 的概率在读取时顺便续期
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.BlacklistTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
		return false, false
	}

	if existsCmd.Val() == 0 {
		return false, false
	}

	if memberCmd.Err() == nil {
		return true, memberCmd.Val()
	}
	if isRedisWrongType(memberCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, false
	}

	LogRedisError(ctx, memberCmd.Err())
	return false, false
}

// CheckIsFriendRelation 判断两用户是否存在好友关系（以 userUUID 为准，先查 Redis，未命中再查 DB）
func (r *friendRepositoryImpl) CheckIsFriendRelation(ctx context.Context, userUUID, peerUUID string) (bool, error) {
	cacheHit, isFriend := r.checkFriendCache(ctx, userUUID, peerUUID)
	if cacheHit {
		return isFriend, nil
	}

	// 缓存未命中，回源 DB（仅以 userUUID 视角判断）
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, peerUUID).
		Where("status = ? AND deleted_at IS NULL", 0).
		Count(&count).Error
	if err != nil {
		return false, WrapDBError(err)
	}

	return count > 0, nil
}

// GetRelationStatus 获取关系状态
func (r *friendRepositoryImpl) GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
	friendHit, meta, isFriend := r.getFriendMetaCache(ctx, userUUID, peerUUID)
	if friendHit && isFriend {
		relation := &model.UserRelation{
			UserUuid: userUUID,
			PeerUuid: peerUUID,
			Status:   0,
		}
		if meta != nil {
			relation.Remark = meta.Remark
			relation.GroupTag = meta.GroupTag
			relation.Source = meta.Source
		}
		return relation, nil
	}

	blacklistHit, isBlacklist := r.checkBlacklistCache(ctx, userUUID, peerUUID)
	if blacklistHit && isBlacklist {
		return &model.UserRelation{
			UserUuid: userUUID,
			PeerUuid: peerUUID,
			Status:   1,
		}, nil
	}

	if friendHit && !isFriend && blacklistHit && !isBlacklist {
		return nil, nil
	}

	var relation model.UserRelation
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, peerUUID).
		First(&relation).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, WrapDBError(err)
	}

	return &relation, nil
}

// SyncFriendList 增量同步好友列表
// 返回值: 变更列表, nextVersion(客户端下次用的时间戳), hasMore(是否还有更多), error
func (r *friendRepositoryImpl) SyncFriendList(ctx context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, bool, error) {
    // 1. 准备查询
    // 客户端传来的 version 是毫秒，转成 time.Time
    lastTime := time.UnixMilli(version)
    
    var relations []*model.UserRelation

    // 2. 执行查询 (极致精简)
    // 核心假设：GORM 软删除时会自动更新 updated_at。如果你的 GORM 配置没关这个，这就没问题。
    err := r.db.WithContext(ctx).
        Unscoped(). // 必须查出已删除的
        Model(&model.UserRelation{}).
        Where("user_uuid = ?", userUUID).
        Where("updated_at > ?", lastTime). // 核心：只看 update 时间，利用索引
        Order("updated_at ASC").           // 核心：利用索引排序，千万别用函数
        Limit(limit + 1).                  // 多查一条，用于判断 hasMore
        Find(&relations).Error

    if err != nil {
        return nil, 0, false, WrapDBError(err)
    }

    // 3. 计算 hasMore 和 nextVersion
    hasMore := false
    var nextVersion int64

    if len(relations) > limit {
        hasMore = true
        relations = relations[:limit] // 去掉多查的那一条
        // 情况 A：还有更多数据，Cursor 必须是本批次最后一条的时间
        nextVersion = relations[len(relations)-1].UpdatedAt.UnixMilli()
    } else {
        hasMore = false
        // 情况 B：没有更多数据了（追平了）
        // 这里的 nextVersion 可以是最后一条的时间，也可以是 ServerTime
        // 推荐：取 ServerTime 并回退 5 秒（安全窗口），防止事务并发导致的数据丢失
        safeTime := time.Now().Add(-5 * time.Second).UnixMilli()
        
        // 如果列表为空，直接用 safeTime；如果不为空，取 max(lastItem, safeTime)
        if len(relations) > 0 {
            lastItemTime := relations[len(relations)-1].UpdatedAt.UnixMilli()
            if lastItemTime > safeTime {
                nextVersion = lastItemTime
            } else {
                nextVersion = safeTime
            }
        } else {
            // 如果本来就没数据，说明 version 已经很新了，保持原样或推进到 safeTime
            if safeTime > version {
                nextVersion = safeTime
            } else {
                nextVersion = version
            }
        }
    }

    return relations, nextVersion, hasMore, nil
}

// BatchCheckIsFriend 批量检查是否为好友（使用Redis Hash优化）
// 返回：map[peerUUID]isFriend
func (r *friendRepositoryImpl) BatchCheckIsFriend(ctx context.Context, userUUID string, peerUUIDs []string) (map[string]bool, error) {
	if len(peerUUIDs) == 0 {
		return make(map[string]bool), nil
	}

	// 构建 Redis Hash key
	cacheKey := rediskey.FriendRelationKey(userUUID)

	// ==================== 1. 组合查询 Redis (Pipeline) ====================
	pipe := r.redisClient.Pipeline()

	// 命令1: 检查 Key 是否存在 (区分缓存命中/未命中)
	existsCmd := pipe.Exists(ctx, cacheKey)

	// 命令2: 批量读取好友元数据
	metaCmd := pipe.HMGet(ctx, cacheKey, peerUUIDs...)

	// 概率续期优化：1% 的概率在读取时顺便续期
	// 无论 Key 是否存在，Expire 都是安全的 (不存在则返回0)
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
	}

	_, err := pipe.Exec(ctx)

	if err != nil && err != redis.Nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			// Redis 挂了，记录日志，降级去查 DB
			LogRedisError(ctx, err)
		}
	} else if err == nil {
		// Redis 正常返回
		// 核心逻辑：先看 Key 在不在
		if existsCmd.Val() > 0 {
			if metaCmd.Err() != nil {
				if isRedisWrongType(metaCmd.Err()) {
					_ = r.redisClient.Del(ctx, cacheKey).Err()
				} else {
					LogRedisError(ctx, metaCmd.Err())
				}
			} else {
				result := make(map[string]bool, len(peerUUIDs))
				values := metaCmd.Val()
				for i, peerUUID := range peerUUIDs {
					if i >= len(values) || values[i] == nil {
						result[peerUUID] = false
						continue
					}
					switch v := values[i].(type) {
					case string:
						_, _ = parseFriendMetaJSON(v)
					case []byte:
						_, _ = parseFriendMetaJSON(string(v))
					default:
						// 非预期类型，直接认为存在
					}
					result[peerUUID] = true
				}
				return result, nil
			}
		}
		// Case B: 缓存未命中 (Miss) -> Exists 返回 0
		// 代码继续往下走，去查数据库
	}

	// ==================== 2. 缓存未命中，回源查询 MySQL ====================
	var relations []model.UserRelation
	err = r.db.WithContext(ctx).
		Where("user_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, 0).
		Find(&relations).Error

	if err != nil {
		return nil, WrapDBError(err)
	}

	// ==================== 3. 重建缓存 (Hash) ====================
	r.rebuildFriendCacheAsync(ctx, userUUID, relations)

	// ==================== 4. 构建返回结果 ====================
	// 构建返回结果
	friendSet := make(map[string]bool, len(relations))
	for _, relation := range relations {
		friendSet[relation.PeerUuid] = true
	}
	result := make(map[string]bool, len(peerUUIDs))
	for _, peerUUID := range peerUUIDs {
		result[peerUUID] = friendSet[peerUUID]
	}

	return result, nil
}

// invalidateFriendCacheAsync 异步更新双方的好友缓存
// 在单个协程中同时处理 userUUID 和 friendUUID 的缓存更新
func (r *friendRepositoryImpl) invalidateFriendCacheAsync(ctx context.Context, userUUID, friendUUID string) {
	async.RunSafe(ctx, func(runCtx context.Context) {
		pairs := []struct{ userKey, newFriend string }{
			{rediskey.FriendRelationKey(userUUID), friendUUID},
			{rediskey.FriendRelationKey(friendUUID), userUUID},
		}
		metaJSON := buildFriendMetaJSON("", "", "", time.Now().UnixMilli())
		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		luaScript := redis.NewScript(luaInsertFriendMetaIfExists)

		for _, pair := range pairs {
			_, err := luaScript.Run(runCtx, r.redisClient,
				[]string{pair.userKey},
				pair.newFriend,
				metaJSON,
				expireSeconds,
			).Result()
			if err != nil && err != redis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, pair.userKey).Err()
					continue
				}
				LogRedisError(runCtx, err)
			}
		}
	}, 0)
}

// removeFriendCacheAsync 异步删除好友缓存（单向）
// 仅在缓存存在时做增量更新，避免过期后写入不完整 Hash
func (r *friendRepositoryImpl) removeFriendCacheAsync(ctx context.Context, userUUID, friendUUID string) {
	cacheKey := rediskey.FriendRelationKey(userUUID)

	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := redis.NewScript(luaRemoveFriendMetaIfExists)
		placeholderJSON := buildFriendMetaJSON("", "", "", 0)
		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			friendUUID,
			placeholderJSON,
			expireSeconds,
		).Result()

		if err != nil && err != redis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, 0)
}

// rebuildFriendCacheAsync 异步重建好友关系缓存（Hash）
func (r *friendRepositoryImpl) rebuildFriendCacheAsync(ctx context.Context, userUUID string, relations []model.UserRelation) {
	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		pipe.Del(runCtx, cacheKey)

		if len(relations) == 0 {
			pipe.HSet(runCtx, cacheKey, "__EMPTY__", buildFriendMetaJSON("", "", "", 0))
			pipe.Expire(runCtx, cacheKey, rediskey.FriendRelationEmptyTTL)
		} else {
			fields := make(map[string]interface{}, len(relations))
			for _, relation := range relations {
				if relation.PeerUuid == "" {
					continue
				}
				fields[relation.PeerUuid] = buildFriendMetaJSON(
					relation.Remark,
					relation.GroupTag,
					relation.Source,
					relation.UpdatedAt.UnixMilli(),
				)
			}
			if len(fields) > 0 {
				pipe.HSet(runCtx, cacheKey, fields)
			}
			pipe.Expire(runCtx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
		}

		if _, err := pipe.Exec(runCtx); err != nil && err != redis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, 0)
}

// updateFriendMetaCacheAsync 异步更新好友元数据缓存（单向）
func (r *friendRepositoryImpl) updateFriendMetaCacheAsync(ctx context.Context, userUUID string, relation *model.UserRelation) {
	if relation == nil || relation.PeerUuid == "" {
		return
	}
	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		metaJSON := buildFriendMetaJSON(
			relation.Remark,
			relation.GroupTag,
			relation.Source,
			relation.UpdatedAt.UnixMilli(),
		)
		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		luaScript := redis.NewScript(luaUpsertFriendMetaIfExists)
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			relation.PeerUuid,
			metaJSON,
			expireSeconds,
		).Result()
		if err != nil && err != redis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, 0)
}

// updateFriendRemarkCacheAsync 异步更新好友备注缓存（单向）
// 若缓存存在但字段缺失，则回源 MySQL 补全后写入
func (r *friendRepositoryImpl) updateFriendRemarkCacheAsync(ctx context.Context, userUUID, friendUUID, remark string, updatedAt int64) {
	if friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		existsCmd := pipe.Exists(runCtx, cacheKey)
		metaCmd := pipe.HGet(runCtx, cacheKey, friendUUID)
		_, err := pipe.Exec(runCtx)

		if err != nil && err != redis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
			return
		}

		if existsCmd.Val() == 0 {
			return
		}

		if metaCmd.Err() == nil {
			meta, err := parseFriendMetaJSON(metaCmd.Val())
			if err != nil {
				return
			}
			meta.Remark = remark
			meta.UpdatedAt = updatedAt
			metaJSON := buildFriendMetaJSON(meta.Remark, meta.GroupTag, meta.Source, meta.UpdatedAt)
			expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
			luaScript := redis.NewScript(luaUpsertFriendMetaIfExists)
			_, err = luaScript.Run(runCtx, r.redisClient,
				[]string{cacheKey},
				friendUUID,
				metaJSON,
				expireSeconds,
			).Result()
			if err != nil && err != redis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, cacheKey).Err()
					return
				}
				LogRedisError(runCtx, err)
			}
			return
		}

		if metaCmd.Err() != redis.Nil {
			if isRedisWrongType(metaCmd.Err()) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
			} else {
				LogRedisError(runCtx, metaCmd.Err())
			}
			return
		}

		// 缓存存在但字段缺失，回源补全
		var relation model.UserRelation
		if err := r.db.WithContext(runCtx).
			Select("peer_uuid", "remark", "group_tag", "source", "updated_at").
			Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
			First(&relation).Error; err != nil {
			return
		}

		r.updateFriendMetaCacheAsync(runCtx, userUUID, &relation)
	}, 0)
}

// updateFriendTagCacheAsync 异步更新好友标签缓存（单向）
// 若缓存存在但字段缺失，则回源 MySQL 补全后写入
func (r *friendRepositoryImpl) updateFriendTagCacheAsync(ctx context.Context, userUUID, friendUUID, groupTag string, updatedAt int64) {
	if friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		existsCmd := pipe.Exists(runCtx, cacheKey)
		metaCmd := pipe.HGet(runCtx, cacheKey, friendUUID)
		_, err := pipe.Exec(runCtx)

		if err != nil && err != redis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
			return
		}

		if existsCmd.Val() == 0 {
			return
		}

		if metaCmd.Err() == nil {
			meta, err := parseFriendMetaJSON(metaCmd.Val())
			if err != nil {
				return
			}
			meta.GroupTag = groupTag
			meta.UpdatedAt = updatedAt
			metaJSON := buildFriendMetaJSON(meta.Remark, meta.GroupTag, meta.Source, meta.UpdatedAt)
			expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
			luaScript := redis.NewScript(luaUpsertFriendMetaIfExists)
			_, err = luaScript.Run(runCtx, r.redisClient,
				[]string{cacheKey},
				friendUUID,
				metaJSON,
				expireSeconds,
			).Result()
			if err != nil && err != redis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, cacheKey).Err()
					return
				}
				LogRedisError(runCtx, err)
			}
			return
		}

		if metaCmd.Err() != redis.Nil {
			if isRedisWrongType(metaCmd.Err()) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
			} else {
				LogRedisError(runCtx, metaCmd.Err())
			}
			return
		}

		// 缓存存在但字段缺失，回源补全
		var relation model.UserRelation
		if err := r.db.WithContext(runCtx).
			Select("peer_uuid", "remark", "group_tag", "source", "updated_at").
			Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
			First(&relation).Error; err != nil {
			return
		}

		r.updateFriendMetaCacheAsync(runCtx, userUUID, &relation)
	}, 0)
}
