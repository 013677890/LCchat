package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMsgHTTPService struct {
	sendMessageFn                func(context.Context, *dto.SendMessageRequest) (*dto.SendMessageResponse, error)
	pullMessagesFn               func(context.Context, *dto.PullMessagesRequest) (*dto.PullMessagesResponse, error)
	batchSyncMessagesFn          func(context.Context, *dto.BatchSyncMessagesRequest) (*dto.BatchSyncMessagesResponse, error)
	getMessagesByIdsFn           func(context.Context, *dto.GetMessagesByIdsRequest) (*dto.GetMessagesByIdsResponse, error)
	updateConversationSettingsFn func(context.Context, *dto.UpdateConvSettingsRequest) error
}

var _ service.MsgService = (*fakeMsgHTTPService)(nil)

func (f *fakeMsgHTTPService) SendMessage(ctx context.Context, req *dto.SendMessageRequest) (*dto.SendMessageResponse, error) {
	if f.sendMessageFn == nil {
		return &dto.SendMessageResponse{}, nil
	}
	return f.sendMessageFn(ctx, req)
}

func (f *fakeMsgHTTPService) PullMessages(ctx context.Context, req *dto.PullMessagesRequest) (*dto.PullMessagesResponse, error) {
	if f.pullMessagesFn == nil {
		return &dto.PullMessagesResponse{}, nil
	}
	return f.pullMessagesFn(ctx, req)
}

func (f *fakeMsgHTTPService) BatchSyncMessages(ctx context.Context, req *dto.BatchSyncMessagesRequest) (*dto.BatchSyncMessagesResponse, error) {
	if f.batchSyncMessagesFn == nil {
		return &dto.BatchSyncMessagesResponse{}, nil
	}
	return f.batchSyncMessagesFn(ctx, req)
}

func (f *fakeMsgHTTPService) GetMessagesByIds(ctx context.Context, req *dto.GetMessagesByIdsRequest) (*dto.GetMessagesByIdsResponse, error) {
	if f.getMessagesByIdsFn == nil {
		return &dto.GetMessagesByIdsResponse{}, nil
	}
	return f.getMessagesByIdsFn(ctx, req)
}

func (f *fakeMsgHTTPService) RecallMessage(context.Context, *dto.RecallMessageRequest) error {
	return nil
}

func (f *fakeMsgHTTPService) GetConversations(context.Context, *dto.GetConversationsRequest) (*dto.GetConversationsResponse, error) {
	return &dto.GetConversationsResponse{}, nil
}

func (f *fakeMsgHTTPService) MarkRead(context.Context, *dto.MarkReadRequest) (*dto.MarkReadResponse, error) {
	return &dto.MarkReadResponse{}, nil
}

func (f *fakeMsgHTTPService) DeleteConversation(context.Context, *dto.DeleteConversationRequest) error {
	return nil
}

func (f *fakeMsgHTTPService) UpdateConversationSettings(ctx context.Context, req *dto.UpdateConvSettingsRequest) error {
	if f.updateConversationSettingsFn == nil {
		return nil
	}
	return f.updateConversationSettingsFn(ctx, req)
}

type msgHandlerResultBody struct {
	Code int `json:"code"`
}

var gatewayMsgHandlerLoggerOnce sync.Once

func initGatewayMsgHandlerLogger() {
	gatewayMsgHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}

func decodeMsgHandlerCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body msgHandlerResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func newMsgJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestMsgHandlerSendMessageValidatesBinding(t *testing.T) {
	initGatewayMsgHandlerLogger()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty_client_msg_id",
			body: `{"clientMsgId":"","convType":1,"targetUuid":"u2","msgType":1,"content":"hi"}`,
		},
		{
			name: "invalid_conv_type",
			body: `{"clientMsgId":"c1","convType":3,"targetUuid":"u2","msgType":1,"content":"hi"}`,
		},
		{
			name: "content_too_large",
			body: `{"clientMsgId":"c1","convType":1,"targetUuid":"u2","msgType":1,"content":"` + strings.Repeat("x", 65537) + `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := NewMsgHandler(&fakeMsgHTTPService{
				sendMessageFn: func(context.Context, *dto.SendMessageRequest) (*dto.SendMessageResponse, error) {
					called = true
					return &dto.SendMessageResponse{}, nil
				},
			})
			w := httptest.NewRecorder()
			req := newMsgJSONRequest(t, http.MethodPost, "/api/v1/auth/messages/send", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.SendMessage(c)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeParamError, decodeMsgHandlerCode(t, w))
			assert.False(t, called)
		})
	}
}

func TestMsgHandlerPullMessagesValidatesLimit(t *testing.T) {
	initGatewayMsgHandlerLogger()

	called := false
	h := NewMsgHandler(&fakeMsgHTTPService{
		pullMessagesFn: func(context.Context, *dto.PullMessagesRequest) (*dto.PullMessagesResponse, error) {
			called = true
			return &dto.PullMessagesResponse{}, nil
		},
	})
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/messages/pull?convId=conv-1&limit=201", nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.PullMessages(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeParamError, decodeMsgHandlerCode(t, w))
	assert.False(t, called)
}

func TestMsgHandlerBatchSyncMessagesValidatesBatchContract(t *testing.T) {
	initGatewayMsgHandlerLogger()

	overBudgetItems := make([]map[string]any, 0, 11)
	for index := 0; index < 11; index++ {
		overBudgetItems = append(overBudgetItems, map[string]any{
			"convId": fmt.Sprintf("conv-%d", index),
			"limit":  50,
		})
	}
	overBudgetBody, err := json.Marshal(map[string]any{"conversations": overBudgetItems})
	require.NoError(t, err)

	tests := []struct {
		name string
		body string
	}{
		{name: "empty_conversations", body: `{"conversations":[]}`},
		{name: "null_conversation", body: `{"conversations":[null]}`},
		{name: "blank_conversation_id", body: `{"conversations":[{"convId":"  ","afterSeq":0}]}`},
		{name: "negative_after_seq", body: `{"conversations":[{"convId":"conv-1","afterSeq":-1}]}`},
		{name: "limit_above_cap", body: `{"conversations":[{"convId":"conv-1","limit":51}]}`},
		{
			name: "duplicate_conversation_id",
			body: `{"conversations":[{"convId":"conv-1"},{"convId":"conv-1"}]}`,
		},
		{name: "total_limit_above_cap", body: string(overBudgetBody)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := NewMsgHandler(&fakeMsgHTTPService{
				batchSyncMessagesFn: func(context.Context, *dto.BatchSyncMessagesRequest) (*dto.BatchSyncMessagesResponse, error) {
					called = true
					return &dto.BatchSyncMessagesResponse{}, nil
				},
			})
			w := httptest.NewRecorder()
			req := newMsgJSONRequest(t, http.MethodPost, "/api/v1/auth/messages/sync-batch", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.BatchSyncMessages(c)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeParamError, decodeMsgHandlerCode(t, w))
			assert.False(t, called)
		})
	}
}

func TestMsgHandlerBatchSyncMessagesForwardsIndependentCursors(t *testing.T) {
	initGatewayMsgHandlerLogger()

	called := false
	h := NewMsgHandler(&fakeMsgHTTPService{
		batchSyncMessagesFn: func(_ context.Context, req *dto.BatchSyncMessagesRequest) (*dto.BatchSyncMessagesResponse, error) {
			called = true
			require.Len(t, req.Conversations, 2)
			assert.Equal(t, "conv-1", req.Conversations[0].ConvID)
			assert.Equal(t, int64(7), req.Conversations[0].AfterSeq)
			assert.Equal(t, "conv-2", req.Conversations[1].ConvID)
			assert.Equal(t, int64(20), req.Conversations[1].AfterSeq)
			assert.Equal(t, int32(3), req.Conversations[1].Limit)
			return &dto.BatchSyncMessagesResponse{
				Results: []*dto.ConversationSyncResultDTO{
					{ConvID: "conv-1", NextSeq: 8},
					{ConvID: "conv-2", NextSeq: 20, ErrorCode: int32(consts.CodeConversationNotFound)},
				},
			}, nil
		},
	})
	w := httptest.NewRecorder()
	req := newMsgJSONRequest(
		t,
		http.MethodPost,
		"/api/v1/auth/messages/sync-batch",
		`{"conversations":[{"convId":"conv-1","afterSeq":7},{"convId":"conv-2","afterSeq":20,"limit":3}]}`,
	)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.BatchSyncMessages(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, decodeMsgHandlerCode(t, w))
	assert.True(t, called)
}

func TestMsgHandlerGetMessagesByIdsValidatesIDs(t *testing.T) {
	initGatewayMsgHandlerLogger()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty_ids", body: `{"convId":"conv-1","msgIds":[]}`},
		{name: "blank_id", body: `{"convId":"conv-1","msgIds":[""]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := NewMsgHandler(&fakeMsgHTTPService{
				getMessagesByIdsFn: func(context.Context, *dto.GetMessagesByIdsRequest) (*dto.GetMessagesByIdsResponse, error) {
					called = true
					return &dto.GetMessagesByIdsResponse{}, nil
				},
			})
			w := httptest.NewRecorder()
			req := newMsgJSONRequest(t, http.MethodPost, "/api/v1/auth/messages/get-by-ids", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.GetMessagesByIds(c)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeParamError, decodeMsgHandlerCode(t, w))
			assert.False(t, called)
		})
	}
}

func TestMsgHandlerUpdateConversationSettingsRequiresPatchField(t *testing.T) {
	initGatewayMsgHandlerLogger()

	t.Run("no_patch_field", func(t *testing.T) {
		called := false
		h := NewMsgHandler(&fakeMsgHTTPService{
			updateConversationSettingsFn: func(context.Context, *dto.UpdateConvSettingsRequest) error {
				called = true
				return nil
			},
		})
		w := httptest.NewRecorder()
		req := newMsgJSONRequest(t, http.MethodPatch, "/api/v1/auth/conversations/settings", `{"convId":"conv-1"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.UpdateConversationSettings(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeMsgHandlerCode(t, w))
		assert.False(t, called)
	})

	t.Run("explicit_false_is_valid", func(t *testing.T) {
		called := false
		h := NewMsgHandler(&fakeMsgHTTPService{
			updateConversationSettingsFn: func(_ context.Context, req *dto.UpdateConvSettingsRequest) error {
				called = true
				require.NotNil(t, req.Mute)
				require.False(t, *req.Mute)
				require.Nil(t, req.Pin)
				return nil
			},
		})
		w := httptest.NewRecorder()
		req := newMsgJSONRequest(t, http.MethodPatch, "/api/v1/auth/conversations/settings", `{"convId":"conv-1","mute":false}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.UpdateConversationSettings(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeMsgHandlerCode(t, w))
		assert.True(t, called)
	})
}
