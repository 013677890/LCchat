package usecase

import (
	"context"
	"fmt"
	"time"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/apps/msg/mq"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// MarkReadWorkflow 标记已读用例（协调层）
//
// 编排步骤：
//  1. conversation.Service.MarkRead → read_seq = GREATEST(read_seq, req.read_seq)
//  2. mq.Producer → 写 Kafka MsgPushEvent{type="MSG_MARK_READ", data=MarkReadNotice}
type MarkReadWorkflow struct {
	convService *convsvc.Service
	producer    *mq.Producer
}

// NewMarkReadWorkflow 创建标记已读用例
func NewMarkReadWorkflow(
	convService *convsvc.Service,
	producer *mq.Producer,
) *MarkReadWorkflow {
	return &MarkReadWorkflow{
		convService: convService,
		producer:    producer,
	}
}

// Execute 执行标记已读的完整流程
func (w *MarkReadWorkflow) Execute(ctx context.Context, req *pb.MarkReadRequest) (*pb.MarkReadResponse, error) {

	// ============================================================
	// Step 1: 会话领域 → 更新 read_seq（单调递增），拿回最新未读数
	// ============================================================
	unreadCount, err := w.convService.MarkRead(ctx, req.OwnerUuid, req.ConvId, req.ReadSeq)
	if err != nil {
		return nil, fmt.Errorf("MarkReadWorkflow: 标记已读失败: %w", err)
	}

	// ============================================================
	// Step 2: Kafka → MsgPushEvent{type="MSG_MARK_READ", data=MarkReadNotice}
	// ============================================================
	// 推送目标：该用户的其他在线设备（多端同步清红点）
	notice := &pb.MarkReadNotice{
		ConvId:  req.ConvId,
		ReadSeq: req.ReadSeq,
	}

	// 序列化失败时有限重试（最多 3 次），重试均失败则跳过投递（MSG-003）。
	// DB 写入已完成，其他设备下次打开时会重新拉取最新 read_seq 补偿。
	const maxMarshalRetry = 3
	var noticeData []byte
	var marshalErr error
	for i := 0; i < maxMarshalRetry; i++ {
		noticeData, marshalErr = proto.Marshal(notice)
		if marshalErr == nil {
			break
		}
		logger.Warn(ctx, "标记已读：MarkReadNotice 序列化失败，重试",
			logger.String("conv_id", req.ConvId),
			logger.Int("attempt", i+1),
			logger.ErrorField("error", marshalErr),
		)
	}

	if marshalErr != nil {
		logger.Warn(ctx, "标记已读：MarkReadNotice 序列化重试耗尽，跳过 Kafka 投递，多端红点将在下次同步时修正",
			logger.String("conv_id", req.ConvId),
			logger.ErrorField("error", marshalErr),
		)
	} else {
		serverTs := time.Now().UnixMilli()
		isP2P := len(req.ConvId) > 4 && req.ConvId[:4] == "p2p-"
		convType := pb.ConvType_CONV_TYPE_GROUP
		if isP2P {
			convType = pb.ConvType_CONV_TYPE_P2P
		}

		// 事件 1：推给自己的其他设备（多端同步清红点）
		selfSyncEvent := &pb.MsgPushEvent{
			ReceiverUuid: req.OwnerUuid,
			DeviceId:     ctxmeta.DeviceID(ctx),
			Type:         "MSG_MARK_READ",
			ConvType:     convType,
			Data:         noticeData,
			TraceId:      ctxmeta.TraceID(ctx),
			FromUuid:     req.OwnerUuid,
			ServerTs:     serverTs,
			Seq:          0,
		}

		if err := w.producer.Publish(ctx, req.ConvId, selfSyncEvent); err != nil {
			logger.Warn(ctx, "标记已读：投递 Kafka（self-sync）失败（不阻断）",
				logger.String("conv_id", req.ConvId),
				logger.ErrorField("error", err),
			)
		}

		// 事件 2：P2P 已读回执 → 通知对端显示"已读"
		if isP2P {
			peerUUID := extractPeerUuid(req.ConvId, req.OwnerUuid)
			if peerUUID != "" {
				receiptEvent := &pb.MsgPushEvent{
					ReceiverUuid: peerUUID,
					Type:         "MSG_READ_RECEIPT",
					ConvType:     pb.ConvType_CONV_TYPE_P2P,
					Data:         noticeData,
					TraceId:      ctxmeta.TraceID(ctx),
					FromUuid:     req.OwnerUuid,
					ServerTs:     serverTs,
					Seq:          0,
				}
				if err := w.producer.Publish(ctx, req.ConvId, receiptEvent); err != nil {
					logger.Warn(ctx, "标记已读：投递 Kafka（read-receipt）失败（不阻断）",
						logger.String("conv_id", req.ConvId),
						logger.String("peer_uuid", peerUUID),
						logger.ErrorField("error", err),
					)
				}
			}
		}
	}

	return &pb.MarkReadResponse{
		UnreadCount: unreadCount,
	}, nil
}
