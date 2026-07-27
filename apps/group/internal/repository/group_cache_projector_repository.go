package repository

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// NewGroupCacheProjectorRepository 创建 group.cache 投影仓储实例。
//
// 这里单独提供一个 projector 构造函数，而不是复用 service 侧的 IGroupRepository，原因是：
//  1. consumer 只依赖“事件投影到 Redis”这一类能力；
//  2. Wire 装配时可以显式区分业务仓储依赖和投影仓储依赖；
//  3. 即便当前底层复用同一个实现体，也能把依赖边界表达清楚。
func NewGroupCacheProjectorRepository(db *gorm.DB, redisClient *goredis.Client) IGroupCacheProjectorRepository {
	repo := &groupRepositoryImpl{db: db, redisClient: redisClient}
	repo.initGroupUUIDBloom(context.Background())
	return repo
}

// ApplyGroupCacheEvent 根据 group.cache 事件把最终事实投影到 Redis。
//
// 设计原则：
//  1. projector 只处理缓存，不做任何业务权限判断；
//  2. 所有 Redis 变更都必须携带 projection_version，并在 Lua 内拒绝旧版本；
//  3. group_created 可以首次完整创建缓存，普通增量事件只 patch 已存在的群维度缓存；
//  4. 用户群反向索引会留下逐群版本 tombstone，但只有完整对账才能写 READY；
//  5. 任意 Redis 可重试错误直接返回，由 Kafka 手动提交模式负责重试。
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
	case groupevent.ActionGroupMuteSettingUpdated:
		return r.applyGroupMuteSettingUpdatedEvent(ctx, payload)
	case groupevent.ActionOwnerTransferred:
		return r.applyOwnerTransferredEvent(ctx, payload)
	case groupevent.ActionMemberRoleUpdated:
		return r.applyMemberRoleUpdatedEvent(ctx, payload)
	case groupevent.ActionMemberProfileUpdated:
		return r.applyMemberProfileUpdatedEvent(ctx, payload)
	case groupevent.ActionMemberMuted:
		return r.applyMemberMutedEvent(ctx, payload)
	case groupevent.ActionJoinRequestCreated:
		return r.applyJoinRequestCreatedEvent(ctx, payload)
	case groupevent.ActionJoinRequestReviewed:
		return r.applyJoinRequestReviewedEvent(ctx, payload)
	case groupevent.ActionJoinRequestCanceled:
		return r.applyJoinRequestReviewedEvent(ctx, payload)
	default:
		return fmt.Errorf("%w: unsupported action %s", ErrInvalidProjectorPayload, payload.Action)
	}
}

