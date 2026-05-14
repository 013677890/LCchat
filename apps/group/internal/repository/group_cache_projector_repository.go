package repository

import (
	"context"
	"errors"
	"fmt"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"strconv"
)

// NewGroupCacheProjectorRepository 创建 group.cache 投影仓储实例。
//
// 这里单独提供一个 projector 构造函数，而不是复用 service 侧的 IGroupRepository，原因是：
//  1. consumer 只依赖“事件投影到 Redis”这一类能力；
//  2. Wire 装配时可以显式区分业务仓储依赖和投影仓储依赖；
//  3. 即便当前底层复用同一个实现体，也能把依赖边界表达清楚。
func NewGroupCacheProjectorRepository(db *gorm.DB, redisClient *goredis.Client) IGroupCacheProjectorRepository {
	return &groupRepositoryImpl{db: db, redisClient: redisClient}
}

// ApplyGroupCacheEvent 根据 group.cache 事件把最终事实投影到 Redis。
//
// 设计原则：
//  1. projector 只处理缓存，不做任何业务权限判断；
//  2. 主缓存 `group:info` 允许在 group_created 首次完整创建；
//  3. 反向索引 `group:user_groups` 继续遵循 patch-if-exists；
//  4. 任意 Redis 可重试错误直接返回，由 Kafka 手动提交模式负责重试。
func (r *groupRepositoryImpl) ApplyGroupCacheEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := validateGroupCacheEventPayload(payload); err != nil {
		return err
	}
	switch payload.Action {
	case groupevent.ActionGroupCreated:
		return r.applyGroupCreatedEvent(ctx, payload)
	case groupevent.ActionMemberAdded:
		return r.applyMemberAddedEvent(ctx, payload)
	case groupevent.ActionMemberRemoved:
		return r.applyMemberRemovedEvent(ctx, payload)
	case groupevent.ActionGroupDismissed:
		return r.applyGroupDismissedEvent(ctx, payload)
	case groupevent.ActionGroupInfoUpdated:
		return r.applyGroupInfoUpdatedEvent(ctx, payload)
	case groupevent.ActionOwnerTransferred:
		return r.applyOwnerTransferredEvent(ctx, payload)
	case groupevent.ActionMemberRoleUpdated:
		return r.applyMemberRoleUpdatedEvent(ctx, payload)
	case groupevent.ActionJoinRequestCreated:
		return r.applyJoinRequestCreatedEvent(ctx, payload)
	case groupevent.ActionJoinRequestReviewed:
		return r.applyJoinRequestReviewedEvent(ctx, payload)
	default:
		return fmt.Errorf("%w: unsupported action %s", ErrInvalidProjectorPayload, payload.Action)
	}
}

