package repository

import (
	"context"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/redisbloom"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	goredis "github.com/redis/go-redis/v9"
)

// groupUUIDBloomFilter 维护 groups.uuid 的存在性索引。
//
// 群规模通常小于用户规模，容量按两百万级预留；容量不足时需要新建过滤器并全量回填。
var groupUUIDBloomFilter = redisbloom.Filter{
	Key:       rediskey.GroupUUIDBloomKey,
	ErrorRate: 0.001,
	Capacity:  2_000_000,
}

// InitGroupUUIDBloom 在读侧构造阶段初始化 group_uuid Bloom Filter。
//
// 初始化失败不阻断服务启动；读路径会继续回源 DB，新增群写路径会在事务前再次写 Bloom。
func InitGroupUUIDBloom(ctx context.Context, redisClient *goredis.Client) {
	if redisClient == nil {
		return
	}
	initCtx, cancel := context.WithTimeout(ctx, async.AsyncRedisTimeout)
	defer cancel()
	if err := groupUUIDBloomFilter.Ensure(initCtx, redisClient); err != nil {
		repoerr.LogRedisError(initCtx, err)
	}
}

// GroupUUIDMayExist 判断 group_uuid 是否可能存在。
//
// 返回 false 表示 Bloom 明确判定该群不存在；RedisBloom 不可用时返回 true，让 DB 兜底。
func GroupUUIDMayExist(ctx context.Context, redisClient *goredis.Client, groupUUID string) bool {
	if redisClient == nil || groupUUID == "" {
		return true
	}
	exists, usable, err := groupUUIDBloomFilter.Exists(ctx, redisClient, groupUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	}
	if !usable {
		return true
	}
	return exists
}

// FilterGroupUUIDsByBloom 用 BF.MEXISTS 过滤批量群资料查询中的确定不存在 UUID。
func FilterGroupUUIDsByBloom(ctx context.Context, redisClient *goredis.Client, groupUUIDs []string) []string {
	if redisClient == nil || len(groupUUIDs) == 0 {
		return groupUUIDs
	}
	exists, usable, err := groupUUIDBloomFilter.MExists(ctx, redisClient, groupUUIDs)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
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

// EnsureGroupUUIDInBloom 在创建群 DB 记录前先写入 group_uuid Bloom Filter。
//
// 先写 Bloom 再写 DB 的代价是 DB 失败时可能留下 false positive，但 false positive
// 只会多一次 DB 查询；如果反过来 DB 成功而 Bloom 写失败，则会误判真实群不存在。
func EnsureGroupUUIDInBloom(ctx context.Context, redisClient *goredis.Client, groupUUID string) error {
	if redisClient == nil || groupUUID == "" {
		return nil
	}
	return groupUUIDBloomFilter.Add(ctx, redisClient, groupUUID)
}

// AddGroupUUIDToBloomBestEffort 用于读回源命中或缓存投影时的自愈补写。
func AddGroupUUIDToBloomBestEffort(ctx context.Context, redisClient *goredis.Client, groupUUID string) {
	if err := EnsureGroupUUIDInBloom(ctx, redisClient, groupUUID); err != nil {
		repoerr.LogRedisError(ctx, err)
	}
}

// AddGroupUUIDsToBloomAsync 批量补写 DB 命中的 group_uuid。
func AddGroupUUIDsToBloomAsync(ctx context.Context, redisClient *goredis.Client, groupUUIDs []string) {
	if redisClient == nil || len(groupUUIDs) == 0 {
		return
	}
	items := make([]string, 0, len(groupUUIDs))
	seen := make(map[string]struct{}, len(groupUUIDs))
	for _, groupUUID := range groupUUIDs {
		if groupUUID == "" {
			continue
		}
		if _, exists := seen[groupUUID]; exists {
			continue
		}
		seen[groupUUID] = struct{}{}
		items = append(items, groupUUID)
	}
	if len(items) == 0 {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := groupUUIDBloomFilter.MAdd(runCtx, redisClient, items); err != nil {
			repoerr.LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}
