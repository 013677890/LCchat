package handler

import (
	"context"
	"errors"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usecase"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
)

// MsgHandler 消息服务 gRPC Handler（薄层）
//
// 职责：接收 gRPC 请求，委托给 domain service 或 usecase workflow。
// 日志策略：入口/出口/耗时由拦截器统一记录，handler 仅记录真正的业务处理点错误。
type MsgHandler struct {
	pb.UnimplementedMsgServiceServer
	msgService            *msgsvc.Service
	convService           *convsvc.Service
	sendMessageWorkflow   *usecase.SendMessageWorkflow
	recallMessageWorkflow *usecase.RecallMessageWorkflow
	markReadWorkflow      *usecase.MarkReadWorkflow
}

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

// SendMessage 发送消息。(单聊/群聊统一入口)
func (h *MsgHandler) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	// 1. 调用发送消息工作流执行发送消息
	resp, err := h.sendMessageWorkflow.Execute(ctx, req)
	// 2. 检查发送消息工作流执行结果是否为空
	if err != nil {
		// 3. 检查是否为幂等处理错误
		if errors.Is(err, msgsvc.ErrIdempotentProcessing) {
			return nil, apperr.New(consts.CodeMessageDuplicate)
		}
		return nil, apperr.Wrap(err, consts.CodeMessageSendFail, consts.GetMessage(consts.CodeMessageSendFail))
	}
	return resp, nil
}

// RecallMessage 撤回消息。
func (h *MsgHandler) RecallMessage(ctx context.Context, req *pb.RecallMessageRequest) (*pb.RecallMessageResponse, error) {
	resp, err := h.recallMessageWorkflow.Execute(ctx, req)
	if err != nil {
		return nil, mapMsgDomainError(ctx, err)
	}
	return resp, nil
}

// MarkRead 标记会话已读。
func (h *MsgHandler) MarkRead(ctx context.Context, req *pb.MarkReadRequest) (*pb.MarkReadResponse, error) {
	resp, err := h.markReadWorkflow.Execute(ctx, req)
	if err != nil {
		return nil, mapConvDomainError(ctx, err)
	}
	return resp, nil
}

// PullMessages 按会话增量拉取历史消息。
func (h *MsgHandler) PullMessages(ctx context.Context, req *pb.PullMessagesRequest) (*pb.PullMessagesResponse, error) {
	// 1. 设置清除序列号
	clearSeq := int64(0)
	// 2. 设置拉取方向
	direction := msgsvc.DirectionForward
	if req.Direction == pb.PullDirection_PULL_DIRECTION_BACKWARD {
		direction = msgsvc.DirectionBackward
	}
	// 3. 设置拉取限制
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	msgs, hasMore, err := h.msgService.PullMessages(ctx, req.ConvId, req.AnchorSeq, direction, limit, clearSeq)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	var maxSeq int64
	if len(msgs) > 0 {
		maxSeq = msgs[len(msgs)-1].Seq
	}
	return &pb.PullMessagesResponse{Messages: msgs, HasMore: hasMore, MaxSeq: maxSeq}, nil
}

// GetMessagesByIds 批量按 ID 查询消息。
func (h *MsgHandler) GetMessagesByIds(ctx context.Context, req *pb.GetMessagesByIdsRequest) (*pb.GetMessagesByIdsResponse, error) {
	msgs, err := h.msgService.GetMessagesByIds(ctx, req.ConvId, req.MsgIds)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.GetMessagesByIdsResponse{Messages: msgs}, nil
}

// GetConversations 获取用户会话列表。
func (h *MsgHandler) GetConversations(ctx context.Context, req *pb.GetConversationsRequest) (*pb.GetConversationsResponse, error) {
	items, hasMore, nextCursor, err := h.convService.GetConversations(ctx, req.OwnerUuid, req.UpdatedSince, req.Cursor, int(req.PageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.GetConversationsResponse{Conversations: items, HasMore: hasMore, NextCursor: nextCursor}, nil
}

// DeleteConversation 逻辑删除会话。
func (h *MsgHandler) DeleteConversation(ctx context.Context, req *pb.DeleteConversationRequest) (*pb.DeleteConversationResponse, error) {
	err := h.convService.DeleteConversation(ctx, req.OwnerUuid, req.ConvId)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			// 幂等：会话不存在视为删除成功
			return &pb.DeleteConversationResponse{}, nil
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.DeleteConversationResponse{}, nil
}

// UpdateConversationSettings 更新会话设置。
func (h *MsgHandler) UpdateConversationSettings(ctx context.Context, req *pb.UpdateConvSettingsRequest) (*pb.UpdateConvSettingsResponse, error) {
	err := h.convService.UpdateSettings(ctx, req.OwnerUuid, req.ConvId, req.Mute, req.Pin)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			return nil, apperr.New(consts.CodeConversationNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.UpdateConvSettingsResponse{}, nil
}

func mapMsgDomainError(_ context.Context, err error) error {
	switch {
	case errors.Is(err, msgsvc.ErrMessageNotFound):
		return apperr.New(consts.CodeMessageNotFound)
	case errors.Is(err, msgsvc.ErrMessageAlreadyRecalled):
		return apperr.New(consts.CodeMessageRevoked)
	case errors.Is(err, msgsvc.ErrRecallTimeout):
		return apperr.New(consts.CodeRecallTimeout)
	case errors.Is(err, msgsvc.ErrRecallNoPermission):
		return apperr.New(consts.CodeRecallNoPermission)
	case errors.Is(err, msgsvc.ErrIdempotentProcessing):
		return apperr.New(consts.CodeMessageDuplicate)
	default:
		return apperr.Wrap(err, consts.CodeInternalError, "消息处理失败")
	}
}

func mapConvDomainError(_ context.Context, err error) error {
	switch {
	case errors.Is(err, convsvc.ErrConversationNotFound):
		return apperr.New(consts.CodeConversationNotFound)
	default:
		return apperr.Wrap(err, consts.CodeInternalError, "会话处理失败")
	}
}