// validateGroupCacheEventPayload 校验 projector 最低可执行载荷。
//
// 这里不追求把所有字段都校验到极致，而是只检查“缺了就一定无法安全投影”的部分，
// 这样可以：
//  1. 及时把坏消息识别出来；
//  2. 避免 consumer 对同一条无效消息无限重试；
//  3. 保持 projector 处理逻辑本身尽量简洁。
func validateGroupCacheEventPayload(payload groupevent.GroupCacheEventPayload) error {
	if payload.EventID == "" || payload.GroupUUID == "" || payload.Action == "" {
		return fmt.Errorf("%w: missing base fields", ErrInvalidProjectorPayload)
	}
	switch payload.Action {
	case groupevent.ActionGroupCreated:
		if payload.Group == nil {
			return fmt.Errorf("%w: group_created missing group snapshot", ErrInvalidProjectorPayload)
		}
		if len(payload.Members) == 0 {
			return fmt.Errorf("%w: group_created missing member snapshots", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionMemberAdded:
		if payload.Group == nil {
			return fmt.Errorf("%w: member_added missing group snapshot", ErrInvalidProjectorPayload)
		}
		if len(payload.Members) == 0 && len(payload.UserUUIDs) == 0 {
			return fmt.Errorf("%w: member_added missing target members", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionMemberRemoved:
		if payload.Group == nil || payload.UserUUID == "" {
			return fmt.Errorf("%w: member_removed missing required fields", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionGroupDismissed, groupevent.ActionGroupInfoUpdated:
		if payload.Group == nil {
			return fmt.Errorf("%w: %s missing group snapshot", ErrInvalidProjectorPayload, payload.Action)
		}
	case groupevent.ActionOwnerTransferred, groupevent.ActionMemberRoleUpdated:
		if payload.Group == nil {
			return fmt.Errorf("%w: %s missing group snapshot", ErrInvalidProjectorPayload, payload.Action)
		}
		if len(payload.Members) == 0 {
			return fmt.Errorf("%w: %s missing member snapshots", ErrInvalidProjectorPayload, payload.Action)
		}
	case groupevent.ActionJoinRequestCreated, groupevent.ActionJoinRequestReviewed:
		if payload.JoinRequest == nil {
			return fmt.Errorf("%w: %s missing join request snapshot", ErrInvalidProjectorPayload, payload.Action)
		}
	default:
		return fmt.Errorf("%w: unsupported action %s", ErrInvalidProjectorPayload, payload.Action)
	}
	return nil
}

// applyGroupCreatedEvent 处理建群后的首次缓存投影。
//
// 这里允许直接完整创建主缓存，原因是：
//  1. group_created 自带完整群快照和首批成员快照；
//  2. 首次建缓存比增量 patch 更稳定；
//  3. user_groups 反向索引仍只做 patch-if-exists，保持和项目既有策略一致。
func (r *groupRepositoryImpl) applyGroupCreatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	group := buildGroupInfoFromSnapshot(payload.Group)
	if err := r.setGroupInfoCache(ctx, group); err != nil {
		return err
	}
	members := buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members)
	if err := r.rebuildGroupMembersCache(ctx, payload.GroupUUID, members); err != nil {
		return err
	}
	return r.addGroupToUserGroupsIfExists(ctx, collectProjectedUserUUIDs(payload), group)
}

// applyMemberAddedEvent 处理成员新增/恢复后的缓存投影。
//
// 这里统一使用 upsert patch，而不是区分 insert / restore 两条路径，原因是：
//  1. 事件里已经携带最终成员快照；
//  2. 恢复成员和新增成员都可以安全落成“当前最新字段值”；
//  3. key 不存在时脚本会自动跳过，仍由读路径负责全量重建。
func (r *groupRepositoryImpl) applyMemberAddedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	group := buildGroupInfoFromSnapshot(payload.Group)
	if err := r.setGroupInfoCacheIfExists(ctx, group); err != nil {
		return err
	}
	for _, member := range buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members) {
		if err := r.upsertGroupMemberCacheIfExists(ctx, payload.GroupUUID, member); err != nil {
			return err
		}
	}
	return r.addGroupToUserGroupsIfExists(ctx, collectProjectedUserUUIDs(payload), group)
}

// applyMemberRemovedEvent 处理成员退群/被踢后的缓存投影。
//
// 删除路径只需要同步三类缓存：
//  1. 群资料里的 member_count；
//  2. 群成员 Hash 中的单个成员 field；
//  3. 目标用户的 user_groups 反向索引。
func (r *groupRepositoryImpl) applyMemberRemovedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := r.setGroupInfoCacheIfExists(ctx, buildGroupInfoFromSnapshot(payload.Group)); err != nil {
		return err
	}
	if err := r.removeGroupMemberCacheIfExists(ctx, payload.GroupUUID, payload.UserUUID); err != nil {
		return err
	}
	return r.removeGroupFromUserGroupsIfExists(ctx, payload.UserUUID, payload.GroupUUID)
}

// applyGroupDismissedEvent 处理群解散后的缓存投影。
//
// 群解散后：
//  1. `group:info` 若存在则补成 status=2；
//  2. `group:members` 直接整 key 删除，避免残留“幽灵成员”；
//  3. 每个活跃成员的 user_groups 反向索引按 patch-if-exists 移除。
func (r *groupRepositoryImpl) applyGroupDismissedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := r.setGroupInfoCacheIfExists(ctx, buildGroupInfoFromSnapshot(payload.Group)); err != nil {
		return err
	}
	if err := r.deleteGroupMembersCache(ctx, payload.GroupUUID); err != nil {
		return err
	}
	for _, userUUID := range payload.UserUUIDs {
		if err := r.removeGroupFromUserGroupsIfExists(ctx, userUUID, payload.GroupUUID); err != nil {
			return err
		}
	}
	return nil
}

// applyGroupInfoUpdatedEvent 处理群资料变更后的缓存投影。
//
// 第二批资料更新只改 `group:info` 主缓存，不主动触碰 members / user_groups，
// 因为成员结构与反向索引都不依赖 notice / add_mode 这些资料字段。
func (r *groupRepositoryImpl) applyGroupInfoUpdatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.setGroupInfoCacheIfExists(ctx, buildGroupInfoFromSnapshot(payload.Group))
}

// applyOwnerTransferredEvent 处理群主转让后的缓存投影。
//
// 该事件需要同步两类缓存：
//  1. `group:info` 里的 owner_uuid；
//  2. `group:members` 中老群主和新群主的 role。
func (r *groupRepositoryImpl) applyOwnerTransferredEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := r.setGroupInfoCacheIfExists(ctx, buildGroupInfoFromSnapshot(payload.Group)); err != nil {
		return err
	}
	for _, member := range buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members) {
		if err := r.upsertGroupMemberCacheIfExists(ctx, payload.GroupUUID, member); err != nil {
			return err
		}
	}
	return nil
}

// applyMemberRoleUpdatedEvent 处理成员角色变更后的缓存投影。
//
// 角色变更不会改变成员集合和 user_groups 反向索引，
// 因此这里只 patch 群资料主缓存与目标成员 role 字段。
func (r *groupRepositoryImpl) applyMemberRoleUpdatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := r.setGroupInfoCacheIfExists(ctx, buildGroupInfoFromSnapshot(payload.Group)); err != nil {
		return err
	}
	for _, member := range buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members) {
		if err := r.upsertGroupMemberCacheIfExists(ctx, payload.GroupUUID, member); err != nil {
			return err
		}
	}
	return nil
}

