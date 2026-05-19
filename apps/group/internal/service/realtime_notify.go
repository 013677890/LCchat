package service

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/realtimepb"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
)

const groupRealtimePublishTimeout = 2 * time.Second

// selectGroupRealtimePublisher 取第一个非空实时提醒生产者，保持测试构造可按需省略。
func selectGroupRealtimePublisher(publishers []realtimepush.Publisher) realtimepush.Publisher {
	for _, publisher := range publishers {
		if publisher != nil {
			return publisher
		}
	}
	return nil
}

// publishGroupStateChangedToMembers 通知群成员按 changed 字段刷新本地群状态。
func (s *groupServiceImpl) publishGroupStateChangedToMembers(ctx context.Context, groupUUID string, changed []string) {
	payload := realtimepush.NewGroupStateChangedPayload(groupUUID, changed, 0)
	data, ok := buildGroupRealtimePayload(ctx, realtimepush.TypeGroupStateChanged, payload)
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeGroupStateChanged, realtimepush.NewGroupMembersTarget(groupUUID), data)
	publishGroupRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishGroupJoinRequestCreated 通知群管理员有新的入群申请待处理。
func (s *groupServiceImpl) publishGroupJoinRequestCreated(ctx context.Context, groupUUID, applicantUUID string, applyID int64) {
	data, ok := buildGroupRealtimePayload(ctx, realtimepush.TypeGroupJoinRequestCreated, &realtimepb.GroupJoinRequestCreatedPayload{
		GroupUuid:     groupUUID,
		ApplicantUuid: applicantUUID,
		ApplyId:       applyID,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeGroupJoinRequestCreated, realtimepush.NewGroupAdminsTarget(groupUUID), data)
	publishGroupRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishGroupJoinRequestReviewed 通知申请人入群审批结果。
func (s *groupServiceImpl) publishGroupJoinRequestReviewed(ctx context.Context, groupUUID, applicantUUID, reviewerUUID string, applyID int64, action int8) {
	data, ok := buildGroupRealtimePayload(ctx, realtimepush.TypeGroupJoinRequestReviewed, &realtimepb.GroupJoinRequestReviewedPayload{
		GroupUuid:     groupUUID,
		ApplicantUuid: applicantUUID,
		ReviewerUuid:  reviewerUUID,
		ApplyId:       applyID,
		Action:        int32(action),
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeGroupJoinRequestReviewed, realtimepush.NewUserTarget(applicantUUID), data)
	publishGroupRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishGroupMemberRemoved 通知被移出或主动退出的用户清理本地群状态。
func (s *groupServiceImpl) publishGroupMemberRemoved(ctx context.Context, groupUUID, operatorUUID, userUUID string) {
	data, ok := buildGroupRealtimePayload(ctx, realtimepush.TypeGroupMemberRemoved, &realtimepb.GroupMemberRemovedPayload{
		GroupUuid:    groupUUID,
		OperatorUuid: operatorUUID,
		UserUuid:     userUUID,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeGroupMemberRemoved, realtimepush.NewUserTarget(userUUID), data)
	publishGroupRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishGroupDismissed 通知群当前成员群已解散。
func (s *groupServiceImpl) publishGroupDismissed(ctx context.Context, groupUUID, operatorUUID string, userUUIDs []string) {
	data, ok := buildGroupRealtimePayload(ctx, realtimepush.TypeGroupDismissed, &realtimepb.GroupDismissedPayload{
		GroupUuid:    groupUUID,
		OperatorUuid: operatorUUID,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeGroupDismissed, realtimepush.NewUserListTarget(userUUIDs), data)
	publishGroupRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishGroupMemberMuted 通知目标成员禁言状态发生变化。
func (s *groupServiceImpl) publishGroupMemberMuted(ctx context.Context, groupUUID, userUUID string, muteUntil int64) {
	data, ok := buildGroupRealtimePayload(ctx, realtimepush.TypeGroupMemberMuted, &realtimepb.GroupMemberMutedPayload{
		GroupUuid: groupUUID,
		UserUuid:  userUUID,
		MuteUntil: muteUntil,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeGroupMemberMuted, realtimepush.NewUserTarget(userUUID), data)
	publishGroupRealtimeEvent(ctx, s.realtimePublisher, event)
}

// loadGroupMemberUUIDsForRealtime 尽力读取当前群成员，用于解散群等无法事后扩散的提醒场景。
func (s *groupServiceImpl) loadGroupMemberUUIDsForRealtime(ctx context.Context, groupUUID string) []string {
	if s == nil || s.groupRepo == nil || groupUUID == "" {
		return nil
	}
	members, err := s.groupRepo.GetGroupMembers(ctx, groupUUID)
	if err != nil {
		logger.Warn(ctx, "group 预加载群成员实时提醒目标失败，跳过该类提醒",
			logger.String("group_uuid", groupUUID),
			logger.ErrorField("error", err),
		)
		return nil
	}
	return collectMemberUUIDs(members)
}

// loadJoinRequestApplicantForRealtime 尽力读取待审批申请的申请人，用于审批后定向通知。
func (s *groupServiceImpl) loadJoinRequestApplicantForRealtime(ctx context.Context, groupUUID string, applyID int64) string {
	if s == nil || s.groupRepo == nil || groupUUID == "" || applyID <= 0 {
		return ""
	}
	applicantUUID, err := s.groupRepo.GetJoinRequestApplicant(ctx, groupUUID, applyID)
	if err != nil {
		logger.Warn(ctx, "group 预加载入群申请人失败，跳过审批结果定向提醒",
			logger.String("group_uuid", groupUUID),
			logger.Int64("apply_id", applyID),
			logger.ErrorField("error", err),
		)
		return ""
	}
	return applicantUUID
}

// publishGroupRealtimeEvent 异步投递 group 实时提醒，失败只记录降级日志，不影响主流程。
func publishGroupRealtimeEvent(ctx context.Context, publisher realtimepush.Publisher, event realtimepush.Event) {
	if publisher == nil {
		return
	}
	if async.Pool() == nil {
		if err := publisher.Publish(ctx, event); err != nil {
			logger.Warn(ctx, "group 发送实时提醒失败（不阻断）",
				logger.String("event_type", event.Type),
				logger.String("target_kind", string(event.Target.Kind)),
				logger.ErrorField("error", err),
			)
		}
		return
	}

	async.RunSafe(ctx, func(taskCtx context.Context) {
		if err := publisher.Publish(taskCtx, event); err != nil {
			logger.Warn(taskCtx, "group 发送实时提醒失败（不阻断）",
				logger.String("event_type", event.Type),
				logger.String("target_kind", string(event.Target.Kind)),
				logger.ErrorField("error", err),
			)
		}
	}, groupRealtimePublishTimeout)
}

// buildGroupRealtimePayload 将 group 负载编码为事件字节体，失败时跳过投递。
func buildGroupRealtimePayload(ctx context.Context, eventType string, payload any) ([]byte, bool) {
	data, err := realtimepush.EncodePayload(payload)
	if err != nil {
		logger.Warn(ctx, "group 编码实时提醒负载失败，跳过投递",
			logger.String("event_type", eventType),
			logger.ErrorField("error", err),
		)
		return nil, false
	}
	return data, true
}
