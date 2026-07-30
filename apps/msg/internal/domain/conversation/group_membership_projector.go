package conversation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/outbox"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const groupMembershipProjectorEventType = groupevent.EventTypeGroupCache + ":msg-membership-projector"

var (
	// ErrInvalidGroupProjectionEvent 表示消息不符合当前 group.cache 严格契约。
	// 这种错误重试不会自行恢复，consumer 必须把原始消息写入死信后再提交 offset。
	ErrInvalidGroupProjectionEvent = errors.New("conversation: invalid group projection event")
	// ErrGroupProjectionVersionGap 表示同一 group_uuid 的事件版本不连续。
	// group.cache 以 group_uuid 为 Kafka key，正常消费不应出现缺口；出现缺口说明
	// topic/offset/生产链路已经破坏，禁止跳过版本继续构造一个无法证明完整的成员视图。
	ErrGroupProjectionVersionGap = errors.New("conversation: group projection version gap")
)

// GroupMembershipProjectorRepository 是 msg-service 对 group.cache 的本地投影能力。
//
// group-service 仍是成员关系的唯一事实源；这里保存的 conversation.Membership* 与
// group_conversation.GroupStatus 只是 msg 会话列表使用的可重建投影。
type GroupMembershipProjectorRepository interface {
	ApplyGroupCacheEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error
}

// NewGroupMembershipProjectorRepository 创建群成员会话投影仓储。
// 单独暴露构造器可让 Wire 为前台会话 Repository 和后台 projector 提供不同接口，
// 两者仍共享同一个 *gorm.DB 连接池和相同的表所有权。
func NewGroupMembershipProjectorRepository(db *gorm.DB) GroupMembershipProjectorRepository {
	return &repositoryImpl{db: db}
}

// ApplyGroupCacheEvent 在一个 MySQL 事务内完成版本栅栏、成员状态投影和消费幂等标记。
//
// 事务边界非常重要：如果成员行已更新而幂等记录尚未写入就崩溃，Kafka 会重投；
// 把两者放在同一事务后，重投要么看到完整结果并跳过，要么重新执行完整投影，
// 不会留下“成员状态已变、消费记录未变”的半完成窗口。
func (r *repositoryImpl) ApplyGroupCacheEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("ApplyGroupCacheEvent: db 未初始化")
	}
	if err := validateMsgGroupProjectionEvent(payload); err != nil {
		return err
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		processed, err := outbox.CheckIdempotent(tx, groupMembershipProjectorEventType, payload.EventID)
		if err != nil {
			return fmt.Errorf("ApplyGroupCacheEvent: 检查幂等记录失败: %w", err)
		}
		if processed {
			return nil
		}

		groupState, err := lockGroupProjectionState(tx, payload.GroupUUID)
		if err != nil {
			return err
		}
		switch {
		case payload.ProjectionVersion <= groupState.ProjectionVersion:
			// Kafka offset 提交失败会重投已经完成的事件。event_id 通常会先命中，
			// 版本栅栏是第二道幂等保护，绝不让旧状态覆盖当前成员关系。
			return markGroupProjectionEventProcessed(tx, payload.EventID)
		case payload.ProjectionVersion != groupState.ProjectionVersion+1:
			return fmt.Errorf(
				"%w: group_uuid=%s current=%d incoming=%d",
				ErrGroupProjectionVersionGap,
				payload.GroupUUID,
				groupState.ProjectionVersion,
				payload.ProjectionVersion,
			)
		}

		if err := validateMsgGroupProjectionTransition(groupState, payload); err != nil {
			return err
		}

		if err := applyGroupMembershipAction(tx, payload); err != nil {
			return err
		}
		if err := advanceGroupProjectionState(tx, groupState, payload); err != nil {
			return err
		}
		return markGroupProjectionEventProcessed(tx, payload.EventID)
	})
}

