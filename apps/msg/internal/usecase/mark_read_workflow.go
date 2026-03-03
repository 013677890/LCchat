package usecase

import (
	"context"
	"fmt"
	"time"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/apps/msg/mq"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
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

	logger.Info(ctx, "标记已读：DB 已更新",
		logger.String("conv_id", req.ConvId),
		logger.Int64("read_seq", req.ReadSeq),
		logger.Int32("unread_count", unreadCount),
	)

	// ============================================================
	// Step 2: Kafka → MsgPushEvent{type="MSG_MARK_READ", data=MarkReadNotice}
	// ============================================================
	// 推送目标：该用户的其他在线设备（多端同步清红点）
	notice := &pb.MarkReadNotice{
		ConvId:  req.ConvId,
		ReadSeq: req.ReadSeq,
	}
	noticeData, _ := proto.Marshal(notice)

	pushEvent := &pb.MsgPushEvent{
		ReceiverUuid: req.OwnerUuid, // 推给自己的其他设备
		Type:         "MSG_MARK_READ",
		Data:         noticeData,
		FromUuid:     req.OwnerUuid,
		ServerTs:     time.Now().UnixMilli(),
	}

	if err := w.producer.Publish(ctx, req.ConvId, pushEvent); err != nil {
		// DB 已更新，其他设备下次打开时会重新拉取最新 read_seq
		logger.Warn(ctx, "标记已读：投递 Kafka 失败（不阻断）",
			logger.String("conv_id", req.ConvId),
			logger.ErrorField("error", err),
		)
	} else {
		logger.Info(ctx, "标记已读：Kafka 投递成功",
			logger.String("conv_id", req.ConvId),
		)
	}

	return &pb.MarkReadResponse{
		UnreadCount: unreadCount,
	}, nil
}
