package handler

import (
	"context"
	"errors"
	"strconv"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usecase"
)

// MsgHandler 消息服务 gRPC Handler（薄层）
//
// 职责：接收 gRPC 请求 → 委托给 domain service 或 usecase workflow
//
// 日志策略：
//   - 入口/出口/耗时/错误码 → 由 grpcx.LoggingUnaryInterceptor 统一记录
//   - handler 层仅记录业务语义日志（幂等命中、降级处理等特殊场景）
//
// 路由规则：
//   - 跨领域操作 → usecase workflow（SendMessage, RecallMessage, MarkRead）
//   - 单领域操作 → 直接调用 domain service
type MsgHandler struct {
	pb.UnimplementedMsgServiceServer

	// domain services
	msgService  *msgsvc.Service
	convService *convsvc.Service

	// usecase workflows
	sendMessageWorkflow   *usecase.SendMessageWorkflow
	recallMessageWorkflow *usecase.RecallMessageWorkflow
	markReadWorkflow      *usecase.MarkReadWorkflow
}

// NewMsgHandler 创建 MsgHandler
func NewMsgHandler(
	msgService *msgsvc.Service,
	convService *convsvc.Service,
	sendWf *usecase.SendMessageWorkflow,
	recallWf *usecase.RecallMessageWorkflow,
	markReadWf *usecase.MarkReadWorkflow,
) *MsgHandler {
	return &MsgHandler{
		msgService:            msgService,
		convService:           convService,
		sendMessageWorkflow:   sendWf,
		recallMessageWorkflow: recallWf,
		markReadWorkflow:      markReadWf,
	}
}

// ==================== 跨领域操作（走 usecase workflow） ====================

// SendMessage 发送消息（单聊/群聊统一入口）
func (h *MsgHandler) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	resp, err := h.sendMessageWorkflow.Execute(ctx, req)
	if err != nil {
		if errors.Is(err, msgsvc.ErrIdempotentProcessing) {
			return nil, status.Error(codes.Aborted, strconv.Itoa(consts.CodeMessageDuplicate))
		}
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeMessageSendFail))
	}
	return resp, nil
}

// RecallMessage 撤回消息
func (h *MsgHandler) RecallMessage(ctx context.Context, req *pb.RecallMessageRequest) (*pb.RecallMessageResponse, error) {
	resp, err := h.recallMessageWorkflow.Execute(ctx, req)
	if err != nil {
		return nil, mapMsgDomainError(ctx, err)
	}
	return resp, nil
}

// MarkRead 标记会话已读
func (h *MsgHandler) MarkRead(ctx context.Context, req *pb.MarkReadRequest) (*pb.MarkReadResponse, error) {
	resp, err := h.markReadWorkflow.Execute(ctx, req)
	if err != nil {
		return nil, mapConvDomainError(ctx, err)
	}
	return resp, nil
}

// ==================== 单领域操作（直调 domain service） ====================

// PullMessages 按会话增量拉取历史消息
func (h *MsgHandler) PullMessages(ctx context.Context, req *pb.PullMessagesRequest) (*pb.PullMessagesResponse, error) {
	// clear_seq 默认为 0（不过滤）
	// TODO: 待 gateway 集成时通过 context 拿 owner_uuid 查 clear_seq
	clearSeq := int64(0)

	direction := msgsvc.DirectionForward
	if req.Direction == pb.PullDirection_PULL_DIRECTION_BACKWARD {
		direction = msgsvc.DirectionBackward
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	msgs, hasMore, err := h.msgService.PullMessages(ctx, req.ConvId, req.AnchorSeq, direction, limit, clearSeq)
	if err != nil {
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	var maxSeq int64
	if len(msgs) > 0 {
		maxSeq = msgs[len(msgs)-1].Seq
	}

	return &pb.PullMessagesResponse{
		Messages: msgs,
		HasMore:  hasMore,
		MaxSeq:   maxSeq,
	}, nil
}

// GetMessagesByIds 批量按 ID 查询消息
func (h *MsgHandler) GetMessagesByIds(ctx context.Context, req *pb.GetMessagesByIdsRequest) (*pb.GetMessagesByIdsResponse, error) {
	msgs, err := h.msgService.GetMessagesByIds(ctx, req.ConvId, req.MsgIds)
	if err != nil {
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	return &pb.GetMessagesByIdsResponse{Messages: msgs}, nil
}

// GetConversations 获取用户会话列表（全量/增量）
func (h *MsgHandler) GetConversations(ctx context.Context, req *pb.GetConversationsRequest) (*pb.GetConversationsResponse, error) {
	items, hasMore, nextCursor, err := h.convService.GetConversations(
		ctx, req.OwnerUuid, req.UpdatedSince, req.Cursor, int(req.PageSize),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	return &pb.GetConversationsResponse{
		Conversations: items,
		HasMore:       hasMore,
		NextCursor:    nextCursor,
	}, nil
}

// DeleteConversation 逻辑删除会话
func (h *MsgHandler) DeleteConversation(ctx context.Context, req *pb.DeleteConversationRequest) (*pb.DeleteConversationResponse, error) {
	err := h.convService.DeleteConversation(ctx, req.OwnerUuid, req.ConvId)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			// 幂等：会话不存在视为删除成功
			return &pb.DeleteConversationResponse{}, nil
		}
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	return &pb.DeleteConversationResponse{}, nil
}

// UpdateConversationSettings 更新会话设置（免打扰/置顶）
func (h *MsgHandler) UpdateConversationSettings(ctx context.Context, req *pb.UpdateConvSettingsRequest) (*pb.UpdateConvSettingsResponse, error) {
	err := h.convService.UpdateSettings(ctx, req.OwnerUuid, req.ConvId, req.Mute, req.Pin)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeConversationNotFound))
		}
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	return &pb.UpdateConvSettingsResponse{}, nil
}

// ==================== 错误映射辅助函数 ====================

// mapMsgDomainError 将消息领域错误映射为 gRPC status
func mapMsgDomainError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, msgsvc.ErrMessageNotFound):
		return status.Error(codes.NotFound, strconv.Itoa(consts.CodeMessageNotFound))
	case errors.Is(err, msgsvc.ErrMessageAlreadyRecalled):
		return status.Error(codes.AlreadyExists, strconv.Itoa(consts.CodeMessageRevoked))
	case errors.Is(err, msgsvc.ErrRecallTimeout):
		return status.Error(codes.FailedPrecondition, strconv.Itoa(consts.CodeRecallTimeout))
	case errors.Is(err, msgsvc.ErrRecallNoPermission):
		return status.Error(codes.PermissionDenied, strconv.Itoa(consts.CodeRecallNoPermission))
	case errors.Is(err, msgsvc.ErrIdempotentProcessing):
		return status.Error(codes.Aborted, strconv.Itoa(consts.CodeMessageDuplicate))
	default:
		logger.Error(ctx, "消息操作失败", logger.ErrorField("error", err))
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
}

// mapConvDomainError 将会话领域错误映射为 gRPC status
func mapConvDomainError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, convsvc.ErrConversationNotFound):
		return status.Error(codes.NotFound, strconv.Itoa(consts.CodeConversationNotFound))
	default:
		logger.Error(ctx, "会话操作失败", logger.ErrorField("error", err))
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
}
