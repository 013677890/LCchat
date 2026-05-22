package usecase

import (
	"context"
	"fmt"
	"time"

	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/mq"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// RecallMessageWorkflow 撤回消息用例（协调层）
//
// 编排步骤：
//  1. message.Service.RecallMessage → 查消息 + 校验权限 + 校验时间窗口 + 更新 DB
//  2. mq.Producer → 写 Kafka MsgPushEvent{type="MSG_RECALL", data=RecallNotice}
type RecallMessageWorkflow struct {
	msgService *msgsvc.Service
	producer   *mq.Producer
}

const recallKafkaPublishTimeout = 200 * time.Millisecond

// NewRecallMessageWorkflow 创建撤回消息用例
func NewRecallMessageWorkflow(
	msgService *msgsvc.Service,
	producer *mq.Producer,
) *RecallMessageWorkflow {
	return &RecallMessageWorkflow{
		msgService: msgService,
		producer:   producer,
	}
}

// Execute 执行撤回消息的完整流程
func (w *RecallMessageWorkflow) Execute(ctx context.Context, req *pb.RecallMessageRequest) (*pb.RecallMessageResponse, error) {

	// ============================================================
	// Step 1: 消息领域 → 权限 + 时间窗口 + DB status=1
	// ============================================================
	msg, err := w.msgService.RecallMessage(ctx, req.ConvId, req.MsgId, req.OperatorUuid)
	if err != nil {
		return nil, fmt.Errorf("RecallMessageWorkflow: 撤回失败: %w", err)
	}

	// ============================================================
	// Step 2: Kafka → MsgPushEvent{type="MSG_RECALL", data=RecallNotice}
	// ============================================================
	serverTs := time.Now().UnixMilli()
	notice := &pb.RecallNotice{
		ConvId:     req.ConvId,
		MsgId:      req.MsgId,
		Operator:   req.OperatorUuid,
		RecallTime: serverTs,
	}
	noticeData, marshalErr := proto.Marshal(notice)
	if marshalErr != nil {
		logger.Warn(ctx, "撤回消息：RecallNotice 序列化失败，跳过 Kafka 投递",
			logger.String("conv_id", req.ConvId),
			logger.String("msg_id", req.MsgId),
			logger.ErrorField("error", marshalErr),
		)
		return &pb.RecallMessageResponse{}, nil
	}

	// 确定 receiver + conv_type
	convType := pb.ConvType_CONV_TYPE_GROUP
	receiverUuid := msg.ConvId // 群聊：receiver = 群 UUID
	if len(msg.ConvId) > 4 && msg.ConvId[:4] == "p2p-" {
		convType = pb.ConvType_CONV_TYPE_P2P
		receiverUuid = extractPeerUuid(msg.ConvId, req.OperatorUuid)
	}

	pushEvent := &pb.MsgPushEvent{
		ReceiverUuid: receiverUuid,
		DeviceId:     ctxmeta.DeviceID(ctx),
		Type:         "MSG_RECALL",
		ConvType:     convType,
		Data:         noticeData,
		TraceId:      ctxmeta.TraceID(ctx),
		FromUuid:     req.OperatorUuid,
		ServerTs:     serverTs,
		Seq:          0,
	}

	w.publishRecallEventBestEffort(ctx, req, pushEvent)

	return &pb.RecallMessageResponse{}, nil
}

// publishRecallEventBestEffort 尝试投递撤回通知，但不会把 Kafka 波动放大为主链路超时。
// 这里显式脱离父请求的 cancel/deadline，并附加一个更短的独立超时，保证“DB 已成功撤回”后，
// 最多只为通知投递额外等待一个很小的时间窗口；失败则记录告警，客户端后续仍可通过 Pull 自愈。
func (w *RecallMessageWorkflow) publishRecallEventBestEffort(ctx context.Context, req *pb.RecallMessageRequest, pushEvent *pb.MsgPushEvent) {
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recallKafkaPublishTimeout)
	defer cancel()

	if err := w.producer.Publish(publishCtx, req.ConvId, pushEvent); err != nil {
		logger.Warn(ctx, "撤回消息：投递 Kafka 失败（不阻断）",
			logger.String("conv_id", req.ConvId),
			logger.String("msg_id", req.MsgId),
			logger.ErrorField("error", err),
		)
	}
}
