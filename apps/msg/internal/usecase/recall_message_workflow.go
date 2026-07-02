package usecase

import (
	"context"
	"fmt"

	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
)

// RecallMessageWorkflow 撤回消息用例（协调层）
//
// 编排步骤：
//  1. message.Service.RecallMessage → 查消息 + 校验权限 + 校验时间窗口 + message/outbox 同事务提交。
//  2. Debezium CDC 后续把 outbox_events 路由到 Kafka msg.push。
type RecallMessageWorkflow struct {
	msgService *msgsvc.Service
}

// NewRecallMessageWorkflow 创建撤回消息用例
func NewRecallMessageWorkflow(msgService *msgsvc.Service) *RecallMessageWorkflow {
	return &RecallMessageWorkflow{
		msgService: msgService,
	}
}

// Execute 执行撤回消息的完整流程。
// operatorUuid 是鉴权主体，由 handler 从 gRPC metadata（ctxmeta）解析后显式传入。
func (w *RecallMessageWorkflow) Execute(ctx context.Context, operatorUuid string, req *pb.RecallMessageRequest) (*pb.RecallMessageResponse, error) {
	if _, err := w.msgService.RecallMessage(ctx, req.ConvId, req.MsgId, operatorUuid); err != nil {
		return nil, fmt.Errorf("RecallMessageWorkflow: 撤回失败: %w", err)
	}

	return &pb.RecallMessageResponse{}, nil
}
