package cache

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	goredis "github.com/redis/go-redis/v9"
)

// GetGroupInfo 按群 UUID 查询有效群资料。
//
// 缓存读取与穿透防护设计：
//  1. 首先尝试从 Redis 主缓存 `group:info:{group_uuid}` 读取（格式为 schema|version|JSON）；
//  2. 若缓存命中且状态正常（status=0），直接反序列化返回；
//  3. 若缓存未命中，调用 RedisBloom 布隆过滤器判定该群 UUID 是否可能存在；
//     - 若布隆过滤器判定“绝对不存在”，则异步写入空值占位缓存（TTL=30s），立即返回 ErrRecordNotFound，阻断穿透；
//  4. 若布隆过滤器判定“可能存在”，回源 MySQL 读取权威快照；
//     - 若 MySQL 查无此记录，同样异步写入空值缓存防穿透；
//     - 若 MySQL 命中，则将 UUID 补入布隆过滤器（自愈），并提交异步对账意图（ScheduleGroupRepair），
//     由后台任务按带版本保护的 Lua 脚本重新投影，请求线程绝不同步晚写 Redis。
func (r *Reader) GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if r == nil || r.store == nil || groupUUID == "" {
		return nil, repoerr.ErrRecordNotFound
	}
	// 1. 读取主缓存
	groupInfo, cacheHit, err := r.getGroupInfoFromCache(ctx, groupUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	} else if cacheHit {
		if groupInfo == nil || groupInfo.Status != repository.GroupStatusNormal {
			return nil, repoerr.ErrRecordNotFound
		}
		return groupInfo, nil
	}

	// 2. 布隆过滤器防穿透前置检查
	if !repository.GroupUUIDMayExist(ctx, r.redisClient, groupUUID) {
		r.setGroupInfoEmptyCacheAsync(ctx, groupUUID)
		return nil, repoerr.ErrRecordNotFound
	}

	// 3. 回源 MySQL 权威快照
	groupInfo, err = r.store.LoadGroupInfoFromDB(ctx, groupUUID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			r.setGroupInfoEmptyCacheAsync(ctx, groupUUID)
		}
		return nil, err
	}

	// 4. 布隆补填与异步调度对账重建
	repository.AddGroupUUIDToBloomBestEffort(ctx, r.redisClient, groupInfo.Uuid)
	r.scheduleGroupRepair(ctx, groupUUID)
	return groupInfo, nil
}

// ensureGroupNormal 确认群当前仍处于可读的正常状态（Status=0 且未解散/停用）。
//
// 专供群成员列表、入群申请列表等读链路在读取前进行前置校验，优先读缓存，miss 时回源。
func (r *Reader) ensureGroupNormal(ctx context.Context, groupUUID string) error {
	if r == nil || r.store == nil || groupUUID == "" {
		return repoerr.ErrRecordNotFound
	}
	groupInfo, cacheHit, err := r.getGroupInfoFromCache(ctx, groupUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	} else if cacheHit {
		if groupInfo == nil {
			return repoerr.ErrRecordNotFound
		}
		if groupInfo.Status == repository.GroupStatusDismissed {
			return repository.ErrGroupDismissed
		}
		if groupInfo.Status != repository.GroupStatusNormal {
			return repoerr.ErrRecordNotFound
		}
		return nil
	}

	if !repository.GroupUUIDMayExist(ctx, r.redisClient, groupUUID) {
		r.setGroupInfoEmptyCacheAsync(ctx, groupUUID)
		return repoerr.ErrRecordNotFound
	}
	_, err = r.store.LoadGroupForRead(ctx, groupUUID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			r.setGroupInfoEmptyCacheAsync(ctx, groupUUID)
		}
		return err
	}
	repository.AddGroupUUIDToBloomBestEffort(ctx, r.redisClient, groupUUID)
	r.scheduleGroupRepair(ctx, groupUUID)
	return nil
}

// loadReadableGroupInfo 读取发送权限检查所需的群状态与全员禁言开关（mute_all）。
//
// 专为 CheckGroupSendPermission 极高频链路优化，仅获取最小必要字段。
func (r *Reader) loadReadableGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	groupInfo, cacheHit, err := r.getGroupInfoFromCache(ctx, groupUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	} else if cacheHit {
		if groupInfo == nil {
			return nil, repoerr.ErrRecordNotFound
		}
		if groupInfo.Status == repository.GroupStatusDismissed {
			return nil, repository.ErrGroupDismissed
		}
		if groupInfo.Status != repository.GroupStatusNormal {
			return nil, repoerr.ErrRecordNotFound
		}
		return groupInfo, nil
	}

	if !repository.GroupUUIDMayExist(ctx, r.redisClient, groupUUID) {
		return nil, repoerr.ErrRecordNotFound
	}
	group, err := r.store.LoadGroupForRead(ctx, groupUUID)
	if err != nil {
		return nil, err
	}
	repository.AddGroupUUIDToBloomBestEffort(ctx, r.redisClient, groupUUID)
	return group, nil
}

// getGroupInfoFromCache 从 Redis 读取 group:info:{group_uuid} 主缓存 String 并严格校验版本前缀。
//
// 编码格式约定：`SchemaVersion|ProjectionVersion|JSON`，例如 `2|15|{"name":"交流群",...}`。
// 返回值：
//   - cacheHit=true：缓存存在且未损坏；
//   - empty=true（通过 decode 返回）：命中了 `__NOT_FOUND__` 空值占位缓存。
func (r *Reader) getGroupInfoFromCache(ctx context.Context, groupUUID string) (*model.GroupInfo, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.GroupInfoKey(groupUUID)
	raw, err := r.redisClient.Get(ctx, cacheKey).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) || cachex.IsRedisWrongType(err) {
			return nil, false, nil
		}
		return nil, false, repoerr.WrapRedisError(err)
	}

	// 严格解码：校验 Schema 版本（必须为 2）、ProjectionVersion（正整数）和 JSON 实体
	entry, projectionVersion, empty, decodeErr := repository.DecodeGroupInfoCacheValue(raw)
	if decodeErr != nil {
		return nil, false, nil
	}
	if empty {
		return nil, true, nil // 命中空值缓存
	}
	if entry == nil || entry.GroupUUID != groupUUID {
		return nil, false, nil
	}
	// 1% 概率随机滑动续期（带 Jitter TTL 抖动，防止大批量缓存同时雪崩过期）
	if cachex.Chance(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, cachex.JitterTTL(rediskey.GroupInfoTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			repoerr.LogRedisError(ctx, expireErr)
		}
	}
	return repository.BuildGroupInfoFromCache(entry, projectionVersion), true, nil
}

// setGroupInfoEmptyCacheAsync 异步写入群资料空值占位缓存（SETNX EX 30s）。
//
// 用于在确认群不存在时短时间缓存空值，阻断针对不存在群的重复高并发请求穿透到数据库。
func (r *Reader) setGroupInfoEmptyCacheAsync(ctx context.Context, groupUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		cacheKey := rediskey.GroupInfoKey(groupUUID)
		if err := r.redisClient.SetNX(runCtx, cacheKey, repository.GroupInfoEmptyValue, rediskey.GroupInfoEmptyTTL).Err(); err != nil {
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}
