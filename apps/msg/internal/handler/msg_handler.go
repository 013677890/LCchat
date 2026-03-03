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
// 路由规则：
//   - 跨领域操作 → usecase workflow（SendMessage, RecallMessage, MarkRead）
//   - 单领域操作 → 直接调用 domain service（PullMessages, GetMessagesByIds, GetConversations, DeleteConv, UpdateSettings）
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
//
// 错误码映射：
//   - 13011 ErrIdempotentProcessing → codes.Aborted（并发同一请求，客户端稍后重试）
//   - 13002 其他发送失败            → codes.Internal
func (h *MsgHandler) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	logger.Info(ctx, "SendMessage",
		logger.String("from_uuid", req.FromUuid),
		logger.String("target_uuid", req.TargetUuid),
		logger.String("client_msg_id", req.ClientMsgId),
	)

	resp, err := h.sendMessageWorkflow.Execute(ctx, req)
	if err != nil {
		if errors.Is(err, msgsvc.ErrIdempotentProcessing) {
			// 并发同一请求正在处理，客户端稍后重试
			return nil, status.Error(codes.Aborted, strconv.Itoa(consts.CodeMessageDuplicate))
		}
		logger.Error(ctx, "SendMessage workflow failed",
			logger.String("from_uuid", req.FromUuid),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeMessageSendFail))
	}

	return resp, nil
}

// RecallMessage 撤回消息
//
// 错误码映射：
//   - 13001 ErrMessageNotFound      → codes.NotFound
//   - 13007 ErrMessageAlreadyRecalled → codes.AlreadyExists（幂等：已撤回视为成功）
//   - 13009 ErrRecallTimeout        → codes.FailedPrecondition
//   - 13010 ErrRecallNoPermission   → codes.PermissionDenied
//   - 30001 其他                     → codes.Internal
func (h *MsgHandler) RecallMessage(ctx context.Context, req *pb.RecallMessageRequest) (*pb.RecallMessageResponse, error) {
	logger.Info(ctx, "RecallMessage",
		logger.String("conv_id", req.ConvId),
		logger.String("msg_id", req.MsgId),
		logger.String("operator_uuid", req.OperatorUuid),
	)

	resp, err := h.recallMessageWorkflow.Execute(ctx, req)
	if err != nil {
		return nil, mapMsgDomainError(ctx, err, "RecallMessage")
	}

	return resp, nil
}

// MarkRead 标记会话已读
//
// 错误码映射：
//   - 13004 ErrConversationNotFound → codes.NotFound
//   - 30001 其他                     → codes.Internal
func (h *MsgHandler) MarkRead(ctx context.Context, req *pb.MarkReadRequest) (*pb.MarkReadResponse, error) {
	logger.Info(ctx, "MarkRead",
		logger.String("owner_uuid", req.OwnerUuid),
		logger.String("conv_id", req.ConvId),
	)

	resp, err := h.markReadWorkflow.Execute(ctx, req)
	if err != nil {
		return nil, mapConvDomainError(ctx, err, "MarkRead")
	}

	return resp, nil
}

// ==================== 单领域操作（直调 domain service） ====================

