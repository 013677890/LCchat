package service

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/realtimepb"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
)

const relationRealtimePublishTimeout = 2 * time.Second

const (
	friendChangeAdded            = "friend_added"
	friendChangeDeleted          = "friend_deleted"
	friendChangeRemarkUpdated    = "remark_updated"
	friendChangeTagUpdated       = "tag_updated"
	friendChangeBlacklistAdded   = "blacklist_added"
	friendChangeBlacklistRemoved = "blacklist_removed"
)

// publishFriendApplyCreated 通知目标用户收到新的好友申请。
func (s *friendServiceImpl) publishFriendApplyCreated(ctx context.Context, applyID int64, applicantUUID, targetUUID string) {
	data, ok := buildRealtimePayload(ctx, realtimepush.TypeFriendApplyCreated, &realtimepb.FriendApplyCreatedPayload{
		ApplyId:       applyID,
		ApplicantUuid: applicantUUID,
		TargetUuid:    targetUUID,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeFriendApplyCreated, realtimepush.NewUserTarget(targetUUID), data)
	publishRelationRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishFriendApplyHandled 通知申请发起人好友申请已被处理。
func (s *friendServiceImpl) publishFriendApplyHandled(ctx context.Context, applyID int64, applicantUUID, targetUUID string, action int32) {
	data, ok := buildRealtimePayload(ctx, realtimepush.TypeFriendApplyHandled, &realtimepb.FriendApplyHandledPayload{
		ApplyId:       applyID,
		ApplicantUuid: applicantUUID,
		TargetUuid:    targetUUID,
		Action:        action,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeFriendApplyHandled, realtimepush.NewUserTarget(applicantUUID), data)
	publishRelationRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishFriendRelationChangedToUsers 通知多用户关系状态发生变化。
func (s *friendServiceImpl) publishFriendRelationChangedToUsers(ctx context.Context, userUUIDs []string, changeType string) {
	data, ok := buildRealtimePayload(ctx, realtimepush.TypeFriendRelationChanged, &realtimepb.FriendRelationChangedPayload{
		UserUuids:  userUUIDs,
		ChangeType: changeType,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeFriendRelationChanged, realtimepush.NewUserListTarget(userUUIDs), data)
	publishRelationRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishFriendRelationChangedToUser 通知单用户关系视图需要刷新。
func (s *friendServiceImpl) publishFriendRelationChangedToUser(ctx context.Context, userUUID, peerUUID, changeType string) {
	data, ok := buildRealtimePayload(ctx, realtimepush.TypeFriendRelationChanged, &realtimepb.FriendRelationChangedPayload{
		UserUuid:   userUUID,
		PeerUuid:   peerUUID,
		ChangeType: changeType,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeFriendRelationChanged, realtimepush.NewUserTarget(userUUID), data)
	publishRelationRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishBlacklistRelationChanged 通知当前用户黑名单关系视图发生变化。
func (s *blacklistServiceImpl) publishBlacklistRelationChanged(ctx context.Context, userUUID, peerUUID, changeType string) {
	data, ok := buildRealtimePayload(ctx, realtimepush.TypeFriendRelationChanged, &realtimepb.FriendRelationChangedPayload{
		UserUuid:   userUUID,
		PeerUuid:   peerUUID,
		ChangeType: changeType,
	})
	if !ok {
		return
	}
	event := realtimepush.NewEvent(realtimepush.TypeFriendRelationChanged, realtimepush.NewUserTarget(userUUID), data)
	publishRelationRealtimeEvent(ctx, s.realtimePublisher, event)
}

// publishRelationRealtimeEvent 异步投递 relation 实时提醒，失败只记录降级日志，不影响主流程。
func publishRelationRealtimeEvent(ctx context.Context, publisher realtimepush.Publisher, event realtimepush.Event) {
	if publisher == nil {
		return
	}
	if async.Pool() == nil {
		if err := publisher.Publish(ctx, event); err != nil {
			logger.Warn(ctx, "relation 发送实时提醒失败（不阻断）",
				logger.String("event_type", event.Type),
				logger.String("target_kind", string(event.Target.Kind)),
				logger.ErrorField("error", err),
			)
		}
		return
	}

	async.RunSafe(ctx, func(taskCtx context.Context) {
		if err := publisher.Publish(taskCtx, event); err != nil {
			logger.Warn(taskCtx, "relation 发送实时提醒失败（不阻断）",
				logger.String("event_type", event.Type),
				logger.String("target_kind", string(event.Target.Kind)),
				logger.ErrorField("error", err),
			)
		}
	}, relationRealtimePublishTimeout)
}

// buildRealtimePayload 将 relation 负载转成实时提醒字节体，编码失败时跳过投递。
func buildRealtimePayload(ctx context.Context, eventType string, payload any) ([]byte, bool) {
	data, err := realtimepush.EncodePayload(payload)
	if err != nil {
		logger.Warn(ctx, "relation 编码实时提醒负载失败，跳过投递",
			logger.String("event_type", eventType),
			logger.ErrorField("error", err),
		)
		return nil, false
	}
	return data, true
}
