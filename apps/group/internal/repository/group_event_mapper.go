package repository

import (
	"context"
	"fmt"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	idutil "github.com/013677890/LCchat-Backend/pkg/util"
	"gorm.io/gorm"
)

// insertGroupCreatedEvent 把“建群成功”事实写入统一的 group.cache outbox 事件。
//
// 这里直接带上群快照和初始成员快照，原因是：
//  1. projector 在消费时不需要再回源 MySQL；
//  2. 创建群是最适合一次性建立主缓存的时机；
//  3. 事件 payload 完整，后续重放也更稳定。
func (r *groupRepositoryImpl) insertGroupCreatedEvent(tx *gorm.DB, group *model.GroupInfo, members []*model.GroupMember) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:    groupevent.ActionGroupCreated,
		GroupUUID: group.Uuid,
		Group:     buildGroupSnapshot(group),
		Members:   buildGroupMemberSnapshots(members),
		UserUUIDs: collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberAddedEvent 把“新增/恢复成员”事实写入 group.cache outbox。
//
// 这里统一传 members 快照而不是只传 UUID，目的是让 projector 能区分：
//  1. 新增普通成员；
//  2. 恢复历史成员；
//  3. JoinedAt 等需要同步进成员 Hash 的字段。
func (r *groupRepositoryImpl) insertMemberAddedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberAdded,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberRemovedEvent 把“移除成员/退群”事实写入 group.cache outbox。
//
// 这里不再携带完整成员快照，因为删除路径只需要知道：
//  1. 哪个群发生了删除；
//  2. 哪个用户需要从成员缓存和用户群列表里移除。
func (r *groupRepositoryImpl) insertMemberRemovedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID, targetUUID string) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberRemoved,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		UserUUID:     targetUUID,
	})
}

// insertGroupDismissedEvent 把“群解散”事实写入 group.cache outbox。
//
// 这里额外带上活跃成员 UUID 列表，是为了让 projector 能按 patch-if-exists
// 规则把每个用户的 user_groups 反向索引删掉，而不需要再查库。
func (r *groupRepositoryImpl) insertGroupDismissedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, userUUIDs []string) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionGroupDismissed,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		UserUUIDs:    userUUIDs,
	})
}

// insertGroupInfoUpdatedEvent 把“群资料更新”事实写入 group.cache outbox。
//
// 这里只传最新群快照，不单独传变更字段集合，原因是 projector 做的是
// 最终状态投影，直接覆盖最新快照比维护字段级 diff 更简单也更稳。
func (r *groupRepositoryImpl) insertGroupInfoUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionGroupInfoUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
	})
}

// insertOwnerTransferredEvent 把“群主转让”事实写入 group.cache outbox。
//
// 这里同时带上最新群快照与受影响成员快照，原因是：
//  1. group:info 需要同步新的 owner_uuid；
//  2. group:members 需要同步老群主与新群主的 role；
//  3. projector 可以直接按最终态覆盖，不必回源数据库二次查询。
func (r *groupRepositoryImpl) insertOwnerTransferredEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionOwnerTransferred,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberRoleUpdatedEvent 把“成员角色更新”事实写入 group.cache outbox。
//
// 事件里保留目标成员最终 role，便于 projector 直接 patch 成员 Hash，
// 同时复用群快照刷新 group:info 的最新 updated_at。
func (r *groupRepositoryImpl) insertMemberRoleUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberRoleUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberProfileUpdatedEvent 把“成员群名片更新”事实写入 group.cache outbox。
//
// 群名片属于成员在单个群内的个人资料，不应刷新整组成员缓存；
// 事件仅携带受影响成员最终快照，让 projector 按 patch-if-exists 更新现有 Hash。
func (r *groupRepositoryImpl) insertMemberProfileUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberProfileUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberMutedEvent 把“成员单人禁言更新”事实写入 group.cache outbox。
//
// 禁言是消息发送链路的高频权限事实，必须随写事务进入 outbox，
// 避免 msg-service 读到旧缓存后错误放行或错误拦截成员发言。
func (r *groupRepositoryImpl) insertMemberMutedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberMuted,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertGroupMuteSettingUpdatedEvent 把“全员禁言开关更新”事实写入 group.cache outbox。
//
// 全员禁言属于群级策略，只需要携带群快照即可；成员缓存不需要变更，
// 发送权限检查会同时读取 group:info 与成员角色来判断管理员豁免规则。
func (r *groupRepositoryImpl) insertGroupMuteSettingUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionGroupMuteSettingUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
	})
}

