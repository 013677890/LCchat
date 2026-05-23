package usecase

import (
	"context"
	"fmt"
	"time"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/groupcli"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// SendMessageWorkflow 发送消息用例（协调层）
//
// 编排步骤：
//  1. message.Service.CreateMessage → 幂等检查 + ULID + conv_id + seq + message/outbox 同事务落库
//  2. 幂等命中 → 直接返回首次结果
//  3. conversation.Service.UpsertForMessage → 更新发送方会话 (isSender=true)
//  4. P2P → Upsert 接收方会话 (isSender=false) / GROUP → UpsertGroupConv
//  5. 返回 {msg_id, seq, conv_id, send_time}；CDC 后续把 outbox 投递到 Kafka msg.push
type SendMessageWorkflow struct {
	msgService        *msgsvc.Service
	convService       *convsvc.Service
	groupCli          *groupcli.Client
	permissionChecker PermissionChecker
}

// PermissionChecker 校验发送者是否有权限向目标会话发送消息。
type PermissionChecker interface {
	CheckCanSend(ctx context.Context, req *pb.SendMessageRequest) error
}

// NewSendMessageWorkflow 创建发送消息用例
func NewSendMessageWorkflow(
	msgService *msgsvc.Service,
	convService *convsvc.Service,
	groupCli *groupcli.Client,
	permissionChecker PermissionChecker,
) *SendMessageWorkflow {
	return &SendMessageWorkflow{
		msgService:        msgService,
		convService:       convService,
		groupCli:          groupCli,
		permissionChecker: permissionChecker,
	}
}

// Execute 执行发送消息的完整流程
func (w *SendMessageWorkflow) Execute(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	// ============================================================
	// Step 0: 跨服务权限校验（好友/黑名单/群成员）
	// ============================================================
	if w.permissionChecker != nil {
		if err := w.permissionChecker.CheckCanSend(ctx, req); err != nil {
			return nil, fmt.Errorf("SendMessageWorkflow: 发送权限校验失败: %w", err)
		}
	}

	// ============================================================
	// Step 1: 消息领域 → 幂等检查 + ULID + conv_id + seq + message/outbox 同事务落库
	// ============================================================
	result, err := w.msgService.CreateMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("SendMessageWorkflow: 创建消息失败: %w", err)
	}

	msg := result.Msg

	// Step 2: 幂等命中 → 直接返回首次创建的结果
	if result.IsIdempotent {
		return &pb.SendMessageResponse{
			MsgId:    msg.MsgId,
			Seq:      msg.Seq,
			ConvId:   msg.ConvId,
			SendTime: msg.SendTime.UnixMilli(),
		}, nil
	}

	// ============================================================
	// Step 3: 会话领域 → Upsert 发送方会话
	// ============================================================
	if err := w.convService.UpsertForMessage(ctx, req.FromUuid, msg, req.ConvType, req.TargetUuid, true); err != nil {
		logger.Warn(ctx, "发送消息：更新发送方会话失败（不阻断）",
			logger.String("conv_id", msg.ConvId),
			logger.ErrorField("error", err),
		)
	}

	// ============================================================
	// Step 4: 会话领域 → 更新接收方 / 群热数据
	// ============================================================
	if req.ConvType == pb.ConvType_CONV_TYPE_P2P {
		// 单聊写扩散：为接收方 upsert 会话 (isSender=false → unread + 1)
		if err := w.convService.UpsertForMessage(ctx, req.TargetUuid, msg, req.ConvType, req.FromUuid, false); err != nil {
			logger.Warn(ctx, "发送消息：更新接收方会话失败（不阻断）",
				logger.String("conv_id", msg.ConvId),
				logger.String("receiver", req.TargetUuid),
				logger.ErrorField("error", err),
			)
		}
	} else if req.ConvType == pb.ConvType_CONV_TYPE_GROUP {
		// 群聊读扩散：只更新群热数据表
		if err := w.convService.UpsertGroupConv(ctx, msg); err != nil {
			logger.Warn(ctx, "发送消息：更新群会话热数据失败（不阻断）",
				logger.String("conv_id", msg.ConvId),
				logger.ErrorField("error", err),
			)
		}

		// 同步兜底确保群成员在 conversation 表有行。失败不阻断发送，但会再交给异步补偿重试。
		if w.groupCli != nil {
			groupUUID := msg.ConvId
			if err := w.ensureGroupMembersConv(ctx, groupUUID); err != nil {
				logger.Warn(ctx, "发送消息：初始化群成员会话行失败，将异步补偿",
					logger.String("group_uuid", groupUUID),
					logger.ErrorField("error", err),
				)
				w.retryEnsureGroupMembersConv(ctx, groupUUID)
			}
		}
	}

	// ============================================================
	// Step 5: 返回结果
	// ============================================================
	return &pb.SendMessageResponse{
		MsgId:    msg.MsgId,
		Seq:      msg.Seq,
		ConvId:   msg.ConvId,
		SendTime: msg.SendTime.UnixMilli(),
	}, nil
}

func (w *SendMessageWorkflow) ensureGroupMembersConv(ctx context.Context, groupUUID string) error {
	members, err := w.groupCli.GetGroupMemberUUIDs(ctx, groupUUID)
	if err != nil {
		return fmt.Errorf("获取群成员失败: %w", err)
	}
	if err := w.convService.EnsureGroupMembersConv(ctx, members, groupUUID); err != nil {
		return fmt.Errorf("批量初始化群成员会话行失败: %w", err)
	}
	return nil
}

func (w *SendMessageWorkflow) retryEnsureGroupMembersConv(ctx context.Context, groupUUID string) {
	async.RunSafe(ctx, func(taskCtx context.Context) {
		backoffs := []time.Duration{200 * time.Millisecond, time.Second, 3 * time.Second}
		var lastErr error
		for attempt, backoff := range backoffs {
			select {
			case <-taskCtx.Done():
				return
			case <-time.After(backoff):
			}

			if err := w.ensureGroupMembersConv(taskCtx, groupUUID); err != nil {
				lastErr = err
				logger.Warn(taskCtx, "发送消息：群成员会话行补偿失败，将继续重试",
					logger.String("group_uuid", groupUUID),
					logger.Int("attempt", attempt+1),
					logger.ErrorField("error", err),
				)
				continue
			}
			return
		}
		logger.Warn(taskCtx, "发送消息：群成员会话行补偿重试耗尽",
			logger.String("group_uuid", groupUUID),
			logger.ErrorField("error", lastErr),
		)
	}, 10*time.Second)
}
