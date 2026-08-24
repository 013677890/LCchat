package projection

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/event"
)

// ApplyGroupCacheEvent 根据 group.cache 事件把最终事实投影到 Redis。
//
// 设计原则：
//  1. projector 只处理缓存，不做任何业务权限判断；
//  2. 所有 Redis 变更都必须携带 projection_version，并在 Lua 内拒绝旧版本；
//  3. group_created 可以首次完整创建缓存，普通增量事件只 patch 已存在的群维度缓存；
//  4. 用户群反向索引会留下逐群版本 tombstone，但只有完整对账才能写 READY；
//  5. 任意 Redis 可重试错误直接返回，由 Kafka 手动提交模式负责重试。
func (r *Repository) ApplyGroupCacheEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	if err := validateGroupCacheEventPayload(payload); err != nil {
		return err
	}
	switch payload.Action {
	case event.ActionGroupCreated:
		return r.applyGroupCreatedEvent(ctx, payload)
	case event.ActionMemberAdded:
		return r.applyMemberAddedEvent(ctx, payload)
	case event.ActionMemberRemoved:
		return r.applyMemberRemovedEvent(ctx, payload)
	case event.ActionGroupDismissed:
		return r.applyGroupDismissedEvent(ctx, payload)
	case event.ActionGroupInfoUpdated:
		return r.applyGroupInfoUpdatedEvent(ctx, payload)
	case event.ActionGroupMuteSettingUpdated:
		return r.applyGroupMuteSettingUpdatedEvent(ctx, payload)
	case event.ActionOwnerTransferred:
		return r.applyOwnerTransferredEvent(ctx, payload)
	case event.ActionMemberRoleUpdated:
		return r.applyMemberRoleUpdatedEvent(ctx, payload)
	case event.ActionMemberProfileUpdated:
		return r.applyMemberProfileUpdatedEvent(ctx, payload)
	case event.ActionMemberMuted:
		return r.applyMemberMutedEvent(ctx, payload)
	case event.ActionJoinRequestCreated:
		return r.applyJoinRequestCreatedEvent(ctx, payload)
	case event.ActionJoinRequestReviewed:
		return r.applyJoinRequestReviewedEvent(ctx, payload)
	case event.ActionJoinRequestCanceled:
		return r.applyJoinRequestReviewedEvent(ctx, payload)
	default:
		return fmt.Errorf("%w: unsupported action %s", repository.ErrInvalidProjectorPayload, payload.Action)
	}
}

// validateGroupCacheEventPayload 把共享的 group.cache v2 契约错误映射为 group projector
// 的永久错误类型。严格语义集中在 pkg/event，禁止 group 与 msg 各自维护一套规则。
func validateGroupCacheEventPayload(payload event.GroupCacheEventPayload) error {
	if err := event.ValidateGroupCachePayload(payload); err != nil {
		return fmt.Errorf("%w: %w", repository.ErrInvalidProjectorPayload, err)
	}
	return nil
}

// applyGroupCreatedEvent 处理建群后的首次缓存投影。
//
// 这里允许直接完整创建主缓存，原因是：
//  1. group_created 自带完整群快照和首批成员快照；
//  2. 首次建缓存比增量 patch 更稳定；
//  3. user_groups 会创建逐群版本 tombstone，但不写 READY，避免局部事件伪装成完整列表。
func (r *Repository) applyGroupCreatedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	group := repository.BuildGroupInfoFromSnapshot(payload.Group)
	// projector 消费到 group_created 时 DB 事实已经存在，这里只做 Bloom 自愈补写；
	// 失败不能阻断主缓存投影，否则会影响 group:info/group:members 的正常重建。
	repository.AddGroupUUIDToBloomBestEffort(ctx, r.redisClient, group.Uuid)
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		group,
		payload.ProjectionVersion,
		groupInfoCreateIfMissing,
	); err != nil {
		return err
	}
	members := repository.BuildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members)
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
func (r *Repository) applyMemberAddedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	group := repository.BuildGroupInfoFromSnapshot(payload.Group)
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
		repository.BuildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members),
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
func (r *Repository) applyMemberRemovedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	group := repository.BuildGroupInfoFromSnapshot(payload.Group)
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
func (r *Repository) applyGroupDismissedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	group := repository.BuildGroupInfoFromSnapshot(payload.Group)
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
func (r *Repository) applyGroupInfoUpdatedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	return r.setVersionedGroupInfoProjection(
		ctx,
		repository.BuildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	)
}

