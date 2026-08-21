package store

import (
	"fmt"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	idutil "github.com/013677890/LCchat-Backend/pkg/util"
	"gorm.io/gorm"
)

// insertGroupCreatedEvent 把建群成功事实写入 group.cache Outbox 事件表。
func (s *Store) insertGroupCreatedEvent(tx *gorm.DB, group *model.GroupInfo, members []*model.GroupMember) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:    groupevent.ActionGroupCreated,
		GroupUUID: group.Uuid,
		Group:     buildGroupSnapshot(group),
		Members:   buildGroupMemberSnapshots(members),
		UserUUIDs: collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberAddedEvent 把新增/恢复成员事实写入 group.cache Outbox 事件表。
func (s *Store) insertMemberAddedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberAdded,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberRemovedEvent 把移除成员/退群事实写入 group.cache Outbox 事件表。
func (s *Store) insertMemberRemovedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID, targetUUID string) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberRemoved,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		UserUUID:     targetUUID,
	})
}

// insertGroupDismissedEvent 把群解散事实写入 group.cache Outbox 事件表。
func (s *Store) insertGroupDismissedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, userUUIDs []string) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionGroupDismissed,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		UserUUIDs:    userUUIDs,
	})
}

// insertGroupInfoUpdatedEvent 把群资料更新事实写入 group.cache Outbox 事件表。
func (s *Store) insertGroupInfoUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionGroupInfoUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
	})
}

// insertOwnerTransferredEvent 把群主转让事实写入 group.cache Outbox 事件表。
func (s *Store) insertOwnerTransferredEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionOwnerTransferred,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberRoleUpdatedEvent 把成员角色更新事实写入 group.cache Outbox 事件表。
func (s *Store) insertMemberRoleUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberRoleUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberProfileUpdatedEvent 把成员群名片更新事实写入 group.cache Outbox 事件表。
func (s *Store) insertMemberProfileUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberProfileUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertMemberMutedEvent 把成员单人禁言更新事实写入 group.cache Outbox 事件表。
func (s *Store) insertMemberMutedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string, members []*model.GroupMember) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionMemberMuted,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
		Members:      buildGroupMemberSnapshots(members),
		UserUUIDs:    collectGroupMemberSnapshotUUIDs(members),
	})
}

// insertGroupMuteSettingUpdatedEvent 把全员禁言开关更新事实写入 group.cache Outbox 事件表。
func (s *Store) insertGroupMuteSettingUpdatedEvent(tx *gorm.DB, group *model.GroupInfo, operatorUUID string) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionGroupMuteSettingUpdated,
		GroupUUID:    group.Uuid,
		OperatorUUID: operatorUUID,
		Group:        buildGroupSnapshot(group),
	})
}

// insertJoinRequestCreatedEvent 把新增待审批入群申请事实写入 group.cache Outbox 事件表。
func (s *Store) insertJoinRequestCreatedEvent(tx *gorm.DB, groupUUID, operatorUUID string, request *model.GroupJoinRequest) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionJoinRequestCreated,
		GroupUUID:    groupUUID,
		OperatorUUID: operatorUUID,
		JoinRequest:  buildGroupJoinRequestSnapshot(request),
	})
}

// insertJoinRequestReviewedEvent 把待审批入群申请已处理事实写入 group.cache Outbox 事件表。
func (s *Store) insertJoinRequestReviewedEvent(tx *gorm.DB, groupUUID, operatorUUID string, request *model.GroupJoinRequest) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionJoinRequestReviewed,
		GroupUUID:    groupUUID,
		OperatorUUID: operatorUUID,
		JoinRequest:  buildGroupJoinRequestSnapshot(request),
	})
}

// insertJoinRequestCanceledEvent 把申请人主动撤销待审批申请事实写入 group.cache Outbox 事件表。
func (s *Store) insertJoinRequestCanceledEvent(tx *gorm.DB, groupUUID, operatorUUID string, request *model.GroupJoinRequest) error {
	return s.insertGroupCacheEvent(tx, groupevent.GroupCacheEventPayload{
		Action:       groupevent.ActionJoinRequestCanceled,
		GroupUUID:    groupUUID,
		OperatorUUID: operatorUUID,
		JoinRequest:  buildGroupJoinRequestSnapshot(request),
	})
}