// PullMessages 按会话增量拉取历史消息
func (h *MsgHandler) PullMessages(ctx context.Context, req *pb.PullMessagesRequest) (*pb.PullMessagesResponse, error) {
	logger.Info(ctx, "PullMessages",
		logger.String("conv_id", req.ConvId),
		logger.Int("direction", int(req.Direction)),
	)

	// 从当前用户会话拉取 clear_seq，过滤删除前的历史消息
	// conv_id + owner_uuid 必须由 gateway 透传。此处 owner_uuid 暂时通过 context 获取。
	// 注意：PullMessagesRequest 中未含 owner_uuid，clear_seq 默认为 0（不过滤）
	// TODO: 待 gateway 集成时通过 context 拿 owner_uuid 查 clear_seq；当前先不过滤
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
		logger.Error(ctx, "PullMessages failed",
			logger.String("conv_id", req.ConvId),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// max_seq: 本次拉到的最大 seq（用于客户端判断 gap；若无数据则返回 0）
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
	logger.Info(ctx, "GetMessagesByIds",
		logger.String("conv_id", req.ConvId),
		logger.Int("count", len(req.MsgIds)),
	)

	msgs, err := h.msgService.GetMessagesByIds(ctx, req.ConvId, req.MsgIds)
	if err != nil {
		logger.Error(ctx, "GetMessagesByIds failed",
			logger.String("conv_id", req.ConvId),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	return &pb.GetMessagesByIdsResponse{
		Messages: msgs,
	}, nil
}

// GetConversations 获取用户会话列表（全量/增量）
func (h *MsgHandler) GetConversations(ctx context.Context, req *pb.GetConversationsRequest) (*pb.GetConversationsResponse, error) {
	logger.Info(ctx, "GetConversations",
		logger.String("owner_uuid", req.OwnerUuid),
		logger.Int("page_size", int(req.PageSize)),
	)

	items, hasMore, nextCursor, err := h.convService.GetConversations(
		ctx,
		req.OwnerUuid,
		req.UpdatedSince,
		req.Cursor,
		int(req.PageSize),
	)
	if err != nil {
		logger.Error(ctx, "GetConversations failed",
			logger.String("owner_uuid", req.OwnerUuid),
			logger.ErrorField("error", err),
		)
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
	logger.Info(ctx, "DeleteConversation",
		logger.String("owner_uuid", req.OwnerUuid),
		logger.String("conv_id", req.ConvId),
	)

	err := h.convService.DeleteConversation(ctx, req.OwnerUuid, req.ConvId)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			// 会话不存在：幂等成功（已删除视为成功）
			return &pb.DeleteConversationResponse{}, nil
		}
		logger.Error(ctx, "DeleteConversation failed",
			logger.String("owner_uuid", req.OwnerUuid),
			logger.String("conv_id", req.ConvId),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	return &pb.DeleteConversationResponse{}, nil
}

// UpdateConversationSettings 更新会话设置（免打扰/置顶）
func (h *MsgHandler) UpdateConversationSettings(ctx context.Context, req *pb.UpdateConvSettingsRequest) (*pb.UpdateConvSettingsResponse, error) {
	logger.Info(ctx, "UpdateConversationSettings",
		logger.String("owner_uuid", req.OwnerUuid),
		logger.String("conv_id", req.ConvId),
	)

	err := h.convService.UpdateSettings(ctx, req.OwnerUuid, req.ConvId, req.Mute, req.Pin)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeConversationNotFound))
		}
		logger.Error(ctx, "UpdateConversationSettings failed",
			logger.String("owner_uuid", req.OwnerUuid),
			logger.String("conv_id", req.ConvId),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	return &pb.UpdateConvSettingsResponse{}, nil
}

// ==================== 错误映射辅助函数 ====================

// mapMsgDomainError 将消息领域错误映射为 gRPC status
func mapMsgDomainError(ctx context.Context, err error, op string) error {
	switch {
	case errors.Is(err, msgsvc.ErrMessageNotFound):
		return status.Error(codes.NotFound, strconv.Itoa(consts.CodeMessageNotFound))

	case errors.Is(err, msgsvc.ErrMessageAlreadyRecalled):
		// 已撤回：幂等，直接返回成功（上层调用者忽略此错误即可）
		// 此处仍返回 AlreadyExists 让调用方感知，不强制 ok
		return status.Error(codes.AlreadyExists, strconv.Itoa(consts.CodeMessageRevoked))

	case errors.Is(err, msgsvc.ErrRecallTimeout):
		return status.Error(codes.FailedPrecondition, strconv.Itoa(consts.CodeRecallTimeout))

	case errors.Is(err, msgsvc.ErrRecallNoPermission):
		return status.Error(codes.PermissionDenied, strconv.Itoa(consts.CodeRecallNoPermission))

	case errors.Is(err, msgsvc.ErrIdempotentProcessing):
		return status.Error(codes.Aborted, strconv.Itoa(consts.CodeMessageDuplicate))

	default:
		logger.Error(ctx, op+" failed",
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
}

// mapConvDomainError 将会话领域错误映射为 gRPC status
func mapConvDomainError(ctx context.Context, err error, op string) error {
	switch {
	case errors.Is(err, convsvc.ErrConversationNotFound):
		return status.Error(codes.NotFound, strconv.Itoa(consts.CodeConversationNotFound))

	default:
		logger.Error(ctx, op+" failed",
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
}
