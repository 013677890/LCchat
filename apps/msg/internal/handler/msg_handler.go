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
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
)

// MsgHandler 消息服务 gRPC Handler（薄层）
//
// 职责：接收 gRPC 请求，委托给 domain service 或 usecase workflow。
// 日志策略：入口/出口/耗时由拦截器统一记录，handler 仅记录真正的业务处理点错误。
type MsgHandler struct {
	pb.UnimplementedMsgServiceServer
	convService           *convsvc.Service
	messageReadWorkflow   *usecase.MessageReadWorkflow
	sendMessageWorkflow   *usecase.SendMessageWorkflow
	recallMessageWorkflow *usecase.RecallMessageWorkflow
	markReadWorkflow      *usecase.MarkReadWorkflow
}

func NewMsgHandler(
	convService *convsvc.Service,
	messageReadWf *usecase.MessageReadWorkflow,
	sendWf *usecase.SendMessageWorkflow,
	recallWf *usecase.RecallMessageWorkflow,
	markReadWf *usecase.MarkReadWorkflow,
) *MsgHandler {
	return &MsgHandler{
		convService:           convService,
		messageReadWorkflow:   messageReadWf,
		sendMessageWorkflow:   sendWf,
		recallMessageWorkflow: recallWf,
		markReadWorkflow:      markReadWf,
	}
}

// SendMessage 发送消息。(单聊/群聊统一入口)
func (h *MsgHandler) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	userUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}
	// device_id 是幂等三元组的一元，metadata 缺失时无法保证重复请求去重，直接拒绝。
	deviceID := ctxmeta.DeviceID(ctx)
	if deviceID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	resp, err := h.sendMessageWorkflow.Execute(ctx, userUUID, deviceID, req)
	if err != nil {
		return nil, mapMsgDomainError(ctx, err)
	}
	return resp, nil
}

// RecallMessage 撤回消息。
func (h *MsgHandler) RecallMessage(ctx context.Context, req *pb.RecallMessageRequest) (*pb.RecallMessageResponse, error) {
	operatorUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := h.recallMessageWorkflow.Execute(ctx, operatorUUID, req)
	if err != nil {
		return nil, mapMsgDomainError(ctx, err)
	}
	return resp, nil
}

// MarkRead 标记会话已读。
func (h *MsgHandler) MarkRead(ctx context.Context, req *pb.MarkReadRequest) (*pb.MarkReadResponse, error) {
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := h.markReadWorkflow.Execute(ctx, ownerUUID, req)
	if err != nil {
		return nil, mapConvDomainError(ctx, err)
	}
	return resp, nil
}

// PullMessages 按会话增量拉取历史消息。
func (h *MsgHandler) PullMessages(ctx context.Context, req *pb.PullMessagesRequest) (*pb.PullMessagesResponse, error) {
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := h.messageReadWorkflow.PullMessages(ctx, ownerUUID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// BatchSyncMessages 按多个会话各自的 seq 位点批量补拉新消息。
// Handler 只解析一次鉴权主体；跨 conversation/message 领域的权限裁决、读取和
// 有界并发编排统一交给 MessageReadWorkflow。
func (h *MsgHandler) BatchSyncMessages(
	ctx context.Context,
	req *pb.BatchSyncMessagesRequest,
) (*pb.BatchSyncMessagesResponse, error) {
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := h.messageReadWorkflow.BatchSyncMessages(ctx, ownerUUID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetMessagesByIds 批量按 ID 查询消息。
func (h *MsgHandler) GetMessagesByIds(ctx context.Context, req *pb.GetMessagesByIdsRequest) (*pb.GetMessagesByIdsResponse, error) {
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := h.messageReadWorkflow.GetMessagesByIDs(ctx, ownerUUID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetConversations 获取用户会话列表。
func (h *MsgHandler) GetConversations(ctx context.Context, req *pb.GetConversationsRequest) (*pb.GetConversationsResponse, error) {
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	items, hasMore, nextCursor, err := h.convService.GetConversations(ctx, ownerUUID, req.UpdatedSince, req.Cursor, int(req.PageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.GetConversationsResponse{Conversations: items, HasMore: hasMore, NextCursor: nextCursor}, nil
}

// DeleteConversation 逻辑删除会话。
func (h *MsgHandler) DeleteConversation(ctx context.Context, req *pb.DeleteConversationRequest) (*pb.DeleteConversationResponse, error) {
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	err = h.convService.DeleteConversation(ctx, ownerUUID, req.ConvId)
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
	ownerUUID, err := authenticatedUserUUID(ctx)
	if err != nil {
		return nil, err
	}

	err = h.convService.UpdateSettings(ctx, ownerUUID, req.ConvId, req.Mute, req.Pin)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			return nil, apperr.New(consts.CodeConversationNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.UpdateConvSettingsResponse{}, nil
}

// authenticatedUserUUID 从 gRPC metadata（经 ctxmeta 注入）解析鉴权主体。
// 所有 RPC 的操作者身份统一来源于此，请求体不携带身份字段。
func authenticatedUserUUID(ctx context.Context) (string, error) {
	userUUID := ctxmeta.UserUUID(ctx)
	if userUUID == "" {
		return "", apperr.New(consts.CodeUnauthorized)
	}
	return userUUID, nil
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
	case errors.Is(err, msgsvc.ErrUnsupportedMsgType):
		return apperr.New(consts.CodeMessageTypeNotSupport)
	case errors.Is(err, msgsvc.ErrIdempotentProcessing):
		return apperr.New(consts.CodeMessageProcessing)
	default:
		return apperr.Wrap(err, consts.CodeMessageSendFail, "消息处理失败")
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