// validateGroupCacheEventPayload 校验 projector 可安全执行的完整 v2 语义。
//
// 仅检查字段存在还不够：例如 group_dismissed 携带 normal 群快照，或群主转让中
// 旧群主没有降为 member，虽然 JSON 完整，却会把自相矛盾的最终态写进多个缓存。
// v2 因此按 action 校验目标集合、角色和群状态；所有协议错误都作为永久坏消息处理，
// 不从其他字段推导缺失值，也不保留旧事件的兼容解释。
func validateGroupCacheEventPayload(payload groupevent.GroupCacheEventPayload) error {
	if payload.SchemaVersion != groupevent.GroupCacheSchemaVersion {
		return fmt.Errorf("%w: unsupported schema_version %d", ErrInvalidProjectorPayload, payload.SchemaVersion)
	}
	if payload.ProjectionVersion <= 0 {
		return fmt.Errorf("%w: projection_version must be positive", ErrInvalidProjectorPayload)
	}
	if payload.EventID == "" || payload.GroupUUID == "" || payload.Action == "" {
		return fmt.Errorf("%w: missing base fields", ErrInvalidProjectorPayload)
	}
	if payload.Group != nil {
		if payload.Group.GroupUUID != payload.GroupUUID {
			return fmt.Errorf("%w: group snapshot uuid mismatch", ErrInvalidProjectorPayload)
		}
		if payload.Group.GroupID <= 0 ||
			payload.Group.OwnerUUID == "" ||
			payload.Group.MemberCount < 0 ||
			(payload.Group.AddMode != 0 && payload.Group.AddMode != 1) ||
			(payload.Group.Status != int32(groupStatusNormal) &&
				payload.Group.Status != int32(groupStatusDisabled) &&
				payload.Group.Status != int32(groupStatusDismissed)) ||
			payload.Group.UpdatedAtUnixMs <= 0 {
			return fmt.Errorf("%w: group snapshot contains invalid required fields", ErrInvalidProjectorPayload)
		}
	}
	if len(payload.Members) > 0 && !validProjectedMemberSnapshots(payload.Members) {
		return fmt.Errorf("%w: member snapshots contain invalid required fields", ErrInvalidProjectorPayload)
	}
	switch payload.Action {
	case groupevent.ActionGroupCreated:
		if payload.Group == nil {
			return fmt.Errorf("%w: group_created missing group snapshot", ErrInvalidProjectorPayload)
		}
		if len(payload.Members) == 0 {
			return fmt.Errorf("%w: group_created missing member snapshots", ErrInvalidProjectorPayload)
		}
		if !sameProjectedMemberSet(payload.Members, payload.UserUUIDs) {
			return fmt.Errorf("%w: group_created user_uuids must exactly match members", ErrInvalidProjectorPayload)
		}
		if payload.Group.Status != int32(groupStatusNormal) ||
			payload.Group.MemberCount != int32(len(payload.Members)) ||
			!validProjectedGroupOwnership(payload.Group, payload.Members) {
			return fmt.Errorf("%w: group_created final state is inconsistent", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionMemberAdded:
		if payload.Group == nil {
			return fmt.Errorf("%w: member_added missing group snapshot", ErrInvalidProjectorPayload)
		}
		if len(payload.Members) == 0 || len(payload.UserUUIDs) == 0 {
			return fmt.Errorf("%w: member_added missing target members", ErrInvalidProjectorPayload)
		}
		if !sameProjectedMemberSet(payload.Members, payload.UserUUIDs) {
			return fmt.Errorf("%w: member_added user_uuids must exactly match members", ErrInvalidProjectorPayload)
		}
		if payload.Group.Status != int32(groupStatusNormal) ||
			!allProjectedMembersHaveRole(payload.Members, memberRoleMember) {
			return fmt.Errorf("%w: member_added final state is inconsistent", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionMemberRemoved:
		if payload.Group == nil || payload.UserUUID == "" {
			return fmt.Errorf("%w: member_removed missing required fields", ErrInvalidProjectorPayload)
		}
		if payload.Group.Status != int32(groupStatusNormal) {
			return fmt.Errorf("%w: member_removed group must be normal", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionGroupDismissed, groupevent.ActionGroupInfoUpdated, groupevent.ActionGroupMuteSettingUpdated:
		if payload.Group == nil {
			return fmt.Errorf("%w: %s missing group snapshot", ErrInvalidProjectorPayload, payload.Action)
		}
		if payload.Action == groupevent.ActionGroupDismissed {
			if payload.Group.Status != int32(groupStatusDismissed) {
				return fmt.Errorf("%w: group_dismissed snapshot must be dismissed", ErrInvalidProjectorPayload)
			}
		} else if payload.Group.Status != int32(groupStatusNormal) {
			return fmt.Errorf("%w: %s group must be normal", ErrInvalidProjectorPayload, payload.Action)
		}
		if payload.Action == groupevent.ActionGroupDismissed && len(payload.UserUUIDs) == 0 {
			return fmt.Errorf("%w: group_dismissed missing historical member uuids", ErrInvalidProjectorPayload)
		}
		if payload.Action == groupevent.ActionGroupDismissed && !validUniqueUUIDs(payload.UserUUIDs) {
			return fmt.Errorf("%w: group_dismissed contains invalid member uuids", ErrInvalidProjectorPayload)
		}
	case groupevent.ActionOwnerTransferred, groupevent.ActionMemberRoleUpdated, groupevent.ActionMemberProfileUpdated, groupevent.ActionMemberMuted:
		if payload.Group == nil {
			return fmt.Errorf("%w: %s missing group snapshot", ErrInvalidProjectorPayload, payload.Action)
		}
		if payload.Group.Status != int32(groupStatusNormal) {
			return fmt.Errorf("%w: %s group must be normal", ErrInvalidProjectorPayload, payload.Action)
		}
		if len(payload.Members) == 0 {
			return fmt.Errorf("%w: %s missing member snapshots", ErrInvalidProjectorPayload, payload.Action)
		}
		if !sameProjectedMemberSet(payload.Members, payload.UserUUIDs) {
			return fmt.Errorf("%w: %s user_uuids must exactly match members", ErrInvalidProjectorPayload, payload.Action)
		}
		switch payload.Action {
		case groupevent.ActionOwnerTransferred:
			if !validProjectedOwnerTransfer(payload.Group, payload.Members) {
				return fmt.Errorf("%w: owner_transferred final state is inconsistent", ErrInvalidProjectorPayload)
			}
		case groupevent.ActionMemberRoleUpdated:
			if len(payload.Members) != 1 ||
				(payload.Members[0].Role != int32(memberRoleMember) &&
					payload.Members[0].Role != int32(memberRoleAdmin)) {
				return fmt.Errorf("%w: member_role_updated final state is inconsistent", ErrInvalidProjectorPayload)
			}
		default:
			if len(payload.Members) != 1 {
				return fmt.Errorf("%w: %s must contain exactly one member", ErrInvalidProjectorPayload, payload.Action)
			}
		}
	case groupevent.ActionJoinRequestCreated, groupevent.ActionJoinRequestReviewed, groupevent.ActionJoinRequestCanceled:
		if payload.JoinRequest == nil ||
			payload.JoinRequest.ApplyID <= 0 ||
			payload.JoinRequest.ApplicantUUID == "" ||
			payload.JoinRequest.CreatedAtUnixMs <= 0 {
			return fmt.Errorf("%w: %s missing join request snapshot", ErrInvalidProjectorPayload, payload.Action)
		}
	default:
		return fmt.Errorf("%w: unsupported action %s", ErrInvalidProjectorPayload, payload.Action)
	}
	return nil
}

func validProjectedGroupOwnership(
	group *groupevent.GroupSnapshot,
	members []groupevent.GroupMemberSnapshot,
) bool {
	if group == nil || group.OwnerUUID == "" || len(members) == 0 {
		return false
	}
	ownerCount := 0
	for _, member := range members {
		if member.Role != int32(memberRoleOwner) {
			continue
		}
		if member.UserUUID != group.OwnerUUID {
			return false
		}
		ownerCount++
	}
	return ownerCount == 1
}

func allProjectedMembersHaveRole(members []groupevent.GroupMemberSnapshot, role int8) bool {
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if member.Role != int32(role) {
			return false
		}
	}
	return true
}

func validProjectedOwnerTransfer(
	group *groupevent.GroupSnapshot,
	members []groupevent.GroupMemberSnapshot,
) bool {
	if group == nil || len(members) != 2 || !validProjectedGroupOwnership(group, members) {
		return false
	}
	// TransferGroupOwner 的领域规则会把旧群主直接降为普通成员，而不是管理员。
	// 因而最终快照必须恰好包含“新群主 + 旧群主（普通成员）”两条记录。
	memberCount := 0
	for _, member := range members {
		if member.Role == int32(memberRoleMember) {
			memberCount++
		}
	}
	return memberCount == 1
}

func validProjectedMemberSnapshots(members []groupevent.GroupMemberSnapshot) bool {
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if member.UserUUID == "" ||
			member.Role < int32(memberRoleMember) ||
			member.Role > int32(memberRoleOwner) ||
			member.MuteUntilUnixMs < 0 ||
			member.JoinedAtUnixMs <= 0 {
			return false
		}
	}
	return true
}

func validUniqueUUIDs(userUUIDs []string) bool {
	if len(userUUIDs) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			return false
		}
		if _, duplicate := seen[userUUID]; duplicate {
			return false
		}
		seen[userUUID] = struct{}{}
	}
	return true
}

// sameProjectedMemberSet 强制校验 members 与 user_uuids 表达同一集合。
//
// 旧实现会在 user_uuids 缺失时偷偷从 members 推导，这会掩盖生产端契约错误；
// v2 明确拒绝这种兼容路径，保证反向索引的目标用户集合没有第二种解释。
func sameProjectedMemberSet(members []groupevent.GroupMemberSnapshot, userUUIDs []string) bool {
	if len(members) == 0 || len(userUUIDs) == 0 {
		return false
	}
	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member.UserUUID == "" {
			return false
		}
		if _, duplicate := memberSet[member.UserUUID]; duplicate {
			return false
		}
		memberSet[member.UserUUID] = struct{}{}
	}
	userSet := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			return false
		}
		if _, duplicate := userSet[userUUID]; duplicate {
			return false
		}
		userSet[userUUID] = struct{}{}
	}
	if len(memberSet) != len(userSet) {
		return false
	}
	for userUUID := range memberSet {
		if _, exists := userSet[userUUID]; !exists {
			return false
		}
	}
	return true
}