// applyJoinRequestCreatedEvent 处理新增待审批入群申请后的缓存投影。
//
// 待审批申请列表缓存和成员缓存一样遵循 patch-if-exists：
//  1. 缓存已存在时增量写入单条申请；
//  2. 缓存不存在时直接跳过，继续由读路径负责全量重建；
//  3. 申请展示资料仍由上层聚合 user_profile，不在这里冗余缓存。
func (r *groupRepositoryImpl) applyJoinRequestCreatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.upsertGroupJoinRequestCacheIfExists(ctx, payload.GroupUUID, buildGroupJoinRequestFromSnapshot(payload.JoinRequest))
}

// applyJoinRequestReviewedEvent 处理待审批入群申请被通过或拒绝后的缓存投影。
//
// 审批完成后，这条申请不应继续停留在“待审批列表”缓存里，
// 因此这里直接按 apply_id 删除对应 field；若缓存缺失则继续保持跳过语义。
func (r *groupRepositoryImpl) applyJoinRequestReviewedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if payload.JoinRequest == nil {
		return fmt.Errorf("%w: join_request_reviewed missing join request snapshot", ErrInvalidProjectorPayload)
	}
	return r.removeGroupJoinRequestCacheIfExists(ctx, payload.GroupUUID, payload.JoinRequest.ApplyID)
}

// setGroupInfoCacheIfExists 仅在 `group:info` 已存在时覆盖最新群快照并刷新 TTL。
//
// 该方法供 projector 的增量事件使用，保证写事实不会反向承担“首次建缓存”职责。
func (r *groupRepositoryImpl) setGroupInfoCacheIfExists(ctx context.Context, group *model.GroupInfo) error {
	if r == nil || r.redisClient == nil || group == nil || group.Uuid == "" {
		return nil
	}
	cacheKey := rediskey.GroupInfoKey(group.Uuid)
	luaScript := goredis.NewScript(luaSetStringIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.GroupInfoTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		encodeGroupInfoCacheValue(group),
		strconv.Itoa(expireSeconds),
	).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// addGroupToUserGroupsIfExists 仅在 user_groups 已存在时把群加入对应用户的反向索引。
//
// 这里统一去重处理，避免同一事件里重复 user_uuid 触发多次 Lua 调用。
func (r *groupRepositoryImpl) addGroupToUserGroupsIfExists(ctx context.Context, userUUIDs []string, group *model.GroupInfo) error {
	if r == nil || r.redisClient == nil || group == nil || group.Uuid == "" || len(userUUIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			continue
		}
		if _, exists := seen[userUUID]; exists {
			continue
		}
		seen[userUUID] = struct{}{}
		if err := r.addGroupToUserGroupIfExists(ctx, userUUID, group); err != nil {
			return err
		}
	}
	return nil
}

// addGroupToUserGroupIfExists 仅在单个用户的 user_groups key 已存在时追加群 UUID。
func (r *groupRepositoryImpl) addGroupToUserGroupIfExists(ctx context.Context, userUUID string, group *model.GroupInfo) error {
	if r == nil || r.redisClient == nil || userUUID == "" || group == nil || group.Uuid == "" {
		return nil
	}
	cacheKey := rediskey.UserGroupListKey(userUUID)
	luaScript := goredis.NewScript(luaZAddIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.UserGroupListTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		strconv.FormatInt(group.UpdatedAt.Unix(), 10),
		group.Uuid,
		strconv.Itoa(expireSeconds),
	).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// removeGroupFromUserGroupsIfExists 仅在 user_groups 已存在时移除对应群 UUID。
func (r *groupRepositoryImpl) removeGroupFromUserGroupsIfExists(ctx context.Context, userUUID, groupUUID string) error {
	if r == nil || r.redisClient == nil || userUUID == "" || groupUUID == "" {
		return nil
	}
	cacheKey := rediskey.UserGroupListKey(userUUID)
	luaScript := goredis.NewScript(luaZRemIfExists)
	expireSeconds := int(getRandomExpireTime(rediskey.UserGroupListTTL).Seconds())
	_, err := luaScript.Run(ctx, r.redisClient,
		[]string{cacheKey},
		groupUUID,
		strconv.Itoa(expireSeconds),
	).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
			return nil
		}
		return WrapRedisError(err)
	}
	return nil
}

// collectProjectedUserUUIDs 提取事件里需要 patch user_groups 的用户集合。
//
// 优先使用显式 `user_uuids`，如果调用方没带，则回退到 `members` 快照，
// 这样可以兼容当前 mapper 和未来可能的轻微载荷调整。
func collectProjectedUserUUIDs(payload groupevent.GroupCacheEventPayload) []string {
	if len(payload.UserUUIDs) > 0 {
		return payload.UserUUIDs
	}
	result := make([]string, 0, len(payload.Members))
	seen := make(map[string]struct{}, len(payload.Members))
	for _, member := range payload.Members {
		if member.UserUUID == "" {
			continue
		}
		if _, exists := seen[member.UserUUID]; exists {
			continue
		}
		seen[member.UserUUID] = struct{}{}
		result = append(result, member.UserUUID)
	}
	return result
}
