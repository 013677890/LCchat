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
			UserUUID:       member.UserUuid,
			Role:           int32(member.Role),
			JoinedAtUnixMs: member.JoinedAt.UnixMilli(),
		})
	}
	return result
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