// applyGroupCreatedEvent 处理建群后的首次缓存投影。
//
// 这里允许直接完整创建主缓存，原因是：
//  1. group_created 自带完整群快照和首批成员快照；
//  2. 首次建缓存比增量 patch 更稳定；
//  3. user_groups 会创建逐群版本 tombstone，但不写 READY，避免局部事件伪装成完整列表。
func (r *groupRepositoryImpl) applyGroupCreatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	group := buildGroupInfoFromSnapshot(payload.Group)
	// projector 消费到 group_created 时 DB 事实已经存在，这里只做 Bloom 自愈补写；
	// 失败不能阻断主缓存投影，否则会影响 group:info/group:members 的正常重建。
	r.addGroupUUIDToBloomBestEffort(ctx, group.Uuid)
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		group,
		payload.ProjectionVersion,
		groupInfoCreateIfMissing,
	); err != nil {
		return err
	}
	members := buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members)
	if err := r.replaceGroupMembersProjection(
		ctx,
		payload.GroupUUID,
		members,
		payload.ProjectionVersion,
		false,
	); err != nil {
		return err
	}
	return r.patchUserGroupsProjection(ctx, payload.UserUUIDs, group, true, payload.ProjectionVersion)
}

// applyMemberAddedEvent 处理成员新增/恢复后的缓存投影。
//
// 这里统一使用 upsert patch，而不是区分 insert / restore 两条路径，原因是：
//  1. 事件里已经携带最终成员快照；
//  2. 恢复成员和新增成员都可以安全落成“当前最新字段值”；
//  3. key 不存在时脚本会自动跳过，仍由读路径负责全量重建。
func (r *groupRepositoryImpl) applyMemberAddedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	group := buildGroupInfoFromSnapshot(payload.Group)
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		group,
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	if err := r.upsertGroupMembersProjectionIfExists(
		ctx,
		payload.GroupUUID,
		buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members),
		payload.ProjectionVersion,
	); err != nil {
		return err
	}
	return r.patchUserGroupsProjection(ctx, payload.UserUUIDs, group, true, payload.ProjectionVersion)
}