// insertJoinRequestCreatedEvent 把“新增待审批入群申请”事实写入 group.cache outbox。
//
// 申请列表缓存本质上也是群维度投影，因此继续复用同一个 group.cache topic，
// 让申请创建、审批处理与成员正式入群在同一 group_uuid 分区内保持顺序。
func (r *groupRepositoryImpl) insertJoinRequestCreatedEvent(tx *gorm.DB, groupUUID, operatorUUID string, request *model.GroupJoinRequest) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionJoinRequestCreated,
		GroupUUID:    groupUUID,
		OperatorUUID: operatorUUID,
		JoinRequest:  buildGroupJoinRequestSnapshot(request),
	})
}

// insertJoinRequestReviewedEvent 把“待审批入群申请已处理”事实写入 group.cache outbox。
//
// projector 只需要知道哪条待审批申请应被移除，因此这里传递申请快照即可，
// 避免消费侧为了删缓存再次回源 MySQL 查询申请记录。
func (r *groupRepositoryImpl) insertJoinRequestReviewedEvent(tx *gorm.DB, groupUUID, operatorUUID string, request *model.GroupJoinRequest) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionJoinRequestReviewed,
		GroupUUID:    groupUUID,
		OperatorUUID: operatorUUID,
		JoinRequest:  buildGroupJoinRequestSnapshot(request),
	})
}

// insertJoinRequestCanceledEvent 把“申请人主动撤销待审批入群申请”事实写入 group.cache outbox。
//
// 撤销后待审批缓存需要移除对应申请，但历史申请记录仍需保留供“我的申请状态”查询，
// 因此这里继续复用申请快照表达“移出待审批集合”的最小事实。
func (r *groupRepositoryImpl) insertJoinRequestCanceledEvent(tx *gorm.DB, groupUUID, operatorUUID string, request *model.GroupJoinRequest) error {
	return r.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionJoinRequestCanceled,
		GroupUUID:    groupUUID,
		OperatorUUID: operatorUUID,
		JoinRequest:  buildGroupJoinRequestSnapshot(request),
	})
}

// insertGroupCacheEvent 统一封装 group.cache 事件的编码与落库。
//
// 关键约束：
//  1. event_type 固定为 group.cache，保证同群事件落到同一 Kafka topic；
//  2. entity_id 固定为 group_uuid，保证同群事件按 key 有序；
//  3. event_id 在这里兜底生成，避免调用方漏填导致幂等链路失效。
func (r *groupRepositoryImpl) insertGroupCacheEvent(tx *gorm.DB, payload groupevent.GroupCacheEventPayload) error {
	if tx == nil || payload.GroupUUID == "" || payload.Action == "" {
		return fmt.Errorf("%w: invalid group cache event payload", ErrDatabase)
	}
	if payload.EventID == "" {
		payload.EventID = idutil.GenIDString()
	}
	encoded, err := groupevent.Encode(payload)
	if err != nil {
		return fmt.Errorf("编码群缓存事件失败: %w", err)
	}
	if err := outbox.InsertEvent(tx, groupevent.EventTypeGroupCache, payload.GroupUUID, encoded); err != nil {
		return WrapDBError(err)
	}
	return nil
}

