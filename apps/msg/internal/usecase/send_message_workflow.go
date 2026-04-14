package usecase

import (
	"context"
	"fmt"
	"time"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/mq"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// SendMessageWorkflow 发送消息用例（协调层）
//
// 编排步骤：
//  1. message.Service.CreateMessage → 幂等检查 + ULID + conv_id + seq + 落库
//  2. 幂等命中 → 直接返回首次结果
//  3. conversation.Service.UpsertForMessage → 更新发送方会话 (isSender=true)
//  4. P2P → Upsert 接收方会话 (isSender=false) / GROUP → UpsertGroupConv
//  5. mq.Producer.Publish → 写 Kafka MsgPushEvent (key=conv_id)
//  6. 返回 {msg_id, seq, conv_id, send_time}
type SendMessageWorkflow struct {
	msgService  *msgsvc.Service
	convService *convsvc.Service
	producer    *mq.Producer
}

// NewSendMessageWorkflow 创建发送消息用例
func NewSendMessageWorkflow(
	msgService *msgsvc.Service,
	convService *convsvc.Service,
	producer *mq.Producer,
) *SendMessageWorkflow {
	return &SendMessageWorkflow{
		msgService:  msgService,
		convService: convService,
		producer:    producer,
	}
}

// Execute 执行发送消息的完整流程
func (w *SendMessageWorkflow) Execute(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {

	// ============================================================
	// Step 1: 消息领域 → 幂等检查 + ULID + conv_id + seq + 落库
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
	}

	// ============================================================
	// Step 5: Kafka → 构造 MsgPushEvent 投递
	// ============================================================
	msgItem := msgsvc.ModelToMsgItem(msg)
	msgItemData, marshalErr := proto.Marshal(msgItem)
	if marshalErr != nil {
		// 序列化失败：阻断投递，避免下游收到损坏载荷（MSG-002）。
		// 消息已落库，客户端可通过拉取补偿，不影响数据一致性。
		logger.Warn(ctx, "发送消息：MsgItem 序列化失败，跳过 Kafka 投递",
			logger.String("conv_id", msg.ConvId),
			logger.String("msg_id", msg.MsgId),
			logger.ErrorField("error", marshalErr),
		)
	} else {
		pushEvent := &pb.MsgPushEvent{
			ReceiverUuid: req.TargetUuid, // 单聊=对端 UUID, 群聊=群 UUID
			DeviceId:     req.DeviceId,   // 发送方设备 ID（多端同步时排除）
			Type:         "MSG_PUSH",     // 新消息推送
			ConvType:     req.ConvType,   // Push-Job 据此判断扩散策略
			Data:         msgItemData,    // MsgItem 序列化 bytes
			FromUuid:     req.FromUuid,   // 多端同步用
			ServerTs:     time.Now().UnixMilli(),
		}

		if err := w.producer.Publish(ctx, msg.ConvId, pushEvent); err != nil {
			logger.Warn(ctx, "发送消息：投递 Kafka 失败（不阻断）",
				logger.String("conv_id", msg.ConvId),
				logger.String("msg_id", msg.MsgId),
				logger.ErrorField("error", err),
			)
		}
	}

	// ============================================================
	// Step 6: 返回结果
	// ============================================================
	return &pb.SendMessageResponse{
		MsgId:    msg.MsgId,
		Seq:      msg.Seq,
		ConvId:   msg.ConvId,
		SendTime: msg.SendTime.UnixMilli(),
	}, nil
}