// validateMsgGroupProjectionTransition 校验单群投影状态机，而不只校验单条事件字段。
//
// 当前数据库从空库直接使用 v2 基线：一个群的首条事件必须是 version=1 的
// group_created，已建立的群不能再次 created，dismissed 之后也不能继续产生新事实。
// 这里不尝试把中途接入的 member_added 当成初始化事件，也不为缺失的建群历史回源补全；
// 任何违反状态机的消息都作为生产链路错误进入死信。
func validateMsgGroupProjectionTransition(
	current *model.GroupConversation,
	payload groupevent.GroupCacheEventPayload,
) error {
	if current == nil {
		return fmt.Errorf("%w: missing current group state", ErrInvalidGroupProjectionEvent)
	}
	switch {
	case current.ProjectionVersion == 0 && payload.Action != groupevent.ActionGroupCreated:
		return fmt.Errorf(
			"%w: group_uuid=%s first action must be %s, got %s",
			ErrInvalidGroupProjectionEvent,
			payload.GroupUUID,
			groupevent.ActionGroupCreated,
			payload.Action,
		)
	case current.ProjectionVersion > 0 && payload.Action == groupevent.ActionGroupCreated:
		return fmt.Errorf(
			"%w: group_uuid=%s cannot be created twice",
			ErrInvalidGroupProjectionEvent,
			payload.GroupUUID,
		)
	case current.GroupStatus == model.GroupConversationStatusDismissed:
		return fmt.Errorf(
			"%w: group_uuid=%s is already dismissed",
			ErrInvalidGroupProjectionEvent,
			payload.GroupUUID,
		)
	default:
		return nil
	}
}

// lockGroupProjectionState 先确保共享群行存在，再对该 group_uuid 加行锁。
// 同一群的 Kafka 事件由版本串行推进；消息发送侧只更新 max_seq/last_msg_*，
// 不触碰 group_status/projection_version，因此两个写路径可以安全共用此行。
func lockGroupProjectionState(tx *gorm.DB, groupUUID string) (*model.GroupConversation, error) {
	seed := &model.GroupConversation{
		GroupUuid:   groupUUID,
		GroupStatus: model.GroupConversationStatusNormal,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(seed).Error; err != nil {
		return nil, fmt.Errorf("创建群投影状态行失败: %w", err)
	}

	var state model.GroupConversation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_uuid = ?", groupUUID).
		First(&state).Error; err != nil {
		return nil, fmt.Errorf("锁定群投影状态行失败: %w", err)
	}
	return &state, nil
}

func applyGroupMembershipAction(tx *gorm.DB, payload groupevent.GroupCacheEventPayload) error {
	switch payload.Action {
	case groupevent.ActionGroupCreated, groupevent.ActionMemberAdded:
		return activateProjectedGroupMembers(tx, payload)
	case groupevent.ActionMemberRemoved:
		return deactivateProjectedGroupMember(tx, payload)
	case groupevent.ActionGroupDismissed,
		groupevent.ActionGroupInfoUpdated,
		groupevent.ActionOwnerTransferred,
		groupevent.ActionMemberRoleUpdated,
		groupevent.ActionMemberProfileUpdated,
		groupevent.ActionMemberMuted,
		groupevent.ActionGroupMuteSettingUpdated,
		groupevent.ActionJoinRequestCreated,
		groupevent.ActionJoinRequestReviewed,
		groupevent.ActionJoinRequestCanceled:
		// 这些 action 不改变 msg 的逐成员集合，但仍必须推进单群版本。
		// 如果直接忽略 offset，下一条真正的成员事件就会被误判为版本缺口。
		return nil
	default:
		return fmt.Errorf("%w: unsupported action %q", ErrInvalidGroupProjectionEvent, payload.Action)
	}
}

func activateProjectedGroupMembers(tx *gorm.DB, payload groupevent.GroupCacheEventPayload) error {
	rows := make([]*model.Conversation, 0, len(payload.Members))
	appliedAt := time.Now()
	for _, member := range payload.Members {
		joinedAt := time.UnixMilli(member.JoinedAtUnixMs)
		rows = append(rows, &model.Conversation{
			ConvId:             payload.GroupUUID,
			Type:               2, // GROUP，与 msg.proto ConvType 保持一致。
			OwnerUuid:          member.UserUUID,
			TargetUuid:         payload.GroupUUID,
			MembershipStatus:   model.ConversationMembershipActive,
			MembershipVersion:  payload.ProjectionVersion,
			MembershipJoinedAt: &joinedAt,
			MembershipLeftAt:   nil,
			Status:             0,
			UpdatedAt:          appliedAt,
		})
	}

	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner_uuid"}, {Name: "target_uuid"}},

		// 单群 group_conversation 已持 FOR UPDATE 锁并验证 incoming=current+1，因此
		// 这里可以一次批量 upsert 当前事件的权威 Membership*，无需再为 K 个成员发 K
		// 条 UPDATE。AssignmentColumns 只覆盖成员投影字段与通用 updated_at，个人
		// read/clear/mute/pin/status 始终保留。
		DoUpdates: clause.AssignmentColumns([]string{
			"membership_status",
			"membership_version",
			"membership_joined_at",
			"membership_left_at",
			"updated_at",
		}),
	}).CreateInBatches(rows, 100).Error; err != nil {
		return fmt.Errorf("批量激活群成员会话投影失败: %w", err)
	}
	return nil
}

