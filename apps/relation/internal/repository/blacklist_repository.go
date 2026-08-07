package repository

import (
	"context"
	"errors"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type blacklistRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// NewBlacklistRepository 创建黑名单仓储实例。
//
// 黑名单是权限判断链路上的高频数据，因此这里恢复为“Redis ZSet + MySQL”双层结构：
// Redis 负责快速判断与列表分页，MySQL 则继续作为最终权威来源，负责兜底与重建缓存。
func NewBlacklistRepository(db *gorm.DB, redisClient *goredis.Client) IBlacklistRepository {
	return &blacklistRepositoryImpl{db: db, redisClient: redisClient}
}

// AddBlacklist 将 targetUUID 加入 userUUID 的黑名单。
//
// 该方法会保留“拉黑前是否为好友”的信息：
//   - status=1 表示拉黑前存在好友关系，取消拉黑时需要恢复为好友；
//   - status=3 表示拉黑前不是好友，取消拉黑时恢复为删除状态。
//
// 写入使用 Upsert，保证重复拉黑或软删除记录恢复时仍然幂等。
func (r *blacklistRepositoryImpl) AddBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status := int8(3)

		// 先查历史关系，决定取消拉黑时应恢复为好友还是删除状态。
		var existing model.UserRelation
		if err := tx.Unscoped().
			Select("status").
			Where("user_uuid = ? AND peer_uuid = ?", userUUID, targetUUID).
			First(&existing).Error; err != nil {
			dbErr := repoerr.WrapDBError(err)
			if !errors.Is(dbErr, repoerr.ErrRecordNotFound) {
				return dbErr
			}
		} else if existing.Status == 0 || existing.Status == 1 || existing.Status == 2 {
			// 曾经是好友或删除好友，都按“拉黑前为好友”处理，便于取消拉黑时恢复关系。
			status = 1
		}

		// relation 里把本次拉黑时刻写进 blacklisted_at，供后续列表排序和缓存 score 使用。
		relation := &model.UserRelation{
			UserUuid:      userUUID,
			PeerUuid:      targetUUID,
			Status:        status,
			BlacklistedAt: &now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		// 使用唯一键 (user_uuid, peer_uuid) 做 Upsert，避免并发重复拉黑产生冲突。
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":         status,
				"deleted_at":     nil,
				"blacklisted_at": now,
				"updated_at":     now,
			}),
		}).Create(relation).Error
	})

	if err != nil {
		return repoerr.WrapDBError(err)
	}

	// 黑名单 ZSet 与好友 Hash 都只维护当前用户视角，因此这里只更新单侧缓存。
	// 先补黑名单缓存，再删好友缓存，保证后续权限判断优先命中新黑名单状态。
	r.updateBlacklistCacheAsync(ctx, userUUID, targetUUID, now.UnixMilli())
	r.removeFriendCacheAsync(ctx, userUUID, targetUUID)
	return nil
}

// RemoveBlacklist 将 targetUUID 从 userUUID 的黑名单中移除。
//
// 根据拉黑状态恢复关系：status=1 恢复为好友；status=3 恢复为已删除的非好友关系。
// 这样可以避免“非好友取消拉黑后错误变成好友”。
func (r *blacklistRepositoryImpl) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	if userUUID == "" || targetUUID == "" {
		return repoerr.ErrRecordNotFound
	}

	var relation model.UserRelation

	// 先用 Unscoped 查出当前黑名单关系的历史 status，决定取消拉黑后该恢复成什么状态。
	if err := r.db.WithContext(ctx).
		Unscoped().
		Select("status").
		Where("user_uuid = ? AND peer_uuid = ? AND status IN ?", userUUID, targetUUID, []int{1, 3}).
		First(&relation).Error; err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return repoerr.ErrRecordNotFound
		}
		return repoerr.WrapDBError(err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"blacklisted_at": nil,
		"updated_at":     now,
	}
	restoreFriend := relation.Status == 1

	// status=1 代表拉黑前是好友；status=3 代表拉黑前本就不是好友。
	if restoreFriend {
		// 拉黑前是好友：取消拉黑后恢复好友关系。
		updates["status"] = 0
		updates["deleted_at"] = nil
	} else {
		// 拉黑前不是好友：取消拉黑后恢复为软删除关系，仅保留历史记录。
		updates["status"] = 2
		updates["deleted_at"] = gorm.DeletedAt{Time: now, Valid: true}
	}

	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, targetUUID).
		Updates(updates)
	if result.Error != nil {
		return repoerr.WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repoerr.ErrRecordNotFound
	}

	// 先删黑名单缓存，再根据恢复结果选择补回好友缓存或继续维持“非好友”状态。
	r.removeBlacklistCacheAsync(ctx, userUUID, targetUUID)
	if restoreFriend {
		r.restoreFriendCacheAsync(ctx, userUUID, targetUUID)
	} else {
		r.removeFriendCacheAsync(ctx, userUUID, targetUUID)
	}
	return nil
}

