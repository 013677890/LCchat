package repository

import (
	"context"
	"errors"
	"fmt"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// groupRepositoryImpl 是 group-service 仓储层的只读实现。
//
// 当前先聚焦“群资料/成员查询”这条闭环，因此这里主要承接：
//  1. 群资料查询；
//  2. 群成员查询；
//  3. 当前用户所属群列表查询；
//  4. 成员昵称头像补齐所需的用户资料批量查询。
//
// 等后续真正开始群管理写逻辑时，再在同一实现上继续扩展写方法即可。
type groupRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
	memberGroup singleflight.Group
}

// NewGroupRepository 创建 group 仓储实例。
//
// 当前仍保持薄构造：
//  1. 只接收 gorm.DB；
//  2. 不在构造阶段探测连通性；
//  3. 由上层 provider 负责基础设施初始化与失败处理。
func NewGroupRepository(db *gorm.DB, redisClient *goredis.Client) IGroupRepository {
	return &groupRepositoryImpl{db: db, redisClient: redisClient}
}

// GetGroupInfo 按群 UUID 查询有效群资料。
//
// 约束：
//  1. 只返回 status=0 且未软删的群；
//  2. 群不存在、已解散、已删除都统一映射为“记录不存在”；
//  3. 上层无需感知 groups 表的存储细节。
func (r *groupRepositoryImpl) GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if r == nil || r.db == nil || groupUUID == "" {
		return nil, ErrRecordNotFound
	}

	var groupInfo model.GroupInfo
	if err := r.db.WithContext(ctx).
		Where("uuid = ? AND status = 0 AND deleted_at IS NULL", groupUUID).
		First(&groupInfo).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return &groupInfo, nil
}

// GetGroupMembers 获取群内有效成员列表。
//
// 查询分两步：
//  1. 先确认群本身存在，避免把“群不存在”和“群存在但成员为空”混淆；
//  2. 再按角色优先、入群时间次序返回有效成员，方便上层直接展示与做权限判断。
func (r *groupRepositoryImpl) GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if r == nil || r.db == nil || groupUUID == "" {
		return []*model.GroupMember{}, nil
	}

	members, cacheHit, err := r.getGroupMembersFromCache(ctx, groupUUID)
	if err != nil {
		LogRedisError(ctx, err)
	} else if cacheHit {
		return members, nil
	}

	return r.fetchGroupMembersWithSingleflight(ctx, groupUUID)
}

func (r *groupRepositoryImpl) loadGroupMembersFromDB(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if _, err := r.GetGroupInfo(ctx, groupUUID); err != nil {
		return nil, err
	}

	var members []*model.GroupMember
	if err := r.db.WithContext(ctx).
		Table("group_members AS gm").
		Select("gm.*").
		Joins("JOIN groups AS g ON g.uuid = gm.group_uuid").
		Where("gm.group_uuid = ? AND gm.status = 0 AND gm.deleted_at IS NULL", groupUUID).
		Where("g.status = 0 AND g.deleted_at IS NULL").
		Order("gm.role DESC, gm.joined_at ASC, gm.id ASC").
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("查询群成员失败: %w", WrapDBError(err))
	}
	return members, nil
}

func (r *groupRepositoryImpl) fetchGroupMembersWithSingleflight(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	value, err, _ := r.memberGroup.Do("group_members:"+groupUUID, func() (interface{}, error) {
		members, loadErr := r.loadGroupMembersFromDB(ctx, groupUUID)
		if loadErr != nil {
			return nil, loadErr
		}
		r.rebuildGroupMembersCache(ctx, groupUUID, members)
		return cloneGroupMembers(members), nil
	})
	if err != nil {
		return nil, err
	}

	members, ok := value.([]*model.GroupMember)
	if !ok {
		return nil, fmt.Errorf("群成员 singleflight 返回类型错误")
	}
	return cloneGroupMembers(members), nil
}