func deactivateProjectedGroupMember(tx *gorm.DB, payload groupevent.GroupCacheEventPayload) error {
	leftAt := time.UnixMilli(payload.Group.UpdatedAtUnixMs)
	// 在当前空库基线和严格状态机下，被移除者必然已经由 group_created/member_added
	// 建立为 Active。这里故意不用 upsert：缺行、重复移除或成员行版本超前都说明投影
	// 历史不完整，不能靠凭空插入一条 tombstone 掩盖上游错误。
	result := tx.Model(&model.Conversation{}).
		Where(
			"owner_uuid = ? AND target_uuid = ? AND type = ? AND membership_status = ? AND membership_version < ?",
			payload.UserUUID,
			payload.GroupUUID,
			2, // GROUP
			model.ConversationMembershipActive,
			payload.ProjectionVersion,
		).
		Updates(map[string]interface{}{
			"membership_status":  model.ConversationMembershipLeft,
			"membership_version": payload.ProjectionVersion,
			"membership_left_at": leftAt,
			"updated_at":         time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("写入退群成员版本 tombstone 失败: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf(
			"%w: group_uuid=%s removed member %s is not an active projected member",
			ErrInvalidGroupProjectionEvent,
			payload.GroupUUID,
			payload.UserUUID,
		)
	}
	return nil
}

func advanceGroupProjectionState(
	tx *gorm.DB,
	current *model.GroupConversation,
	payload groupevent.GroupCacheEventPayload,
) error {
	updates := map[string]interface{}{
		"projection_version": payload.ProjectionVersion,
	}
	if payload.Group != nil {
		updates["group_status"] = int8(payload.Group.Status)
	}
	if payload.Action == groupevent.ActionGroupCreated {
		createdAt := time.UnixMilli(payload.Group.UpdatedAtUnixMs)

		// 没有消息的新群也需要稳定的列表排序位点。若消息发送已经抢先写入真实
		// last_msg_at，则保留真实消息时间，禁止较早的建群事件把活跃时间回退。
		updates["last_msg_at"] = gorm.Expr(
			"CASE WHEN last_msg_at IS NULL THEN ? ELSE last_msg_at END",
			createdAt,
		)
	}

	result := tx.Model(&model.GroupConversation{}).
		Where(
			"group_uuid = ? AND projection_version = ?",
			payload.GroupUUID,
			current.ProjectionVersion,
		).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("推进群会话投影版本失败: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("推进群会话投影版本失败: group_uuid=%s 版本被并发修改", payload.GroupUUID)
	}
	return nil
}

func markGroupProjectionEventProcessed(tx *gorm.DB, eventID string) error {
	if err := outbox.MarkIdempotent(tx, groupMembershipProjectorEventType, eventID); err != nil {
		return fmt.Errorf("写入群会话投影幂等记录失败: %w", err)
	}
	return nil
}

// validateMsgGroupProjectionEvent 把共享的 group.cache v2 契约错误映射为 msg projector
// 的永久错误类型。group Redis 与 msg 会话投影必须调用同一个校验器，避免一条事件在
// 两个消费者中出现不同解释。
func validateMsgGroupProjectionEvent(payload groupevent.GroupCacheEventPayload) error {
	if err := groupevent.ValidateGroupCachePayload(payload); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidGroupProjectionEvent, err)
	}
	return nil
}