// GetBlacklistList 分页查询 userUUID 的黑名单列表。
//
// 列表缓存使用 ZSet，member=peer_uuid，score=blacklisted_at 毫秒时间戳；这样既能高效判断
// 是否拉黑，也能天然支持按拉黑时间倒序分页。
func (r *blacklistRepositoryImpl) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	_, pageSize, offset := normalizePage(page, pageSize)

	if r.redisClient != nil {
		cacheKey := rediskey.BlacklistRelationKey(userUUID)
		pipe := r.redisClient.Pipeline()

		// exists/count/range/emptyScore 一起查，尽量一次 RTT 拿到分页和占位符信息。
		existsCmd := pipe.Exists(ctx, cacheKey)
		countCmd := pipe.ZCard(ctx, cacheKey)
		rangeCmd := pipe.ZRevRangeWithScores(ctx, cacheKey, int64(offset), int64(offset+pageSize-1))
		emptyScoreCmd := pipe.ZScore(ctx, cacheKey, "__EMPTY__")
		if cachex.Chance(0.01) {
			pipe.Expire(ctx, cacheKey, cachex.JitterTTL(rediskey.BlacklistTTL))
		}

		_, err := pipe.Exec(ctx)
		if err == nil {
			if existsCmd.Val() > 0 {
				total := countCmd.Val()

				// 只有 __EMPTY__ 占位时，直接返回空列表，不再回源数据库。
				if total == 1 && emptyScoreCmd.Err() == nil {
					return []*model.UserRelation{}, 0, nil
				}

				relations := make([]*model.UserRelation, 0, len(rangeCmd.Val()))
				for _, z := range rangeCmd.Val() {
					// 跳过空 member 和 __EMPTY__ 占位，只保留真实黑名单用户。
					member, ok := z.Member.(string)
					if !ok || member == "" || member == "__EMPTY__" {
						continue
					}
					blacklistedAt := time.UnixMilli(int64(z.Score))
					relations = append(relations, &model.UserRelation{
						UserUuid:      userUUID,
						PeerUuid:      member,
						Status:        1,
						BlacklistedAt: &blacklistedAt,
					})
				}

				realTotal := total

				// 若存在占位符，需要把它从总数里扣掉再返回给上层分页。
				if emptyScoreCmd.Err() == nil && realTotal > 0 {
					realTotal--
				}
				return relations, realTotal, nil
			}
		} else if err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(ctx, cacheKey).Err()
			} else {
				repoerr.LogRedisError(ctx, err)
			}
		}
	}

	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, []int{1, 3})

	var total int64

	// 缓存 miss 时先走数据库 count，再做分页查询，最后异步重建整份 ZSet。
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, repoerr.WrapDBError(err)
	}

	var relations []*model.UserRelation
	if err := query.
		Order("blacklisted_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&relations).Error; err != nil {
		return nil, 0, repoerr.WrapDBError(err)
	}

	// 列表缓存必须从全量数据重建，避免把当前分页误写成完整黑名单集合。
	r.rebuildBlacklistCacheFromDBAsync(ctx, userUUID)
	return relations, total, nil
}