// insertGroupCacheEvent 统一封装 group.cache 事件的编码与 Outbox 表落库。
//
// 事务与版本严格保序机制：
//  1. 同事务递增版本：调用 nextGroupCacheProjectionVersion 原子将 `groups.cache_version + 1`，
//     并将递增后的版本赋值给 payload.ProjectionVersion；
//  2. 保证有序投递：event_type 固定为 group.cache，entity_id 固定为 group_uuid，
//     下游发布到 Kafka 时按 group_uuid 进行分区哈希，保证同一个群的所有事件在同一 Kafka 分区内绝对保序；
//  3. 契约校验：在落库前使用 groupevent.ValidateGroupCachePayload 进行严格强校验，拒绝任何非法快照。
func (s *Store) insertGroupCacheEvent(tx *gorm.DB, payload groupevent.GroupCacheEventPayload) error {
	if tx == nil || payload.GroupUUID == "" || payload.Action == "" {
		return fmt.Errorf("%w: invalid group cache event payload", repoerr.ErrDatabase)
	}
	// 1. 在当前写事务内为事件领取下一个严格递增的投影版本号
	projectionVersion, err := s.nextGroupCacheProjectionVersion(tx, payload.GroupUUID)
	if err != nil {
		return err
	}
	payload.SchemaVersion = groupevent.GroupCacheSchemaVersion
	payload.ProjectionVersion = projectionVersion
	if payload.EventID == "" {
		payload.EventID = idutil.GenIDString()
	}
	// 2. 校验 Payload 契约合法性
	if err := groupevent.ValidateGroupCachePayload(payload); err != nil {
		return fmt.Errorf("%w: %w", repository.ErrInvalidProjectorPayload, err)
	}
	encoded, err := groupevent.Encode(payload)
	if err != nil {
		return fmt.Errorf("编码群缓存事件失败: %w", err)
	}
	// 3. 将事件写入 MySQL outbox_events 表，同业务事务提交
	if err := outbox.InsertEvent(tx, groupevent.EventTypeGroupCache, payload.GroupUUID, encoded); err != nil {
		return repoerr.WrapDBError(err)
	}
	return nil
}

// nextGroupCacheProjectionVersion 在当前 MySQL 业务事务中为单条 group.cache 事件领取下一个递增版本号。
//
// 机制：通过 `UPDATE groups SET cache_version = cache_version + 1 WHERE uuid = ?` 原子领取新版本，
// 从而确保每次群业务写操作都拥有单调递增的序号，作为 Redis 投影的并发栅栏。
func (s *Store) nextGroupCacheProjectionVersion(tx *gorm.DB, groupUUID string) (int64, error) {
	if tx == nil || groupUUID == "" {
		return 0, fmt.Errorf("%w: invalid group cache projection version request", repoerr.ErrDatabase)
	}
	result := tx.Model(&model.GroupInfo{}).
		Where("uuid = ?", groupUUID).
		UpdateColumn("cache_version", gorm.Expr("cache_version + 1"))
	if result.Error != nil {
		return 0, repoerr.WrapDBError(result.Error)
	}
	if result.RowsAffected != 1 {
		return 0, fmt.Errorf("%w: group %s not found while allocating cache version", repoerr.ErrDatabase, groupUUID)
	}
	var projectionVersion int64
	if err := tx.Model(&model.GroupInfo{}).
		Select("cache_version").
		Where("uuid = ?", groupUUID).
		Scan(&projectionVersion).Error; err != nil {
		return 0, repoerr.WrapDBError(err)
	}
	if projectionVersion <= 0 {
		return 0, fmt.Errorf("%w: allocated invalid cache version %d", repoerr.ErrDatabase, projectionVersion)
	}
	return projectionVersion, nil
}

func buildGroupSnapshot(group *model.GroupInfo) *groupevent.GroupSnapshot {
	if group == nil {
		return nil
	}
	return &groupevent.GroupSnapshot{
		GroupID:         group.Id,
		GroupUUID:       group.Uuid,
		Name:            group.Name,
		Avatar:          group.Avatar,
		Notice:          group.Notice,
		OwnerUUID:       group.OwnerUuid,
		MemberCount:     int32(group.MemberCnt),
		AddMode:         int32(group.AddMode),
		MuteAll:         group.MuteAll,
		Status:          int32(group.Status),
		UpdatedAtUnixMs: group.UpdatedAt.UnixMilli(),
	}
}

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
		muteUntilUnixMs := int64(0)
		if member.MuteUntil != nil {
			muteUntilUnixMs = member.MuteUntil.UnixMilli()
		}
		result = append(result, groupevent.GroupMemberSnapshot{
			UserUUID:        member.UserUuid,
			Role:            int32(member.Role),
			Remark:          member.Remark,
			JoinedAtUnixMs:  member.JoinedAt.UnixMilli(),
			MuteUntilUnixMs: muteUntilUnixMs,
		})
	}
	return result
}

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