func (r *groupRepositoryImpl) getGroupMembersFromCache(ctx context.Context, groupUUID string) ([]*model.GroupMember, bool, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return nil, false, nil
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	values, err := r.redisClient.HGetAll(ctx, cacheKey).Result()
	if err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		return nil, false, WrapRedisError(err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}

	members := make([]*model.GroupMember, 0, len(values))
	for userUUID, raw := range values {
		entry, decodeErr := decodeGroupMemberCacheValue(raw)
		if decodeErr != nil {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil, false, nil
		}
		if member := buildGroupMemberFromCache(userUUID, entry); member != nil {
			members = append(members, member)
		}
	}
	sortGroupMembers(members)

	if getRandomBool(0.01) {
		if expireErr := r.redisClient.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL)).Err(); expireErr != nil && !errors.Is(expireErr, goredis.Nil) {
			LogRedisError(ctx, expireErr)
		}
	}

	return members, true, nil
}

func (r *groupRepositoryImpl) rebuildGroupMembersCache(ctx context.Context, groupUUID string, members []*model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	fields := make(map[string]string, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		fields[member.UserUuid] = encodeGroupMemberCacheValue(member)
	}
	if len(fields) == 0 {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return
	}

	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, cacheKey)
	pipe.HSet(ctx, cacheKey, fields)
	pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL))
	if _, err := pipe.Exec(ctx); err != nil {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return
		}
		LogRedisError(ctx, err)
	}
}

// insertGroupMembersCacheAsync 在群成员 Hash 已存在时，异步增量插入新成员 field。
//
// 该方法供后续 CreateGroup / AddMember 写路径复用；如果 key 已过期，则交给下一次读路径
// 统一全量重建，避免把局部成员集合误写成完整事实。
func (r *groupRepositoryImpl) insertGroupMembersCacheAsync(ctx context.Context, groupUUID string, members []*model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" || len(members) == 0 {
		return
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaInsertGroupMemberIfExists)
		expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
		seen := make(map[string]struct{}, len(members))

		for _, member := range members {
			if member == nil || member.UserUuid == "" {
				continue
			}
			if _, ok := seen[member.UserUuid]; ok {
				continue
			}
			seen[member.UserUuid] = struct{}{}

			_, err := luaScript.Run(runCtx, r.redisClient,
				[]string{cacheKey},
				member.UserUuid,
				encodeGroupMemberCacheValue(member),
				expireSeconds,
			).Result()
			if err != nil && err != goredis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, cacheKey).Err()
					return
				}
				LogRedisError(runCtx, err)
			}
		}
	}, async.AsyncRedisTimeout)
}

// upsertGroupMemberCacheAsync 在群成员 Hash 已存在时，异步更新单个成员 field。
//
// 该方法供后续角色调整、重新入群等写路径复用；若缓存整体缺失，则继续让读路径全量重建。
func (r *groupRepositoryImpl) upsertGroupMemberCacheAsync(ctx context.Context, groupUUID string, member *model.GroupMember) {
	if r == nil || r.redisClient == nil || groupUUID == "" || member == nil || member.UserUuid == "" {
		return
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaUpsertGroupMemberIfExists)
		expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			member.UserUuid,
			encodeGroupMemberCacheValue(member),
			expireSeconds,
		).Result()
		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// removeGroupMemberCacheAsync 在群成员 Hash 已存在时，异步删除单个成员 field。
