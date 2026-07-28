package usecase

import (
	"context"
	"fmt"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// SendMessageWorkflow 发送消息用例（协调层）
//
// 编排步骤：
//  1. message.Service.CreateMessage → 幂等检查 + ULID + conv_id + seq + message/outbox 同事务落库
//  2. 幂等命中 → 幂等修复固定数量的会话投影后返回首次结果
//  3. conversation.Service.UpsertForMessage → 更新发送方会话 (isSender=true)
//  4. P2P → Upsert 接收方会话 (isSender=false) / GROUP → UpsertGroupConv
//  5. 返回 {msg_id, seq, conv_id, send_time}；CDC 后续把 outbox 投递到 Kafka msg.push
//
// 群成员 conversation 行不属于发送编排：group.cache 独立消费组会在建群/加人/退群时
// 增量投影成员状态。禁止在每条群消息上重新拉取全体成员并做 INSERT IGNORE。
type SendMessageWorkflow struct {
	msgService        *msgsvc.Service
	convService       *convsvc.Service
	permissionChecker PermissionChecker
}

// PermissionChecker 校验发送者是否有权限向目标会话发送消息。
// fromUuid 是鉴权主体，由 handler 从 gRPC metadata 解析后传入。
type PermissionChecker interface {
	CheckCanSend(ctx context.Context, fromUuid string, req *pb.SendMessageRequest) error
}

// NewSendMessageWorkflow 创建发送消息用例
func NewSendMessageWorkflow(
	msgService *msgsvc.Service,
	convService *convsvc.Service,
	permissionChecker PermissionChecker,
) *SendMessageWorkflow {
	return &SendMessageWorkflow{
		msgService:        msgService,
		convService:       convService,
		permissionChecker: permissionChecker,
	}
}

// Execute 执行发送消息的完整流程。
// fromUuid / deviceId 是鉴权主体，由 handler 从 gRPC metadata（ctxmeta）解析后显式传入。
func (w *SendMessageWorkflow) Execute(ctx context.Context, fromUuid, deviceId string, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	// ============================================================
	// Step 0: 跨服务权限校验（好友/黑名单/群成员）
	// ============================================================
	if w.permissionChecker != nil {
		if err := w.permissionChecker.CheckCanSend(ctx, fromUuid, req); err != nil {
			return nil, fmt.Errorf("SendMessageWorkflow: 发送权限校验失败: %w", err)
		}
	}

	// ============================================================
	// Step 1: 消息领域 → 幂等检查 + ULID + conv_id + seq + message/outbox 同事务落库
	// ============================================================
	result, err := w.msgService.CreateMessage(ctx, fromUuid, deviceId, req)
	if err != nil {
		return nil, fmt.Errorf("SendMessageWorkflow: 创建消息失败: %w", err)
	}

	msg := result.Msg

	// Step 2: 幂等命中 → 修复首次请求可能漏写的固定数量会话投影，再返回原结果。
	// GROUP 只修复发送者个人行与一条共享群行，不恢复逐成员写扩散。
	if result.IsIdempotent {
		if err := w.repairIdempotentConversationProjection(ctx, fromUuid, req, msg); err != nil {
			return nil, fmt.Errorf("SendMessageWorkflow: 修复幂等消息会话投影失败: %w", err)
		}
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
	if err := w.convService.UpsertForMessage(ctx, fromUuid, msg, req.ConvType, req.TargetUuid, true); err != nil {
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
		if err := w.convService.UpsertForMessage(ctx, req.TargetUuid, msg, req.ConvType, fromUuid, false); err != nil {
			logger.Warn(ctx, "发送消息：更新接收方会话失败（不阻断）",
				logger.String("conv_id", msg.ConvId),
				logger.String("receiver", req.TargetUuid),
				logger.ErrorField("error", err),
			)
		}
	} else if req.ConvType == pb.ConvType_CONV_TYPE_GROUP {
		// 群聊读扩散：每条消息只更新一行共享热数据。
		// 成员增减由 group.cache projector 按成员事件处理，发送耗时不再随群人数增长。
		if err := w.convService.UpsertGroupConv(ctx, msg); err != nil {
			logger.Warn(ctx, "发送消息：更新群会话热数据失败（不阻断）",
				logger.String("conv_id", msg.ConvId),
				logger.ErrorField("error", err),
			)
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

func (w *SendMessageWorkflow) repairIdempotentConversationProjection(ctx context.Context, fromUuid string, req *pb.SendMessageRequest, msg *model.Message) error {
	switch req.ConvType {
	case pb.ConvType_CONV_TYPE_P2P:
		if err := w.convService.RepairForMessage(ctx, fromUuid, msg, req.ConvType, req.TargetUuid, true); err != nil {
			return fmt.Errorf("修复发送方会话失败: %w", err)
		}
		if err := w.convService.RepairForMessage(ctx, req.TargetUuid, msg, req.ConvType, fromUuid, false); err != nil {
			return fmt.Errorf("修复接收方会话失败: %w", err)
		}
		return nil
	case pb.ConvType_CONV_TYPE_GROUP:
		if err := w.convService.RepairForMessage(ctx, fromUuid, msg, req.ConvType, req.TargetUuid, true); err != nil {
			return fmt.Errorf("修复群发送方个人会话失败: %w", err)
		}
		if err := w.convService.UpsertGroupConv(ctx, msg); err != nil {
			return fmt.Errorf("修复群共享会话失败: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("不支持的会话类型: %s", req.ConvType.String())
	}
}
