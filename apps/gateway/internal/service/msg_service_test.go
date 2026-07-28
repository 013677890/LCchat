package service

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGatewayMsgClient struct {
	gatewaypb.MsgServiceClient

	sendMessageFn                func(context.Context, *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error)
	pullMessagesFn               func(context.Context, *msgpb.PullMessagesRequest) (*msgpb.PullMessagesResponse, error)
	batchSyncMessagesFn          func(context.Context, *msgpb.BatchSyncMessagesRequest) (*msgpb.BatchSyncMessagesResponse, error)
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

func (f *fakeGatewayMsgClient) BatchSyncMessages(ctx context.Context, req *msgpb.BatchSyncMessagesRequest) (*msgpb.BatchSyncMessagesResponse, error) {
	if f.batchSyncMessagesFn == nil {
		return nil, errors.New("unexpected BatchSyncMessages call")
	}
	return f.batchSyncMessagesFn(ctx, req)
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
				// 身份由 gRPC metadata 携带（客户端拦截器注入），请求体只承载业务字段。
				assert.Equal(t, "c1", req.ClientMsgId)
				assert.Equal(t, "u-to", req.TargetUuid)
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

	t.Run("rejects_out_of_contract_result_order", func(t *testing.T) {
		client := &fakeGatewayMsgClient{
			batchSyncMessagesFn: func(context.Context, *msgpb.BatchSyncMessagesRequest) (*msgpb.BatchSyncMessagesResponse, error) {
				return &msgpb.BatchSyncMessagesResponse{
					Results: []*msgpb.ConversationSyncResult{{ConvId: "another-conversation"}},
				}, nil
			},
		}
		svc := NewMsgService(client)

		resp, err := svc.BatchSyncMessages(context.Background(), &dto.BatchSyncMessagesRequest{
			Conversations: []*dto.ConversationSyncCursorDTO{{ConvID: "conv-1"}},
		})

		require.Nil(t, resp)
		require.Error(t, err)
		assert.Equal(t, consts.CodeInternalError, apperr.Code(err))
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

func TestGatewayMsgServiceBatchSyncMessages(t *testing.T) {
	t.Run("maps_independent_cursors_and_results", func(t *testing.T) {
		client := &fakeGatewayMsgClient{
			batchSyncMessagesFn: func(_ context.Context, req *msgpb.BatchSyncMessagesRequest) (*msgpb.BatchSyncMessagesResponse, error) {
				require.Len(t, req.Conversations, 2)
				assert.Equal(t, "conv-1", req.Conversations[0].ConvId)
				assert.Equal(t, int64(7), req.Conversations[0].AfterSeq)
				assert.Zero(t, req.Conversations[0].Limit)
				assert.Equal(t, "conv-2", req.Conversations[1].ConvId)
				assert.Equal(t, int64(20), req.Conversations[1].AfterSeq)
				assert.Equal(t, int32(5), req.Conversations[1].Limit)

				return &msgpb.BatchSyncMessagesResponse{
					Results: []*msgpb.ConversationSyncResult{
						{
							ConvId:   "conv-1",
							Messages: []*msgpb.MsgItem{{MsgId: "msg-8", Seq: 8}},
							HasMore:  true,
							MaxSeq:   10,
							NextSeq:  8,
						},
						{
							ConvId:    "conv-2",
							NextSeq:   20,
							ErrorCode: int32(consts.CodeConversationNotFound),
						},
					},
				}, nil
			},
		}
		svc := NewMsgService(client)

		resp, err := svc.BatchSyncMessages(context.Background(), &dto.BatchSyncMessagesRequest{
			Conversations: []*dto.ConversationSyncCursorDTO{
				{ConvID: "conv-1", AfterSeq: 7},
				{ConvID: "conv-2", AfterSeq: 20, Limit: 5},
			},
		})

		require.NoError(t, err)
		require.Len(t, resp.Results, 2)
		require.Len(t, resp.Results[0].Messages, 1)
		assert.Equal(t, "msg-8", resp.Results[0].Messages[0].MsgID)
		assert.Equal(t, int64(8), resp.Results[0].NextSeq)
		assert.True(t, resp.Results[0].HasMore)
		assert.Equal(t, int32(consts.CodeConversationNotFound), resp.Results[1].ErrorCode)
		assert.Equal(t, int64(20), resp.Results[1].NextSeq)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("batch sync failed")
		client := &fakeGatewayMsgClient{
			batchSyncMessagesFn: func(context.Context, *msgpb.BatchSyncMessagesRequest) (*msgpb.BatchSyncMessagesResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewMsgService(client)

		resp, err := svc.BatchSyncMessages(context.Background(), &dto.BatchSyncMessagesRequest{
			Conversations: []*dto.ConversationSyncCursorDTO{{ConvID: "conv-1"}},
		})

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