// buildGroupSnapshot 把领域模型转换为事件快照。
//
// 这里记录 UpdatedAtUnix，是为了让 projector 在维护 user_groups ZSet 时
// 复用同一份“最近更新时间”作为 score，保持读排序和 DB 一致。
func buildGroupSnapshot(group *model.GroupInfo) *groupevent.GroupSnapshot {
	if group == nil {
		return nil
	}
	return &groupevent.GroupSnapshot{
		GroupUUID:     group.Uuid,
		Name:          group.Name,
		Avatar:        group.Avatar,
		Notice:        group.Notice,
		OwnerUUID:     group.OwnerUuid,
		MemberCount:   int32(group.MemberCnt),
		AddMode:       int32(group.AddMode),
		MuteAll:       group.MuteAll,
		Status:        int32(group.Status),
		UpdatedAtUnix: group.UpdatedAt.Unix(),
	}
}

// buildGroupMemberSnapshots 把成员模型批量转换为事件快照。
//
// 这里做去重是为了防止：
//  1. 调用方误传重复成员；
//  2. 恢复成员和新增成员在组合切片时出现重复；
//  3. projector 处理时重复 patch 同一用户。
func buildGroupMemberSnapshots(members []*model.GroupMember) []groupevent.GroupMemberSnapshot {
	if len(members) == 0 {
		return []groupevent.GroupMemberSnapshot{}
	}
	result := make([]groupevent.GroupMemberSnapshot, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		if _, exists := seen[member.UserUuid]; exists {
			continue
		}
		seen[member.UserUuid] = struct{}{}
		result = append(result, groupevent.GroupMemberSnapshot{
			UserUUID:        member.UserUuid,
			Role:            int32(member.Role),
			Remark:          member.Remark,
			JoinedAtUnixMs:  member.JoinedAt.UnixMilli(),
			MuteUntilUnixMs: buildMuteUntilUnixMs(member),
		})
	}
	return result
}

// buildMuteUntilUnixMs 把可空禁言时间转换为事件里的毫秒时间戳。
func buildMuteUntilUnixMs(member *model.GroupMember) int64 {
	if member == nil || member.MuteUntil == nil {
		return 0
	}
	return member.MuteUntil.UnixMilli()
}

// buildGroupJoinRequestSnapshot 把入群申请模型转换为事件快照。
func buildGroupJoinRequestSnapshot(request *model.GroupJoinRequest) *groupevent.GroupJoinRequestSnapshot {
	if request == nil || request.Id <= 0 || request.ApplicantUuid == "" {
		return nil
	}
	return &groupevent.GroupJoinRequestSnapshot{
		ApplyID:         request.Id,
		ApplicantUUID:   request.ApplicantUuid,
		Reason:          request.Reason,
		CreatedAtUnixMs: request.CreatedAt.UnixMilli(),
	}
}

// collectGroupMemberSnapshotUUIDs 提取成员 UUID 列表，供 user_groups 反向索引 patch 使用。
func collectGroupMemberSnapshotUUIDs(members []*model.GroupMember) []string {
	if len(members) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		if _, exists := seen[member.UserUuid]; exists {
			continue
		}
		seen[member.UserUuid] = struct{}{}
		result = append(result, member.UserUuid)
	}
	return result
}

// loadActiveMemberUUIDs 在事务内加载当前群的活跃成员 UUID。
//
// 这个方法主要服务于 group_dismissed 事件：
//  1. 先在事务里拿到一份稳定成员快照；
//  2. 再写出 outbox；
//  3. 避免事务提交后再查库导致投影成员集与解散事实错位。
func (r *groupRepositoryImpl) loadActiveMemberUUIDs(ctx context.Context, tx *gorm.DB, groupUUID string) ([]string, error) {
	if tx == nil || groupUUID == "" {
		return []string{}, nil
	}
	var members []*model.GroupMember
	if err := tx.WithContext(ctx).
		Select("user_uuid").
		Where("group_uuid = ? AND status = ? AND deleted_at IS NULL", groupUUID, memberStatusNormal).
		Find(&members).Error; err != nil {
		return nil, WrapDBError(err)
	}
	userUUIDs := make([]string, 0, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		userUUIDs = append(userUUIDs, member.UserUuid)
	}
	return userUUIDs, nil
}
