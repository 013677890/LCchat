package service

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGatewayMsgClient struct {
	gatewaypb.MsgServiceClient

	sendMessageFn                func(context.Context, *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error)
	pullMessagesFn               func(context.Context, *msgpb.PullMessagesRequest) (*msgpb.PullMessagesResponse, error)
	getMessagesByIdsFn           func(context.Context, *msgpb.GetMessagesByIdsRequest) (*msgpb.GetMessagesByIdsResponse, error)
	recallMessageFn              func(context.Context, *msgpb.RecallMessageRequest) (*msgpb.RecallMessageResponse, error)
	getConversationsFn           func(context.Context, *msgpb.GetConversationsRequest) (*msgpb.GetConversationsResponse, error)
	markReadFn                   func(context.Context, *msgpb.MarkReadRequest) (*msgpb.MarkReadResponse, error)
	deleteConversationFn         func(context.Context, *msgpb.DeleteConversationRequest) (*msgpb.DeleteConversationResponse, error)
	updateConversationSettingsFn func(context.Context, *msgpb.UpdateConvSettingsRequest) (*msgpb.UpdateConvSettingsResponse, error)
}

func (f *fakeGatewayMsgClient) SendMessage(ctx context.Context, req *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error) {
	if f.sendMessageFn == nil {
		return nil, errors.New("unexpected SendMessage call")
	}
	return f.sendMessageFn(ctx, req)
}

func (f *fakeGatewayMsgClient) PullMessages(ctx context.Context, req *msgpb.PullMessagesRequest) (*msgpb.PullMessagesResponse, error) {
	if f.pullMessagesFn == nil {
		return nil, errors.New("unexpected PullMessages call")
	}
	return f.pullMessagesFn(ctx, req)
}

func (f *fakeGatewayMsgClient) GetMessagesByIds(ctx context.Context, req *msgpb.GetMessagesByIdsRequest) (*msgpb.GetMessagesByIdsResponse, error) {
	if f.getMessagesByIdsFn == nil {
		return nil, errors.New("unexpected GetMessagesByIds call")
	}
	return f.getMessagesByIdsFn(ctx, req)
}

func (f *fakeGatewayMsgClient) RecallMessage(ctx context.Context, req *msgpb.RecallMessageRequest) (*msgpb.RecallMessageResponse, error) {
	if f.recallMessageFn == nil {
		return nil, errors.New("unexpected RecallMessage call")
	}
	return f.recallMessageFn(ctx, req)
}

func (f *fakeGatewayMsgClient) GetConversations(ctx context.Context, req *msgpb.GetConversationsRequest) (*msgpb.GetConversationsResponse, error) {
	if f.getConversationsFn == nil {
		return nil, errors.New("unexpected GetConversations call")
	}
	return f.getConversationsFn(ctx, req)
}

func (f *fakeGatewayMsgClient) MarkRead(ctx context.Context, req *msgpb.MarkReadRequest) (*msgpb.MarkReadResponse, error) {
	if f.markReadFn == nil {
		return nil, errors.New("unexpected MarkRead call")
	}
	return f.markReadFn(ctx, req)
}

func (f *fakeGatewayMsgClient) DeleteConversation(ctx context.Context, req *msgpb.DeleteConversationRequest) (*msgpb.DeleteConversationResponse, error) {
	if f.deleteConversationFn == nil {
		return nil, errors.New("unexpected DeleteConversation call")
	}
	return f.deleteConversationFn(ctx, req)
}

func (f *fakeGatewayMsgClient) UpdateConversationSettings(ctx context.Context, req *msgpb.UpdateConvSettingsRequest) (*msgpb.UpdateConvSettingsResponse, error) {
	if f.updateConversationSettingsFn == nil {
		return nil, errors.New("unexpected UpdateConversationSettings call")
	}
	return f.updateConversationSettingsFn(ctx, req)
}

func TestGatewayMsgServiceSendMessage(t *testing.T) {
	t.Run("success_mapping", func(t *testing.T) {
		ctx := ctxmeta.WithUserUUID(context.Background(), "u-from")
		ctx = ctxmeta.WithDeviceID(ctx, "dev-1")

		client := &fakeGatewayMsgClient{
			sendMessageFn: func(_ context.Context, req *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error) {
				assert.Equal(t, "u-from", req.FromUuid)
				assert.Equal(t, "dev-1", req.DeviceId)
				assert.Equal(t, "c1", req.ClientMsgId)
				return &msgpb.SendMessageResponse{
					MsgId:    "m1",
					Seq:      10,
					ConvId:   "conv-1",
					SendTime: 1700000000,
				}, nil
			},
		}
		svc := NewMsgService(client)
		resp, err := svc.SendMessage(ctx, &dto.SendMessageRequest{
			ClientMsgID: "c1",
			ConvType:    1,
			TargetUUID:  "u-to",
			MsgType:     1,
			Content:     "hi",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "m1", resp.MsgID)
		assert.Equal(t, int64(10), resp.Seq)
		assert.Equal(t, "conv-1", resp.ConvID)
		assert.Equal(t, int64(1700000000), resp.SendTime)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		ctx := ctxmeta.WithUserUUID(context.Background(), "u1")
		client := &fakeGatewayMsgClient{
			sendMessageFn: func(_ context.Context, _ *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewMsgService(client)
		resp, err := svc.SendMessage(ctx, &dto.SendMessageRequest{
			ClientMsgID: "c1",
			ConvType:    1,
			TargetUUID:  "u2",
			MsgType:     1,
			Content:     "x",
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayMsgServicePullMessages(t *testing.T) {
	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("pull failed")
		client := &fakeGatewayMsgClient{
			pullMessagesFn: func(_ context.Context, _ *msgpb.PullMessagesRequest) (*msgpb.PullMessagesResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewMsgService(client)
		resp, err := svc.PullMessages(context.Background(), &dto.PullMessagesRequest{ConvID: "conv-x"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayMsgServiceRecallMessage(t *testing.T) {
	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("recall failed")
		ctx := ctxmeta.WithUserUUID(context.Background(), "op-1")
		client := &fakeGatewayMsgClient{
			recallMessageFn: func(_ context.Context, req *msgpb.RecallMessageRequest) (*msgpb.RecallMessageResponse, error) {
				assert.Equal(t, "op-1", req.OperatorUuid)
				assert.Equal(t, "conv-1", req.ConvId)
				assert.Equal(t, "m1", req.MsgId)
				return nil, wantErr
			},
		}
		svc := NewMsgService(client)
		err := svc.RecallMessage(ctx, &dto.RecallMessageRequest{ConvID: "conv-1", MsgID: "m1"})
		require.ErrorIs(t, err, wantErr)
	})
}
