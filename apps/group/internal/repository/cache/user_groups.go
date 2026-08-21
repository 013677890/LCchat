package cache

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
)

// ListUserGroups 获取当前用户所属的有效群列表。
//
// 架构设计与一致性保障机制：
//  1. 尝试从 Redis 反向索引读取：执行 Lua 脚本原子读取 `user:group:list:{userUUID}`（ZSet）和 `user:group:version:{userUUID}`（Hash）；
//  2. 栅栏校验（__READY__ 标记）：只有全量对账过的缓存才会标记 `__READY__=1`，单条 Kafka 增量事件绝不立旗，防止局部数据被当作全量；
//  3. 交叉版本校验（Cross-Version Validation）：遍历每个群点查 `group:info`，若 `group.CacheVersion < reference.Version`，
//     说明群资料滞后于反向索引，立即判定缓存失效回源 MySQL，防止返回“半新半旧”的群数据；
//  4. 读命中后的异步租约对账（Audit After Hit）：即使 100% 命中缓存，也会通过分布式租约（SETNX 锁）低频触发后台对账，
//     自愈消除“用户被踢但缓存残留”等单群扫描无法发现的幽灵反向索引数据，同时读请求立即返回，零额外网络延迟。
func (r *Reader) ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	if r == nil || r.store == nil || userUUID == "" {
		return []*model.GroupInfo{}, nil
	}
	// 1. 尝试从 Redis 反向索引与群主资料缓存中组合读取
	groups, cacheHit, err := r.getUserGroupsFromCache(ctx, userUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	} else if cacheHit {
		// 读命中后尝试抢占租约发起低频异步对账（Audit After Hit）
		r.scheduleUserAudit(ctx, userUUID)
		return groups, nil
	}

	// 2. 缓存 miss 时回源 MySQL 权威快照
	groups, err = r.store.LoadUserGroupsFromDB(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	// 3. 调度后台异步对账任务重建用户群反向索引（立旗 __READY__=1）
	r.scheduleUserRepair(ctx, userUUID)
	return groups, nil
}

// getUserGroupsFromCache 读取 READY 状态的用户群列表，并与各个 group:info 进行版本交叉比对。
//
// 返回值：
//   - cacheHit=true：反向索引 READY 且所有关联群的主资料版本均达到或超过索引版本；
//   - cacheHit=false：反向索引未就绪、任意群资料丢失或资料版本滞后，需回源 MySQL。
func (r *Reader) getUserGroupsFromCache(ctx context.Context, userUUID string) ([]*model.GroupInfo, bool, error) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return nil, false, nil
	}
	// 1. 调用 Lua 脚本原子读取就绪的 ZSet 群列表及 Hash 中记录的单群投影版本
	references, cacheHit, err := repository.ReadVersionedUserGroups(ctx, r.redisClient, userUUID, cachex.Chance(0.01))
	if err != nil || !cacheHit {
		return nil, false, err
	}
	// 处理空群列表哨兵（用户未加入任何群）
	if len(references) == 1 && references[0].GroupUUID == repository.UserGroupsEmptyValue {
		return []*model.GroupInfo{}, true, nil
	}
	if len(references) == 0 {
		return nil, false, nil
	}

	groups := make([]*model.GroupInfo, 0, len(references))
	// 2. 遍历每个群，点查 group:info 并进行版本栅栏比对
	for _, reference := range references {
		group, groupCacheHit, cacheErr := r.getGroupInfoFromCache(ctx, reference.GroupUUID)
		if cacheErr != nil {
			return nil, false, cacheErr
		}
		// 核心拦截规则：
		// - 资料缓存未命中或解码失败
		// - 群已处于非正常状态（如已解散）
		// - 群主资料版本滞后于反向索引版本（group.CacheVersion < reference.Version）
		// 只要有任意一个群不满足，立即判定整体缓存失效回源！
		if !groupCacheHit ||
			group == nil ||
			group.Status != repository.GroupStatusNormal ||
			group.CacheVersion < reference.Version {
			return nil, false, nil
		}
		groups = append(groups, group)
	}
	// 3. 按群更新时间倒序进行确定性内存排序
	repository.SortGroupInfos(groups)
	return groups, true, nil
}