// applyGroupMuteSettingUpdatedEvent 处理全员禁言开关变更后的缓存投影。
//
// 全员禁言只影响群级发送策略，因此这里只刷新 group:info；
// 成员角色和单人禁言仍保留在 group:members，由发送权限检查组合判断。
func (r *Repository) applyGroupMuteSettingUpdatedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	return r.setVersionedGroupInfoProjection(
		ctx,
		repository.BuildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	)
}

// applyOwnerTransferredEvent 处理群主转让后的缓存投影。
//
// 该事件需要同步两类缓存：
//  1. `group:info` 里的 owner_uuid；
//  2. `group:members` 中老群主和新群主的 role。
func (r *Repository) applyOwnerTransferredEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		repository.BuildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	return r.upsertGroupMembersProjectionIfExists(
		ctx,
		payload.GroupUUID,
		repository.BuildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members),
		payload.ProjectionVersion,
	)
}

// applyMemberRoleUpdatedEvent 处理成员角色变更后的缓存投影。
//
// 角色变更不会改变成员集合和 user_groups 反向索引，
// 因此这里只 patch 群资料主缓存与目标成员 role 字段。
func (r *Repository) applyMemberRoleUpdatedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		repository.BuildGroupInfoFromSnapshot(payload.Group),
		payload.ProjectionVersion,
		groupInfoPatchExisting,
	); err != nil {
		return err
	}
	return r.upsertGroupMembersProjectionIfExists(
		ctx,
		payload.GroupUUID,
		repository.BuildGroupMembersFromSnapshots(payload.GroupUUID, payload.Members),
		payload.ProjectionVersion,
	)
}

// applyMemberProfileUpdatedEvent 处理成员群名片变更后的缓存投影。
//
// 该事件与角色更新一样只 patch 受影响成员字段，避免为了单个群名片更新重建整个成员 Hash。
func (r *Repository) applyMemberProfileUpdatedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	return r.applyMemberRoleUpdatedEvent(ctx, payload)
}

// applyMemberMutedEvent 处理成员单人禁言变更后的缓存投影。
//
// 单人禁言是成员维度权限事实，复用成员 patch 逻辑即可同步 mute_until 到 Redis。
func (r *Repository) applyMemberMutedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	return r.applyMemberRoleUpdatedEvent(ctx, payload)
}

// applyJoinRequestCreatedEvent 处理新增待审批入群申请后的缓存投影。
//
// 待审批申请列表缓存和成员缓存一样遵循 patch-if-exists：
//  1. 缓存已存在时增量写入单条申请；
//  2. 缓存不存在时直接跳过，继续由读路径负责全量重建；
//  3. 申请展示资料仍由上层聚合 user_profile，不在这里冗余缓存。
func (r *Repository) applyJoinRequestCreatedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	return r.upsertGroupJoinRequestProjectionIfExists(
		ctx,
		payload.GroupUUID,
		repository.BuildGroupJoinRequestFromSnapshot(payload.JoinRequest),
		payload.ProjectionVersion,
	)
}

// applyJoinRequestReviewedEvent 处理待审批入群申请被通过或拒绝后的缓存投影。
//
// 审批完成后，这条申请不应继续停留在“待审批列表”缓存里，
// 因此这里直接按 apply_id 删除对应 field；若缓存缺失则继续保持跳过语义。
func (r *Repository) applyJoinRequestReviewedEvent(ctx context.Context, payload event.GroupCacheEventPayload) error {
	if payload.JoinRequest == nil {
		return fmt.Errorf("%w: join_request_reviewed missing join request snapshot", repository.ErrInvalidProjectorPayload)
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
func (r *Repository) patchUserGroupsProjection(
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
