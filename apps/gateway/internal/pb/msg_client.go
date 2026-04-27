package pb

import (
	"context"

	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
)

// msgServiceClientImpl 消息服务 gRPC 客户端实现
type msgServiceClientImpl struct {
	msgClient msgpb.MsgServiceClient
	breaker   *gobreaker.CircuitBreaker
}

// NewMsgServiceClient 创建消息服务 gRPC 客户端实例
// conn: 消息服务 gRPC 连接
// breaker: 熔断器实例
func NewMsgServiceClient(conn *grpc.ClientConn, breaker *gobreaker.CircuitBreaker) MsgServiceClient {
	return &msgServiceClientImpl{
		msgClient: msgpb.NewMsgServiceClient(conn),
		breaker:   breaker,
	}
}

// ==================== 消息发送 ====================

// SendMessage 发送消息
func (c *msgServiceClientImpl) SendMessage(ctx context.Context, req *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "SendMessage", func() (*msgpb.SendMessageResponse, error) {
		return c.msgClient.SendMessage(ctx, req)
	})
}

// ==================== 消息拉取 ====================

// PullMessages 按会话增量拉取历史消息
func (c *msgServiceClientImpl) PullMessages(ctx context.Context, req *msgpb.PullMessagesRequest) (*msgpb.PullMessagesResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "PullMessages", func() (*msgpb.PullMessagesResponse, error) {
		return c.msgClient.PullMessages(ctx, req)
	})
}

// GetMessagesByIds 批量获取指定消息
func (c *msgServiceClientImpl) GetMessagesByIds(ctx context.Context, req *msgpb.GetMessagesByIdsRequest) (*msgpb.GetMessagesByIdsResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "GetMessagesByIds", func() (*msgpb.GetMessagesByIdsResponse, error) {
		return c.msgClient.GetMessagesByIds(ctx, req)
	})
}

// ==================== 消息操作 ====================

// RecallMessage 撤回消息
func (c *msgServiceClientImpl) RecallMessage(ctx context.Context, req *msgpb.RecallMessageRequest) (*msgpb.RecallMessageResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "RecallMessage", func() (*msgpb.RecallMessageResponse, error) {
		return c.msgClient.RecallMessage(ctx, req)
	})
}

// ==================== 会话管理 ====================

// GetConversations 获取会话列表
func (c *msgServiceClientImpl) GetConversations(ctx context.Context, req *msgpb.GetConversationsRequest) (*msgpb.GetConversationsResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "GetConversations", func() (*msgpb.GetConversationsResponse, error) {
		return c.msgClient.GetConversations(ctx, req)
	})
}

// MarkRead 标记会话已读
func (c *msgServiceClientImpl) MarkRead(ctx context.Context, req *msgpb.MarkReadRequest) (*msgpb.MarkReadResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "MarkRead", func() (*msgpb.MarkReadResponse, error) {
		return c.msgClient.MarkRead(ctx, req)
	})
}

// DeleteConversation 删除会话
func (c *msgServiceClientImpl) DeleteConversation(ctx context.Context, req *msgpb.DeleteConversationRequest) (*msgpb.DeleteConversationResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "DeleteConversation", func() (*msgpb.DeleteConversationResponse, error) {
		return c.msgClient.DeleteConversation(ctx, req)
	})
}

// UpdateConversationSettings 更新会话设置
func (c *msgServiceClientImpl) UpdateConversationSettings(ctx context.Context, req *msgpb.UpdateConvSettingsRequest) (*msgpb.UpdateConvSettingsResponse, error) {
	return ExecuteWithBreaker(c.breaker, "msg.MsgService", "UpdateConversationSettings", func() (*msgpb.UpdateConvSettingsResponse, error) {
		return c.msgClient.UpdateConversationSettings(ctx, req)
	})
}

// ==================== gRPC 连接工具函数 ====================

// msgRetryPolicy 消息服务 gRPC 重试策略
const msgRetryPolicy = `{
	"methodConfig": [{
		"name": [{"service": "msg.MsgService"}],
		"waitForReady": true,
		"timeout": "3s",
		"retryPolicy": {
			"maxAttempts": 3,
			"initialBackoff": "0.1s",
			"maxBackoff": "1s",
			"backoffMultiplier": 2,
			"retryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED"]
		}
	}]
}`

// CreateMsgServiceConnection 创建消息服务 gRPC 连接
// addr: 消息服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateMsgServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, msgRetryPolicy, breaker)
}