// applyMemberRemovedEvent 处理成员退群/被踢后的缓存投影。
//
// 删除路径只需要同步三类缓存：
//  1. 群资料里的 member_count；
//  2. 群成员 Hash 中的单个成员 field；
//  3. 目标用户的 user_groups 反向索引。
func (r *groupRepositoryImpl) applyMemberRemovedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	group := buildGroupInfoFromSnapshot(payload.Group)
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		group,
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	if err := r.removeGroupMemberProjectionIfExists(
		ctx,
		payload.GroupUUID,
		payload.UserUUID,
		payload.ProjectionVersion,
	); err != nil {
		return err
	}
	return r.patchUserGroupProjection(
		ctx,
		payload.UserUUID,
		group,
		false,
		payload.ProjectionVersion,
		false,
	)
}

// applyGroupDismissedEvent 处理群解散后的缓存投影。
//
// 群解散后：
//  1. `group:info` 若存在则补成 status=2；
//  2. `group:members` 原子替换为带删除版本的空 Hash，拒绝旧 member_added 复活成员；
//  3. 每个活跃成员的 user_groups 写入逐群删除 tombstone。
func (r *groupRepositoryImpl) applyGroupDismissedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	group := buildGroupInfoFromSnapshot(payload.Group)
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		group,
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	if err := r.replaceGroupMembersProjection(
		ctx,
		payload.GroupUUID,
		[]*model.GroupMember{},
		payload.ProjectionVersion,
		false,
	); err != nil {
		return err
	}
	return r.patchUserGroupsProjection(ctx, payload.UserUUIDs, group, false, payload.ProjectionVersion)
}

// applyGroupInfoUpdatedEvent 处理群资料变更后的缓存投影。
//
// 第二批资料更新只改 `group:info` 主缓存，不主动触碰 members / user_groups，
// 因为成员结构与反向索引都不依赖 notice / add_mode 这些资料字段。
func (r *groupRepositoryImpl) applyGroupInfoUpdatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.setVersionedGroupInfoProjection(
		ctx,
		buildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	)
}

