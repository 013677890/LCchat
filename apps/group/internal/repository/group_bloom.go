package repository

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/async"
)

// initGroupUUIDBloom 在仓储构造阶段初始化 group_uuid Bloom Filter。
//
// 初始化失败不阻断服务启动；读路径会继续回源 DB，新增群写路径会在事务前再次写 Bloom，
// 写 Bloom 失败时拒绝写 DB，防止新群记录出现 false negative。
func (r *groupRepositoryImpl) initGroupUUIDBloom(ctx context.Context) {
	if r == nil || r.redisClient == nil {
		return
	}
	initCtx, cancel := context.WithTimeout(ctx, async.AsyncRedisTimeout)
	defer cancel()
	if err := groupUUIDBloomFilter.Ensure(initCtx, r.redisClient); err != nil {
		LogRedisError(initCtx, err)
	}
}

// groupUUIDMayExist 判断 group_uuid 是否可能存在。
//
// 返回 false 表示 Bloom 明确判定该群不存在，可以直接返回业务不存在；RedisBloom 不可用时
// 返回 true，让原有 DB 权威链路兜底。
func (r *groupRepositoryImpl) groupUUIDMayExist(ctx context.Context, groupUUID string) bool {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return true
	}
	exists, usable, err := groupUUIDBloomFilter.Exists(ctx, r.redisClient, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	}
	if !usable {
		return true
	}
	return exists
}

// filterGroupUUIDsByBloom 用 BF.MEXISTS 过滤批量群资料查询中的确定不存在 UUID。
//
// 只在 Bloom 可用且返回数量匹配时生效，避免 Redis 异常导致批量查询误丢结果。
func (r *groupRepositoryImpl) filterGroupUUIDsByBloom(ctx context.Context, groupUUIDs []string) []string {
	if r == nil || r.redisClient == nil || len(groupUUIDs) == 0 {
		return groupUUIDs
	}
	exists, usable, err := groupUUIDBloomFilter.MExists(ctx, r.redisClient, groupUUIDs)
	if err != nil {
		LogRedisError(ctx, err)
	}
	if !usable || len(exists) != len(groupUUIDs) {
		return groupUUIDs
	}
	filtered := make([]string, 0, len(groupUUIDs))
	for i, groupUUID := range groupUUIDs {
		if exists[i] {
			filtered = append(filtered, groupUUID)
		}
	}
	return filtered
}

// ensureGroupUUIDInBloom 在创建群 DB 记录前先写入 group_uuid Bloom Filter。
//
// 先写 Bloom 再写 DB 的代价是 DB 失败时可能留下 false positive，但 false positive 只会多一次
// DB 查询；如果反过来 DB 成功而 Bloom 写失败，则会误判真实群不存在。
func (r *groupRepositoryImpl) ensureGroupUUIDInBloom(ctx context.Context, groupUUID string) error {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil
	}
	return groupUUIDBloomFilter.Add(ctx, r.redisClient, groupUUID)
}

// addGroupUUIDToBloomBestEffort 用于读回源命中或缓存投影时的自愈补写。
//
// 这些场景不是创建事实的入口，补写失败只记录日志，不影响当前读结果或投影主流程。
func (r *groupRepositoryImpl) addGroupUUIDToBloomBestEffort(ctx context.Context, groupUUID string) {
	if err := r.ensureGroupUUIDInBloom(ctx, groupUUID); err != nil {
		LogRedisError(ctx, err)
	}
}

// addGroupUUIDsToBloomAsync 批量补写 DB 命中的 group_uuid。
//
// 用于批量查询后的渐进自愈；失败不影响已经从 DB 读到的群资料。
func (r *groupRepositoryImpl) addGroupUUIDsToBloomAsync(ctx context.Context, groupUUIDs []string) {
	if r == nil || r.redisClient == nil || len(groupUUIDs) == 0 {
		return
	}
	items := make([]string, 0, len(groupUUIDs))
	seen := make(map[string]struct{}, len(groupUUIDs))
	for _, groupUUID := range groupUUIDs {
		if groupUUID == "" {
			continue
		}
		if _, ok := seen[groupUUID]; ok {
			continue
		}
		seen[groupUUID] = struct{}{}
		items = append(items, groupUUID)
	}

	if len(items) == 0 {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := groupUUIDBloomFilter.MAdd(runCtx, r.redisClient, items); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}
