package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFriendHTTPService struct {
	sendApplyFn     func(context.Context, *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error)
	applyListFn     func(context.Context, *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error)
	sentApplyListFn func(context.Context, *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error)
	handleApplyFn   func(context.Context, *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error)
	unreadCountFn   func(context.Context, *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error)
	markReadFn      func(context.Context, *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error)
	friendListFn    func(context.Context, *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error)
	syncFn          func(context.Context, *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error)
	deleteFn        func(context.Context, *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error)
	remarkFn        func(context.Context, *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error)
	tagFn           func(context.Context, *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error)
	checkFn         func(context.Context, *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error)
	getRelationFn   func(context.Context, *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error)
}

var _ service.FriendService = (*fakeFriendHTTPService)(nil)

func (f *fakeFriendHTTPService) SendFriendApply(ctx context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
	if f.sendApplyFn == nil {
		return &dto.SendFriendApplyResponse{}, nil
	}
	return f.sendApplyFn(ctx, req)
}

func (f *fakeFriendHTTPService) GetFriendApplyList(ctx context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error) {
	if f.applyListFn == nil {
		return &dto.GetFriendApplyListResponse{}, nil
	}
	return f.applyListFn(ctx, req)
}

func (f *fakeFriendHTTPService) GetSentApplyList(ctx context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error) {
	if f.sentApplyListFn == nil {
		return &dto.GetSentApplyListResponse{}, nil
	}
	return f.sentApplyListFn(ctx, req)
}

func (f *fakeFriendHTTPService) HandleFriendApply(ctx context.Context, req *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error) {
	if f.handleApplyFn == nil {
		return &dto.HandleFriendApplyResponse{}, nil
	}
	return f.handleApplyFn(ctx, req)
}

func (f *fakeFriendHTTPService) GetUnreadApplyCount(ctx context.Context, req *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error) {
	if f.unreadCountFn == nil {
		return &dto.GetUnreadApplyCountResponse{}, nil
	}
	return f.unreadCountFn(ctx, req)
}

func (f *fakeFriendHTTPService) MarkApplyAsRead(ctx context.Context, req *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error) {
	if f.markReadFn == nil {
		return &dto.MarkApplyAsReadResponse{}, nil
	}
	return f.markReadFn(ctx, req)
}

func (f *fakeFriendHTTPService) GetFriendList(ctx context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
	if f.friendListFn == nil {
		return &dto.GetFriendListResponse{}, nil
	}
	return f.friendListFn(ctx, req)
}

func (f *fakeFriendHTTPService) SyncFriendList(ctx context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error) {
	if f.syncFn == nil {
		return &dto.SyncFriendListResponse{}, nil
	}
	return f.syncFn(ctx, req)
}

func (f *fakeFriendHTTPService) DeleteFriend(ctx context.Context, req *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error) {
	if f.deleteFn == nil {
		return &dto.DeleteFriendResponse{}, nil
	}
	return f.deleteFn(ctx, req)
}

func (f *fakeFriendHTTPService) SetFriendRemark(ctx context.Context, req *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error) {
	if f.remarkFn == nil {
		return &dto.SetFriendRemarkResponse{}, nil
	}
	return f.remarkFn(ctx, req)
}

func (f *fakeFriendHTTPService) SetFriendTag(ctx context.Context, req *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error) {
	if f.tagFn == nil {
		return &dto.SetFriendTagResponse{}, nil
	}
	return f.tagFn(ctx, req)
}

func (f *fakeFriendHTTPService) CheckIsFriend(ctx context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error) {
	if f.checkFn == nil {
		return &dto.CheckIsFriendResponse{}, nil
	}
	return f.checkFn(ctx, req)
}

func (f *fakeFriendHTTPService) GetRelationStatus(ctx context.Context, req *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error) {
	if f.getRelationFn == nil {
		return &dto.GetRelationStatusResponse{}, nil
	}
	return f.getRelationFn(ctx, req)
}

type friendHandlerResultBody struct {
	Code int `json:"code"`
}

var gatewayFriendHandlerLoggerOnce sync.Once

func initGatewayFriendHandlerLogger() {
	gatewayFriendHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}

func decodeFriendHandlerCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body friendHandlerResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func newFriendJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestFriendHandlerSendFriendApply(t *testing.T) {
	initGatewayFriendHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeFriendHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name:       "bind_failed",
			body:       "{",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name: "success",
			body: `{"targetUuid":"u2","reason":"hello","source":"search"}`,
			setupSvc: func(s *fakeFriendHTTPService, called *bool) {
				s.sendApplyFn = func(_ context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
					*called = true
					require.Equal(t, "u2", req.TargetUUID)
					require.Equal(t, "hello", req.Reason)
					require.Equal(t, "search", req.Source)
					return &dto.SendFriendApplyResponse{ApplyID: 1001}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"targetUuid":"u2","reason":"hello","source":"search"}`,
			setupSvc: func(s *fakeFriendHTTPService, called *bool) {
				s.sendApplyFn = func(_ context.Context, _ *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
					*called = true
					return nil, apperr.New(consts.CodeFriendRequestSent)
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeFriendRequestSent,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"targetUuid":"u2","reason":"hello","source":"search"}`,
			setupSvc: func(s *fakeFriendHTTPService, called *bool) {
				s.sendApplyFn = func(_ context.Context, _ *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
					*called = true
					return nil, errors.New("internal")
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   consts.CodeInternalError,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &fakeFriendHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewFriendHandler(svc)

			w := httptest.NewRecorder()
			req := newFriendJSONRequest(t, http.MethodPost, "/api/v1/auth/friend/apply", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.SendFriendApply(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeFriendHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestFriendHandlerGetApplyLists(t *testing.T) {
	initGatewayFriendHandlerLogger()

	t.Run("get_friend_apply_list_bind_failed", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHTTPService{})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/friend/apply-list?page=abc", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetFriendApplyList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeFriendHandlerCode(t, w))
	})

	t.Run("get_friend_apply_list_default_status_pending", func(t *testing.T) {
		called := false
		h := NewFriendHandler(&fakeFriendHTTPService{
			applyListFn: func(_ context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error) {
				called = true
				require.Equal(t, int32(0), req.Status)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return &dto.GetFriendApplyListResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/friend/apply-list?page=1&pageSize=20", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetFriendApplyList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeFriendHandlerCode(t, w))
		assert.True(t, called)
	})

	t.Run("get_friend_apply_list_status_all", func(t *testing.T) {
		called := false
		h := NewFriendHandler(&fakeFriendHTTPService{
			applyListFn: func(_ context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error) {
				called = true
				require.Equal(t, int32(-1), req.Status)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return &dto.GetFriendApplyListResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/friend/apply-list?status=-1&page=1&pageSize=20", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetFriendApplyList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeFriendHandlerCode(t, w))
		assert.True(t, called)
	})

	t.Run("get_sent_apply_list_default_status_pending", func(t *testing.T) {
		called := false
		h := NewFriendHandler(&fakeFriendHTTPService{
			sentApplyListFn: func(_ context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error) {
				called = true
				require.Equal(t, int32(0), req.Status)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return &dto.GetSentApplyListResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/friend/apply/sent?page=1&pageSize=20", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetSentApplyList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeFriendHandlerCode(t, w))
		assert.True(t, called)
	})

	t.Run("get_sent_apply_list_status_all", func(t *testing.T) {
		called := false
		h := NewFriendHandler(&fakeFriendHTTPService{
			sentApplyListFn: func(_ context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error) {
				called = true
				require.Equal(t, int32(-1), req.Status)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return &dto.GetSentApplyListResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/friend/apply/sent?status=-1&page=1&pageSize=20", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetSentApplyList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeFriendHandlerCode(t, w))
		assert.True(t, called)
	})
}

func TestFriendHandlerHandleAndMarkApply(t *testing.T) {
	initGatewayFriendHandlerLogger()

	t.Run("handle_apply_bind_failed", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHTTPService{})
		w := httptest.NewRecorder()
		req := newFriendJSONRequest(t, http.MethodPost, "/api/v1/auth/friend/apply/handle", "{")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.HandleFriendApply(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeFriendHandlerCode(t, w))
	})

	t.Run("handle_apply_business_error", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHTTPService{
			handleApplyFn: func(_ context.Context, req *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error) {
				require.Equal(t, int64(1), req.ApplyID)
				return nil, apperr.New(consts.CodeNoPermission)
			},
		})
		w := httptest.NewRecorder()
		req := newFriendJSONRequest(t, http.MethodPost, "/api/v1/auth/friend/apply/handle", `{"applyId":1,"action":2}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.HandleFriendApply(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeNoPermission, decodeFriendHandlerCode(t, w))
	})

	t.Run("mark_apply_as_read_empty_ids", func(t *testing.T) {
		called := false
		h := NewFriendHandler(&fakeFriendHTTPService{
			markReadFn: func(_ context.Context, _ *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error) {
				called = true
				return &dto.MarkApplyAsReadResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newFriendJSONRequest(t, http.MethodPost, "/api/v1/auth/friend/apply/read", `{"applyIds":[]}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.MarkApplyAsRead(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeFriendHandlerCode(t, w))
		assert.False(t, called)
	})

	t.Run("mark_apply_as_read_internal_error", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHTTPService{
			markReadFn: func(_ context.Context, req *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error) {
				require.Equal(t, []int64{1, 2}, req.ApplyIDs)
				return nil, errors.New("internal")
			},
		})
		w := httptest.NewRecorder()
		req := newFriendJSONRequest(t, http.MethodPost, "/api/v1/auth/friend/apply/read", `{"applyIds":[1,2]}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.MarkApplyAsRead(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, consts.CodeInternalError, decodeFriendHandlerCode(t, w))
	})
}

func TestFriendHandlerFriendListAndSync(t *testing.T) {
	initGatewayFriendHandlerLogger()

	t.Run("get_friend_list_with_valid_query", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHTTPService{
			friendListFn: func(_ context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				require.Equal(t, "work", req.GroupTag)
				return &dto.GetFriendListResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/friend/list?page=1&pageSize=20&groupTag=work", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetFriendList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeFriendHandlerCode(t, w))
	})

	t.Run("sync_default_limit", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHTTPService{
			syncFn: func(_ context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error) {
				require.Equal(t, int32(100), req.Limit)
				return &dto.SyncFriendListResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newFriendJSONRequest(t, http.MethodPost, "/api/v1/auth/friend/sync", `{"version":0}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SyncFriendList(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeFriendHandlerCode(t, w))
	})
}

func TestFriendHandlerSimpleMethods(t *testing.T) {
	initGatewayFriendHandlerLogger()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		invoke     func(*FriendHandler, *gin.Context)
		setupSvc   func(*fakeFriendHTTPService)
		wantStatus int
		wantCode   int
	}{
		{
			name:   "get_unread_success",
			method: http.MethodGet,
			path:   "/api/v1/auth/friend/apply/unread",
			invoke: func(h *FriendHandler, c *gin.Context) { h.GetUnreadApplyCount(c) },
			setupSvc: func(s *fakeFriendHTTPService) {
				s.unreadCountFn = func(_ context.Context, _ *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error) {
					return &dto.GetUnreadApplyCountResponse{UnreadCount: 1}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
		},
		{
			name:   "delete_friend_internal_error",
			method: http.MethodPost,
			path:   "/api/v1/auth/friend/delete",
			body:   `{"userUuid":"u2"}`,
			invoke: func(h *FriendHandler, c *gin.Context) { h.DeleteFriend(c) },
			setupSvc: func(s *fakeFriendHTTPService) {
				s.deleteFn = func(_ context.Context, _ *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error) {
					return nil, errors.New("internal")
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   consts.CodeInternalError,
		},
		{
			name:   "set_remark_success",
			method: http.MethodPost,
			path:   "/api/v1/auth/friend/remark",
			body:   `{"userUuid":"u2","remark":"buddy"}`,
			invoke: func(h *FriendHandler, c *gin.Context) { h.SetFriendRemark(c) },
			setupSvc: func(s *fakeFriendHTTPService) {
				s.remarkFn = func(_ context.Context, req *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error) {
					require.Equal(t, "u2", req.UserUUID)
					require.Equal(t, "buddy", req.Remark)
					return &dto.SetFriendRemarkResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
		},
		{
			name:   "set_tag_success",
			method: http.MethodPost,
			path:   "/api/v1/auth/friend/tag",
			body:   `{"userUuid":"u2","groupTag":"work"}`,
			invoke: func(h *FriendHandler, c *gin.Context) { h.SetFriendTag(c) },
			setupSvc: func(s *fakeFriendHTTPService) {
				s.tagFn = func(_ context.Context, req *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error) {
					require.Equal(t, "work", req.GroupTag)
					return &dto.SetFriendTagResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
		},
		{
			name:   "check_is_friend_success",
			method: http.MethodPost,
			path:   "/api/v1/auth/friend/check",
			body:   `{"userUuid":"u1","peerUuid":"u2"}`,
			invoke: func(h *FriendHandler, c *gin.Context) { h.CheckIsFriend(c) },
			setupSvc: func(s *fakeFriendHTTPService) {
				s.checkFn = func(_ context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error) {
					require.Equal(t, "u1", req.UserUUID)
					require.Equal(t, "u2", req.PeerUUID)
					return &dto.CheckIsFriendResponse{IsFriend: true}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
		},
		{
			name:   "relation_status_business_error",
			method: http.MethodPost,
			path:   "/api/v1/auth/friend/relation",
			body:   `{"userUuid":"u1","peerUuid":"u2"}`,
			invoke: func(h *FriendHandler, c *gin.Context) { h.GetRelationStatus(c) },
			setupSvc: func(s *fakeFriendHTTPService) {
				s.getRelationFn = func(_ context.Context, _ *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error) {
					return nil, apperr.New(consts.CodeParamError)
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeFriendHTTPService{}
			tt.setupSvc(svc)
			h := NewFriendHandler(svc)

			w := httptest.NewRecorder()
			req := newFriendJSONRequest(t, tt.method, tt.path, tt.body)
			if tt.method == http.MethodGet {
				req.Body = http.NoBody
			}
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			// 模拟鉴权中间件已注入当前登录用户身份；CheckIsFriend/GetRelationStatus 会以该身份覆盖 body 的 userUuid。
			ctxmeta.SetUserUUID(c, "u1")

			tt.invoke(h, c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeFriendHandlerCode(t, w))
		})
	}
}