//
// 若移除后集合为空，则脚本会补一个 __EMPTY__ 占位，避免极端空集合场景持续穿透数据库。
func (r *groupRepositoryImpl) removeGroupMemberCacheAsync(ctx context.Context, groupUUID, userUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" || userUUID == "" {
		return
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaRemoveGroupMemberIfExists)
		expireSeconds := int(getRandomExpireTime(rediskey.GroupMembersTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			userUUID,
			groupMembersEmptyValue,
			expireSeconds,
		).Result()
		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// deleteGroupMembersCacheAsync 异步删除整份群成员缓存。
//
// 该方法供后续 DismissGroup 等强失效写路径复用，此类场景直接删 key 比逐个 patch 更安全。
func (r *groupRepositoryImpl) deleteGroupMembersCacheAsync(ctx context.Context, groupUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.redisClient.Del(runCtx, cacheKey).Err(); err != nil && err != goredis.Nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// CheckGroupMember 检查指定用户是否仍是群内有效成员，并返回角色。
//
// 这里把 member + group 状态放到同一条 join 查询里，
// 避免群已解散后仅凭 group_members 残留数据读出“幽灵成员”结论。
func (r *groupRepositoryImpl) CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error) {
	if r == nil || r.db == nil || groupUUID == "" || userUUID == "" {
		return false, -1, ErrRecordNotFound
	}

	cacheHit, isMember, role, err := r.checkGroupMemberFromCache(ctx, groupUUID, userUUID)
	if err != nil {
		LogRedisError(ctx, err)
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

func (r *groupRepositoryImpl) checkGroupMemberFromCache(ctx context.Context, groupUUID, userUUID string) (bool, bool, int8, error) {
	if r == nil || r.redisClient == nil || groupUUID == "" || userUUID == "" {
		return false, false, -1, nil
	}

	cacheKey := rediskey.GroupMembersKey(groupUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	memberCmd := pipe.HGet(ctx, cacheKey, userUUID)
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.GroupMembersTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return false, false, -1, nil
		}
		return false, false, -1, WrapRedisError(err)
	}
	if existsCmd.Val() == 0 {
		return false, false, -1, nil
	}

	if memberCmd.Err() == nil {
		entry, decodeErr := decodeGroupMemberCacheValue(memberCmd.Val())
		if decodeErr != nil {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return false, false, -1, nil
		}
		return true, true, entry.Role, nil
	}
	if errors.Is(memberCmd.Err(), goredis.Nil) {
		return true, false, -1, nil
	}
	if isRedisWrongType(memberCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, false, -1, nil
	}
	return false, false, -1, WrapRedisError(memberCmd.Err())
}

// ListUserGroups 获取当前用户所属的有效群列表。
//
// 这里使用 join 一次性筛出：
//  1. 群成员记录有效；
//  2. 群本身有效；
//  3. 结果按最近更新时间倒序，便于上层默认展示最近活跃/最近变更的群。
func (r *groupRepositoryImpl) ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	if r == nil || r.db == nil || userUUID == "" {
		return []*model.GroupInfo{}, nil
	}

	var groups []*model.GroupInfo
	if err := r.db.WithContext(ctx).
		Table("groups AS g").
		Select("DISTINCT g.*").
		Joins("JOIN group_members AS gm ON gm.group_uuid = g.uuid").
		Where("gm.user_uuid = ? AND gm.status = 0 AND gm.deleted_at IS NULL", userUUID).
		Where("g.status = 0 AND g.deleted_at IS NULL").
		Order("g.updated_at DESC, g.id DESC").
		Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("查询用户群列表失败: %w", WrapDBError(err))
	}
	return groups, nil
}

// GetUserProfiles 按用户 UUID 批量查询资料。
//
// 返回 map 而不是切片，目的是让 service 层在组装成员列表时能 O(1) 命中昵称头像，
// 避免再做额外的二次索引构建。
func (r *groupRepositoryImpl) GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
	result := make(map[string]*model.UserProfile)
	if r == nil || r.db == nil || len(userUUIDs) == 0 {
		return result, nil
	}

	var profiles []*model.UserProfile
	if err := r.db.WithContext(ctx).
		Where("user_uuid IN ?", userUUIDs).
		Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("批量查询用户资料失败: %w", WrapDBError(err))
	}

	for _, profile := range profiles {
		if profile == nil || profile.UserUuid == "" {
			continue
		}
		result[profile.UserUuid] = profile
	}
	return result, nil
}