// IsBlocked 判断 userUUID 是否已拉黑 targetUUID。
//
// 该方法是消息权限校验和好友申请校验的高频路径，因此优先命中 Redis；缓存缺失时再回源
// 数据库，并异步重建整份黑名单集合，避免后续查询继续穿透。
func (r *blacklistRepositoryImpl) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	if r.redisClient != nil {
		cacheKey := rediskey.BlacklistRelationKey(userUUID)
		pipe := r.redisClient.Pipeline()

		// exists + zscore 一起执行，用于区分“key 不存在”和“目标不在集合里”。
		existsCmd := pipe.Exists(ctx, cacheKey)
		scoreCmd := pipe.ZScore(ctx, cacheKey, targetUUID)
		if cachex.Chance(0.01) {
			pipe.Expire(ctx, cacheKey, cachex.JitterTTL(rediskey.BlacklistTTL))
		}

		_, err := pipe.Exec(ctx)
		if err == nil {
			if existsCmd.Val() > 0 {
				if scoreCmd.Err() == nil {
					return true, nil
				}
				if scoreCmd.Err() == goredis.Nil {
					return false, nil
				}
				if cachex.IsRedisWrongType(scoreCmd.Err()) {
					_ = r.redisClient.Del(ctx, cacheKey).Err()
				} else {
					repoerr.LogRedisError(ctx, scoreCmd.Err())
				}
			}
		} else if err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(ctx, cacheKey).Err()
			} else {
				repoerr.LogRedisError(ctx, err)
			}
		}
	}

	var count int64

	// 缓存无法给出结论时，最终仍以数据库里 status IN (1,3) 的有效黑名单关系为准。
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, targetUUID, []int{1, 3}).
		Count(&count).Error; err != nil {
		return false, repoerr.WrapDBError(err)
	}

	r.rebuildBlacklistCacheFromDBAsync(ctx, userUUID)
	return count > 0, nil
}

// GetBlacklistRelation 查询一条有效黑名单关系。
//
// 当关系不存在时返回 repoerr.ErrRecordNotFound，调用方可据此映射为“不在黑名单中”的业务错误。
func (r *blacklistRepositoryImpl) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	if err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, targetUUID, []int{1, 3}).
		First(&relation).Error; err != nil {
		if errors.Is(repoerr.WrapDBError(err), repoerr.ErrRecordNotFound) {
			return nil, repoerr.ErrRecordNotFound
		}
		return nil, repoerr.WrapDBError(err)
	}
	return &relation, nil
}

// rebuildBlacklistCacheFromDBAsync 异步从数据库重建整份黑名单缓存。
//
// 黑名单点查只靠单条结果无法推断整份集合是否完整，因此缓存 miss 后统一回源全量列表再
// 重建，避免把局部结果误写成最终事实。
func (r *blacklistRepositoryImpl) rebuildBlacklistCacheFromDBAsync(ctx context.Context, userUUID string) {
	if r.redisClient == nil || userUUID == "" {
		return
	}

	async.RunSafe(ctx, func(runCtx context.Context) {
		var relations []model.UserRelation
		err := r.db.WithContext(runCtx).
			Select("peer_uuid", "blacklisted_at", "updated_at").
			Where("user_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, []int{1, 3}).
			Find(&relations).Error
		if err != nil {
			return
		}
		r.rebuildBlacklistCacheAsync(runCtx, userUUID, relations)
	}, async.AsyncDBTimeout)
}

// rebuildBlacklistCacheAsync 异步写入整份黑名单 ZSet。
//
// 若当前用户没有任何黑名单成员，则写入 __EMPTY__ 占位符，防止高频空查询持续穿透。
func (r *blacklistRepositoryImpl) rebuildBlacklistCacheAsync(ctx context.Context, userUUID string, relations []model.UserRelation) {
	if r.redisClient == nil || userUUID == "" {
		return
	}

	cacheKey := rediskey.BlacklistRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		pipe.Del(runCtx, cacheKey)

		if len(relations) == 0 {
			pipe.ZAdd(runCtx, cacheKey, goredis.Z{Score: 0, Member: "__EMPTY__"})
			pipe.Expire(runCtx, cacheKey, rediskey.BlacklistEmptyTTL)
		} else {
			members := make([]goredis.Z, 0, len(relations))
			for _, relation := range relations {
				if relation.PeerUuid == "" {
					continue
				}
				blacklistedAt := relation.UpdatedAt
				if relation.BlacklistedAt != nil {
					blacklistedAt = *relation.BlacklistedAt
				}
				members = append(members, goredis.Z{
					Score:  float64(blacklistedAt.UnixMilli()),
					Member: relation.PeerUuid,
				})
			}

			if len(members) > 0 {
				pipe.ZAdd(runCtx, cacheKey, members...)
			}

			// 非空黑名单集合使用常规 TTL，热点 key 后续靠读路径小概率续期维持。
			pipe.Expire(runCtx, cacheKey, cachex.JitterTTL(rediskey.BlacklistTTL))
		}

		if _, err := pipe.Exec(runCtx); err != nil && err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisPipelineTimeout)
}

