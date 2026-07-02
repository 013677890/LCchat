package usecase

import (
	"context"
	"fmt"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
)

// MarkReadWorkflow 标记已读用例（协调层）
//
// 编排步骤：
//  1. conversation.Service.MarkRead → conversation/outbox 同事务提交。
//  2. Debezium CDC 后续把 outbox_events 路由到 Kafka msg.push。
type MarkReadWorkflow struct {
	convService *convsvc.Service
}

// NewMarkReadWorkflow 创建标记已读用例
func NewMarkReadWorkflow(convService *convsvc.Service) *MarkReadWorkflow {
	return &MarkReadWorkflow{
		convService: convService,
	}
}

// Execute 执行标记已读的完整流程。
// ownerUuid 是鉴权主体，由 handler 从 gRPC metadata（ctxmeta）解析后显式传入。
func (w *MarkReadWorkflow) Execute(ctx context.Context, ownerUuid string, req *pb.MarkReadRequest) (*pb.MarkReadResponse, error) {
	unreadCount, err := w.convService.MarkRead(ctx, ownerUuid, req.ConvId, req.ReadSeq)
	if err != nil {
		return nil, fmt.Errorf("MarkReadWorkflow: 标记已读失败: %w", err)
	}

	return &pb.MarkReadResponse{
		UnreadCount: unreadCount,
	}, nil
}