// applyGroupMuteSettingUpdatedEvent 处理全员禁言开关变更后的缓存投影。
//
// 全员禁言只影响群级发送策略，因此这里只刷新 group:info；
// 成员角色和单人禁言仍保留在 group:members，由发送权限检查组合判断。
func (r *groupRepositoryImpl) applyGroupMuteSettingUpdatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.setVersionedGroupInfoProjection(
		ctx,
		buildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	)
}

// applyOwnerTransferredEvent 处理群主转让后的缓存投影。
//
// 该事件需要同步两类缓存：
//  1. `group:info` 里的 owner_uuid；
//  2. `group:members` 中老群主和新群主的 role。
func (r *groupRepositoryImpl) applyOwnerTransferredEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		buildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	return r.upsertGroupMembersProjectionIfExists(
		ctx,
		payload.GroupUUID,
		buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members),
		payload.ProjectionVersion,
	)
}

// applyMemberRoleUpdatedEvent 处理成员角色变更后的缓存投影。
//
// 角色变更不会改变成员集合和 user_groups 反向索引，
// 因此这里只 patch 群资料主缓存与目标成员 role 字段。
func (r *groupRepositoryImpl) applyMemberRoleUpdatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		buildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	return r.upsertGroupMembersProjectionIfExists(
		ctx,
		payload.GroupUUID,
		buildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members),
		payload.ProjectionVersion,
	)
}

// applyMemberProfileUpdatedEvent 处理成员群名片变更后的缓存投影。
//
// 该事件与角色更新一样只 patch 受影响成员字段，避免为了单个群名片更新重建整个成员 Hash。
func (r *groupRepositoryImpl) applyMemberProfileUpdatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.applyMemberRoleUpdatedEvent(ctx, payload)
}

// applyMemberMutedEvent 处理成员单人禁言变更后的缓存投影。
//
// 单人禁言是成员维度权限事实，复用成员 patch 逻辑即可同步 mute_until 到 Redis。
func (r *groupRepositoryImpl) applyMemberMutedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.applyMemberRoleUpdatedEvent(ctx, payload)
}

// applyJoinRequestCreatedEvent 处理新增待审批入群申请后的缓存投影。
//
// 待审批申请列表缓存和成员缓存一样遵循 patch-if-exists：
//  1. 缓存已存在时增量写入单条申请；
//  2. 缓存不存在时直接跳过，继续由读路径负责全量重建；
//  3. 申请展示资料仍由上层聚合 user_profile，不在这里冗余缓存。
func (r *groupRepositoryImpl) applyJoinRequestCreatedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	return r.upsertGroupJoinRequestProjectionIfExists(
		ctx,
		payload.GroupUUID,
		buildGroupJoinRequestFromSnapshot(payload.JoinRequest),
		payload.ProjectionVersion,
	)
}

// applyJoinRequestReviewedEvent 处理待审批入群申请被通过或拒绝后的缓存投影。
//
// 审批完成后，这条申请不应继续停留在“待审批列表”缓存里，
// 因此这里直接按 apply_id 删除对应 field；若缓存缺失则继续保持跳过语义。
func (r *groupRepositoryImpl) applyJoinRequestReviewedEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if payload.JoinRequest == nil {
		return fmt.Errorf("%w: join_request_reviewed missing join request snapshot", ErrInvalidProjectorPayload)
	}
	return r.removeGroupJoinRequestProjectionIfExists(
		ctx,
		payload.GroupUUID,
		payload.JoinRequest.ApplyID,
		payload.ProjectionVersion,
	)
}

// patchUserGroupsProjection 对同一事件的目标用户逐个写入版本化反向索引。
//
// 每个用户对应独立 Redis key，无法跨用户做单条 Lua；但每个用户内部的 ZSet 与
// 版本 Hash 都由 Lua 原子更新。事件重试时，已经成功的用户会因版本相等直接跳过，
// 尚未成功的用户继续完成，不会产生部分重试副作用。
func (r *groupRepositoryImpl) patchUserGroupsProjection(
	ctx context.Context,
	userUUIDs []string,
	group *model.GroupInfo,
	active bool,
	projectionVersion int64,
) error {
	for _, userUUID := range userUUIDs {
		if err := r.patchUserGroupProjection(
			ctx,
			userUUID,
			group,
			active,
			projectionVersion,
			false,
		); err != nil {
			return err
		}
	}
	return nil
}
