package cache

import (
	"context"
	"errors"
	"strconv"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	goredis "github.com/redis/go-redis/v9"
)

// ListJoinRequests 获取群待审批入群申请列表。
//
// 权限与缓存设计权衡：
//  1. 群基础状态（是否正常/解散）优先走 Redis 最终一致缓存判断；
//  2. 操作者是否具备管理员/群主权限（EnsureActiveMemberRole）必须回源 MySQL 权威点查，
//     防止依赖异步成员缓存产生“已撤销管理员仍能查看敏感审批列表”的越权风险；
//  3. 待审批数据优先从 Redis Hash `group:join_request:pending:{groupUUID}` 读取并在内存中排序分页；
//  4. 缓存 miss 时从 MySQL 权威查询并异步调度后台对账自愈。
func (r *Reader) ListJoinRequests(ctx context.Context, groupUUID, operatorUUID string, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if r == nil || r.store == nil || groupUUID == "" || operatorUUID == "" {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	// 1. 前置确认群未解散
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return nil, 0, err
	}
	// 2. 权威校验操作者管理员权限（回源 MySQL 点查）
	if _, err := r.store.EnsureActiveMemberRole(ctx, groupUUID, operatorUUID, repository.MemberRoleAdmin); err != nil {
		return nil, 0, err
	}

	// 3. 尝试从 Redis 待审批 Hash 缓存读取
	cachedItems, cacheHit, cacheErr := r.getGroupJoinRequestsFromCache(ctx, groupUUID)
	if cacheErr != nil {
		repoerr.LogRedisError(ctx, cacheErr)
	} else if cacheHit {
		return pageJoinRequests(cachedItems, page, pageSize)
	}

	// 4. 缓存未命中时回源 MySQL 权威列表
	items, err := r.store.LoadPendingJoinRequestsFromDB(ctx, groupUUID)
	if err != nil {
		return nil, 0, err
	}
	// 异步提交群缓存对账意图
	r.scheduleGroupRepair(ctx, groupUUID)
	return pageJoinRequests(items, page, pageSize)
}

// pageJoinRequests 对已经排好序的待审批列表做内存分页切片。
func pageJoinRequests(items []*model.GroupJoinRequest, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	total := int64(len(items))
	if total == 0 {
		return []*model.GroupJoinRequest{}, 0, nil
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

// getGroupJoinRequestsFromCache 从 Redis Hash 读取群待审批申请列表。
//
// 严格完整性校验：
//   - 检查 __SCHEMA__ == "2" 以及 __COMPLETE__ == "1"；
//   - 支持 __EMPTY__ 空集合占位哨兵，区分“群无待审批申请（有效空集）”与“缓存缺失（Miss）”；
//   - 反序列化所有单条申请 field，并在内存中按创建时间倒序（CreatedAt DESC, ApplyID DESC）排序。
func (r *Reader) getGroupJoinRequestsFromCache(ctx context.Context, groupUUID string) ([]*model.GroupJoinRequest, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.GroupJoinRequestPendingKey(groupUUID)
	values, err := r.redisClient.HGetAll(ctx, cacheKey).Result()
	if err != nil {
		if cachex.IsRedisWrongType(err) {
			return nil, false, nil
		}
		return nil, false, repoerr.WrapRedisError(err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}
	if values[repository.GroupProjectionSchemaField] != repository.GroupCacheSchemaVersion {
		return nil, false, nil
	}
	if values[repository.GroupProjectionCompleteField] != "1" {
		return nil, false, nil
	}
	projectionVersion, versionErr := strconv.ParseInt(values[repository.GroupProjectionVersionField], 10, 64)
	if versionErr != nil || projectionVersion <= 0 {
		return nil, false, nil
	}

	items := make([]*model.GroupJoinRequest, 0, len(values))
	sawEmpty := false
	for field, raw := range values {
		if field == repository.GroupProjectionSchemaField ||
			field == repository.GroupProjectionVersionField ||
			field == repository.GroupProjectionCompleteField {
			continue
		}
		if field == repository.GroupJoinRequestsEmptyField {
			if raw != repository.GroupJoinRequestsEmptyValue || sawEmpty || len(items) > 0 {
				return nil, false, nil
			}
			sawEmpty = true
			continue
		}
		if sawEmpty {
			return nil, false, nil
		}
		entry, decodeErr := repository.DecodeGroupJoinRequestCacheValue(raw)
		if decodeErr != nil {
			return nil, false, nil
		}
		applyID, parseErr := strconv.ParseInt(field, 10, 64)
		if parseErr != nil || entry.ApplyID != applyID {
			return nil, false, nil
		}
		item := repository.BuildGroupJoinRequestFromCache(entry)
		if item == nil {
			return nil, false, nil
		}
		items = append(items, item)
	}
	if len(items) == 0 && !sawEmpty {
		return nil, false, nil
	}
	if sawEmpty && len(values) != 4 {
		return nil, false, nil
	}
	repository.SortGroupJoinRequests(items)
	if cachex.Chance(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, cachex.JitterTTL(rediskey.GroupJoinRequestTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			repoerr.LogRedisError(ctx, expireErr)
		}
	}
	return items, true, nil
}
