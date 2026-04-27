package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
)

// MsgServiceImpl 消息服务实现
type MsgServiceImpl struct {
	msgClient pb.MsgServiceClient
}

// NewMsgService 创建消息服务实例
func NewMsgService(msgClient pb.MsgServiceClient) MsgService {
	return &MsgServiceImpl{
		msgClient: msgClient,
	}
}

// SendMessage 发送消息
func (s *MsgServiceImpl) SendMessage(ctx context.Context, req *dto.SendMessageRequest) (*dto.SendMessageResponse, error) {
	userUUID := ctxmeta.UserUUID(ctx)
	deviceID := ctxmeta.DeviceID(ctx)

	protoReq := dto.ConvertToProtoSendMessageRequest(req, userUUID, deviceID)

	resp, err := s.msgClient.SendMessage(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertSendMessageResponseFromProto(resp), nil
}

// PullMessages 拉取历史消息
func (s *MsgServiceImpl) PullMessages(ctx context.Context, req *dto.PullMessagesRequest) (*dto.PullMessagesResponse, error) {
	protoReq := dto.ConvertToProtoPullMessagesRequest(req)

	resp, err := s.msgClient.PullMessages(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertPullMessagesResponseFromProto(resp), nil
}

// GetMessagesByIds 批量获取指定消息
func (s *MsgServiceImpl) GetMessagesByIds(ctx context.Context, req *dto.GetMessagesByIdsRequest) (*dto.GetMessagesByIdsResponse, error) {
	protoReq := dto.ConvertToProtoGetMessagesByIdsRequest(req)

	resp, err := s.msgClient.GetMessagesByIds(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertGetMessagesByIdsResponseFromProto(resp), nil
}

// RecallMessage 撤回消息
func (s *MsgServiceImpl) RecallMessage(ctx context.Context, req *dto.RecallMessageRequest) error {
	userUUID := ctxmeta.UserUUID(ctx)

	protoReq := dto.ConvertToProtoRecallMessageRequest(req, userUUID)

	_, err := s.msgClient.RecallMessage(ctx, protoReq)
	return err
}

// GetConversations 获取会话列表
func (s *MsgServiceImpl) GetConversations(ctx context.Context, req *dto.GetConversationsRequest) (*dto.GetConversationsResponse, error) {
	userUUID := ctxmeta.UserUUID(ctx)

	protoReq := dto.ConvertToProtoGetConversationsRequest(req, userUUID)

	resp, err := s.msgClient.GetConversations(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertGetConversationsResponseFromProto(resp), nil
}

// MarkRead 标记会话已读
func (s *MsgServiceImpl) MarkRead(ctx context.Context, req *dto.MarkReadRequest) (*dto.MarkReadResponse, error) {
	userUUID := ctxmeta.UserUUID(ctx)

	protoReq := dto.ConvertToProtoMarkReadRequest(req, userUUID)

	resp, err := s.msgClient.MarkRead(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertMarkReadResponseFromProto(resp), nil
}

// DeleteConversation 删除会话
func (s *MsgServiceImpl) DeleteConversation(ctx context.Context, req *dto.DeleteConversationRequest) error {
	userUUID := ctxmeta.UserUUID(ctx)

	protoReq := dto.ConvertToProtoDeleteConversationRequest(req, userUUID)

	_, err := s.msgClient.DeleteConversation(ctx, protoReq)
	return err
}

// UpdateConversationSettings 更新会话设置
func (s *MsgServiceImpl) UpdateConversationSettings(ctx context.Context, req *dto.UpdateConvSettingsRequest) error {
	userUUID := ctxmeta.UserUUID(ctx)

	protoReq := dto.ConvertToProtoUpdateConvSettingsRequest(req, userUUID)

	_, err := s.msgClient.UpdateConversationSettings(ctx, protoReq)
	return err
}
