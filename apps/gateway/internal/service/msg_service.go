package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
)

// MsgServiceImpl 消息服务实现
//
// 操作者身份（user_uuid/device_id）不再写入请求体：
// 出站 gRPC 由 grpcx MetadataUnaryClientInterceptor 统一从 ctx 注入 metadata，
// msg-service 服务端经 MetadataUnaryInterceptor 落回 ctx 后自行解析鉴权主体。
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
	protoReq := dto.ConvertToProtoSendMessageRequest(req)
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

// BatchSyncMessages 将 HTTP 层的逐会话位点一次性转发给 msg-service。
// 部分失败仍是一个成功的 RPC 响应，具体失败码保留在每个 result.errorCode 中。
func (s *MsgServiceImpl) BatchSyncMessages(ctx context.Context, req *dto.BatchSyncMessagesRequest) (*dto.BatchSyncMessagesResponse, error) {
	protoReq := dto.ConvertToProtoBatchSyncMessagesRequest(req)
	resp, err := s.msgClient.BatchSyncMessages(ctx, protoReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Results) != len(protoReq.Conversations) {
		return nil, apperr.New(consts.CodeInternalError)
	}
	for index, result := range resp.Results {
		requestItem := protoReq.Conversations[index]
		if result == nil || requestItem == nil || result.ConvId != requestItem.ConvId {
			// 严格执行“一项请求对应同下标的一项结果”契约，不猜测、不重排，也不静默
			// 跳过异常项；下游违反契约时整次 Gateway 调用按内部错误失败。
			return nil, apperr.New(consts.CodeInternalError)
		}
	}
	return dto.ConvertBatchSyncMessagesResponseFromProto(resp), nil
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
	protoReq := dto.ConvertToProtoRecallMessageRequest(req)
	_, err := s.msgClient.RecallMessage(ctx, protoReq)
	return err
}

// GetConversations 获取会话列表
func (s *MsgServiceImpl) GetConversations(ctx context.Context, req *dto.GetConversationsRequest) (*dto.GetConversationsResponse, error) {
	protoReq := dto.ConvertToProtoGetConversationsRequest(req)
	resp, err := s.msgClient.GetConversations(ctx, protoReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertGetConversationsResponseFromProto(resp), nil
}

// MarkRead 标记会话已读
func (s *MsgServiceImpl) MarkRead(ctx context.Context, req *dto.MarkReadRequest) (*dto.MarkReadResponse, error) {
	protoReq := dto.ConvertToProtoMarkReadRequest(req)
	resp, err := s.msgClient.MarkRead(ctx, protoReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertMarkReadResponseFromProto(resp), nil
}

// DeleteConversation 删除会话
func (s *MsgServiceImpl) DeleteConversation(ctx context.Context, req *dto.DeleteConversationRequest) error {
	protoReq := dto.ConvertToProtoDeleteConversationRequest(req)
	_, err := s.msgClient.DeleteConversation(ctx, protoReq)
	return err
}

// UpdateConversationSettings 更新会话设置
func (s *MsgServiceImpl) UpdateConversationSettings(ctx context.Context, req *dto.UpdateConvSettingsRequest) error {
	protoReq := dto.ConvertToProtoUpdateConvSettingsRequest(req)
	_, err := s.msgClient.UpdateConversationSettings(ctx, protoReq)
	return err
}
