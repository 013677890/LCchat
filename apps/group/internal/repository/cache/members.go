package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	goredis "github.com/redis/go-redis/v9"
)

// GetGroupMembers 获取群内有效成员列表。
//
// 缓存读取与一致性保障：
//  1. 必须首先确认群本身处于正常可读状态（ensureGroupNormal）：若群已解散，空成员 Hash 只能代表解散态 tombstone，
//     绝不能误判为“群存在但成员为空”；
//  2. 优先从 Redis Hash `group:members:{group_uuid}` 读取（校验 __SCHEMA__、__VERSION__、__COMPLETE__=1）；
//  3. 若缓存 miss 或不完整，使用 Singleflight 合并并发请求回源 MySQL，并异步调度后台 ReconcileGroupCache 重建缓存。
func (r *Reader) GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if r == nil || r.store == nil || groupUUID == "" {
		return []*model.GroupMember{}, nil
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return nil, err
	}
	members, cacheHit, err := r.getGroupMembersFromCache(ctx, groupUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	} else if cacheHit {
		return members, nil
	}
	return r.fetchGroupMembersWithSingleflight(ctx, groupUUID)
}

// CheckGroupMember 检查指定用户是否为群内有效成员，并返回角色（Role: 0=普通成员, 1=管理员, 2=群主）。
//
// 性能优化设计：
//   - 点查时禁止对大群执行 `HGETALL`，而是调用专用的 `LuaReadVersionedHashField` 在单次网络往返中原子校验
//     Hash 结构完整性并只读取目标用户的单 field（O(1) 时间复杂度）；
//   - 只有在缓存结构不存在/非法时，才通过 Singleflight 回源 MySQL 并查找。
func (r *Reader) CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error) {
	if r == nil || r.store == nil || groupUUID == "" || userUUID == "" {
		return false, -1, repoerr.ErrRecordNotFound
	}
	if err := r.ensureGroupNormal(ctx, groupUUID); err != nil {
		return false, -1, err
	}
	cacheHit, isMember, role, err := r.checkGroupMemberFromCache(ctx, groupUUID, userUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
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

// fetchGroupMembersWithSingleflight 把同一群的成员 miss 回源合并成一次 MySQL 查询。
//
// Singleflight 并发控制：
//   - 防止瞬时高并发访问未缓存或过期的成员列表时，大量请求同时穿透并压垮 MySQL 数据库；
//   - 仅第一个请求实际查询 MySQL 并异步触发对账重建，后续并发请求共享查询结果（Clone 副本返回，防指针逃逸并发修改）。
func (r *Reader) fetchGroupMembersWithSingleflight(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	value, err, _ := r.flightGroup.Do("group_members:"+groupUUID, func() (interface{}, error) {
		members, loadErr := r.store.LoadGroupMembersFromDB(ctx, groupUUID)
		if loadErr != nil {
			return nil, loadErr
		}
		// 调度后台异步对账自愈，禁止当前读线程直接晚写 Redis
		r.scheduleGroupRepair(ctx, groupUUID)
		return repository.CloneGroupMembers(members), nil
	})
	if err != nil {
		return nil, err
	}
	members, ok := value.([]*model.GroupMember)
	if !ok {
		return nil, fmt.Errorf("群成员 singleflight 返回类型错误")
	}
	return repository.CloneGroupMembers(members), nil
}

// getGroupMembersFromCache 读取完整成员 Hash。
//
// 严格完整性校验规则：
//  1. 缺少 `__SCHEMA__`（当前必须为 "2"）或 `__COMPLETE__ != "1"` 时一律按 miss 处理；
//  2. 空集合哨兵 `__EMPTY__` 与业务成员 field 不能并存；
//  3. 过滤保留元数据字段（`__` 前缀），反序列化每个成员并按角色和入群时间排好序返回。
func (r *Reader) getGroupMembersFromCache(ctx context.Context, groupUUID string) ([]*model.GroupMember, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}
	cacheKey := rediskey.GroupMembersKey(groupUUID)
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

	members := make([]*model.GroupMember, 0, len(values))
	sawEmpty := false
	for userUUID, raw := range values {
		if userUUID == repository.GroupProjectionSchemaField ||
			userUUID == repository.GroupProjectionVersionField ||
			userUUID == repository.GroupProjectionCompleteField {
			continue
		}
		if userUUID == repository.GroupMembersEmptyField {
			if raw != repository.GroupMembersEmptyValue || sawEmpty || len(members) > 0 {
				return nil, false, nil
			}
			sawEmpty = true
			continue
		}
		if strings.HasPrefix(userUUID, "__") || sawEmpty {
			return nil, false, nil
		}
		entry, decodeErr := repository.DecodeGroupMemberCacheValue(raw)
		if decodeErr != nil {
			return nil, false, nil
		}
		member := repository.BuildGroupMemberFromCache(userUUID, entry)
		if member == nil {
			return nil, false, nil
		}
		members = append(members, member)
	}
	if len(members) == 0 && !sawEmpty {
		return nil, false, nil
	}
	if sawEmpty && len(values) != 4 {
		return nil, false, nil
	}
	repository.SortGroupMembers(members)
	if cachex.Chance(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, cachex.JitterTTL(rediskey.GroupMembersTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			repoerr.LogRedisError(ctx, expireErr)
		}
	}
	return members, true, nil
}

// checkGroupMemberFromCache 点查成员身份。
//
// 返回值：
//   - cacheHit: Hash 结构是否存在且完整；
//   - isMember: 用户是否在该群成员 Hash 中（仅在 cacheHit=true 时有效）；
//   - role: 成员角色值（0=普通成员, 1=管理员, 2=群主）。
func (r *Reader) checkGroupMemberFromCache(ctx context.Context, groupUUID, userUUID string) (bool, bool, int8, error) {
	raw, fieldExists, cacheHit, err := r.readMemberHashField(ctx, groupUUID, userUUID)
	if err != nil || !cacheHit {
		return false, false, -1, err
	}
	if !fieldExists {
		return true, false, -1, nil
	}
	entry, decodeErr := repository.DecodeGroupMemberCacheValue(raw)
	if decodeErr != nil {
		return false, false, -1, nil
	}
	return true, true, entry.Role, nil
}

// getGroupMemberFromCache 点查单个成员快照，供 CheckGroupSendPermission 发送权限检查使用。
func (r *Reader) getGroupMemberFromCache(ctx context.Context, groupUUID, userUUID string) (*model.GroupMember, bool, error) {
	raw, fieldExists, cacheHit, err := r.readMemberHashField(ctx, groupUUID, userUUID)
	if err != nil || !cacheHit {
		return nil, cacheHit, err
	}
	if !fieldExists {
		return nil, true, nil
	}
	entry, decodeErr := repository.DecodeGroupMemberCacheValue(raw)
	if decodeErr != nil {
		return nil, false, nil
	}
	return repository.BuildGroupMemberFromCache(userUUID, entry), true, nil
}

// readMemberHashField 用版本化 Lua 脚本 LuaReadVersionedHashField 原子读取单个成员 field。
//
// 关键优化：禁止 HGETALL，避免千人大群在点查发言权限时拉取全量成员数据造成的 CPU 与网络冲击。
func (r *Reader) readMemberHashField(ctx context.Context, groupUUID, userUUID string) (string, bool, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" || userUUID == "" {
		return "", false, false, nil
	}
	return repository.ReadVersionedHashField(
		ctx,
		r.redisClient,
		rediskey.GroupMembersKey(groupUUID),
		userUUID,
		int(cachex.JitterTTL(rediskey.GroupMembersTTL).Seconds()),
		cachex.Chance(0.01),
	)
}