// updateBlacklistCacheAsync 异步把单个用户增量加入黑名单缓存。
//
// 与好友缓存相同，这里只在 key 已存在时做增量写入；如果缓存整体已过期，则交给下一次
// 读路径统一全量回填，避免产生不完整集合。
func (r *blacklistRepositoryImpl) updateBlacklistCacheAsync(ctx context.Context, userUUID, targetUUID string, blockedAt int64) {
	if r.redisClient == nil || userUUID == "" || targetUUID == "" {
		return
	}

	cacheKey := rediskey.BlacklistRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaAddBlacklistIfExists)
		expireSeconds := int(cachex.JitterTTL(rediskey.BlacklistTTL).Seconds())
		// 仅在 key 已存在时做补丁写入；缓存整体缺失时交给后续读路径统一全量重建。
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			blockedAt,
			targetUUID,
			expireSeconds,
		).Result()

		if err != nil && err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// removeBlacklistCacheAsync 异步移除黑名单缓存中的单个成员。
//
// 若移除后集合为空，则脚本会自动补写 __EMPTY__ 占位，保证后续空查询仍然能命中缓存。
func (r *blacklistRepositoryImpl) removeBlacklistCacheAsync(ctx context.Context, userUUID, targetUUID string) {
	if r.redisClient == nil || userUUID == "" || targetUUID == "" {
		return
	}

	cacheKey := rediskey.BlacklistRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaRemoveBlacklistIfExists)
		expireSeconds := int(cachex.JitterTTL(rediskey.BlacklistTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			targetUUID,
			expireSeconds,
		).Result()

		if err != nil && err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// removeFriendCacheAsync 异步删除当前用户侧的好友缓存 field。
//
// 拉黑后 A->B 不再是好友，但 B->A 关系保持不变，因此这里只清理当前用户这一侧的好友 Hash。
func (r *blacklistRepositoryImpl) removeFriendCacheAsync(ctx context.Context, userUUID, friendUUID string) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaRemoveFriendMetaIfExists)
		placeholderJSON := buildFriendMetaJSON("", "", "", 0)
		expireSeconds := int(cachex.JitterTTL(rediskey.FriendRelationTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			friendUUID,
			placeholderJSON,
			expireSeconds,
		).Result()

		if err != nil && err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// restoreFriendCacheAsync 在“取消拉黑并恢复好友”时补回当前用户侧的好友缓存。
//
// 恢复好友时只要把 field 放回 Hash 即可；若整个 Hash 不存在，则继续让读路径下次全量重建。
func (r *blacklistRepositoryImpl) restoreFriendCacheAsync(ctx context.Context, userUUID, friendUUID string) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaInsertFriendMetaIfExists)
		metaJSON := buildFriendMetaJSON("", "", "", time.Now().UnixMilli())
		expireSeconds := int(cachex.JitterTTL(rediskey.FriendRelationTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			friendUUID,
			metaJSON,
			expireSeconds,
		).Result()

		if err != nil && err != goredis.Nil {
			if cachex.IsRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}
