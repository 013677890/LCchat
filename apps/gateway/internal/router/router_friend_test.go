package router

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
	v1 "github.com/013677890/LCchat-Backend/apps/gateway/internal/router/v1"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeRouterFriendService struct {
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

var _ service.FriendService = (*fakeRouterFriendService)(nil)

func (f *fakeRouterFriendService) SendFriendApply(ctx context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
	if f.sendApplyFn == nil {
		return &dto.SendFriendApplyResponse{}, nil
	}
	return f.sendApplyFn(ctx, req)
}

func (f *fakeRouterFriendService) GetFriendApplyList(ctx context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error) {
	if f.applyListFn == nil {
		return &dto.GetFriendApplyListResponse{}, nil
	}
	return f.applyListFn(ctx, req)
}

func (f *fakeRouterFriendService) GetSentApplyList(ctx context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error) {
	if f.sentApplyListFn == nil {
		return &dto.GetSentApplyListResponse{}, nil
	}
	return f.sentApplyListFn(ctx, req)
}

func (f *fakeRouterFriendService) HandleFriendApply(ctx context.Context, req *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error) {
	if f.handleApplyFn == nil {
		return &dto.HandleFriendApplyResponse{}, nil
	}
	return f.handleApplyFn(ctx, req)
}

func (f *fakeRouterFriendService) GetUnreadApplyCount(ctx context.Context, req *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error) {
	if f.unreadCountFn == nil {
		return &dto.GetUnreadApplyCountResponse{}, nil
	}
	return f.unreadCountFn(ctx, req)
}

func (f *fakeRouterFriendService) MarkApplyAsRead(ctx context.Context, req *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error) {
	if f.markReadFn == nil {
		return &dto.MarkApplyAsReadResponse{}, nil
	}
	return f.markReadFn(ctx, req)
}

func (f *fakeRouterFriendService) GetFriendList(ctx context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
	if f.friendListFn == nil {
		return &dto.GetFriendListResponse{}, nil
	}
	return f.friendListFn(ctx, req)
}

func (f *fakeRouterFriendService) SyncFriendList(ctx context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error) {
	if f.syncFn == nil {
		return &dto.SyncFriendListResponse{}, nil
	}
	return f.syncFn(ctx, req)
}

func (f *fakeRouterFriendService) DeleteFriend(ctx context.Context, req *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error) {
	if f.deleteFn == nil {
		return &dto.DeleteFriendResponse{}, nil
	}
	return f.deleteFn(ctx, req)
}

func (f *fakeRouterFriendService) SetFriendRemark(ctx context.Context, req *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error) {
	if f.remarkFn == nil {
		return &dto.SetFriendRemarkResponse{}, nil
	}
	return f.remarkFn(ctx, req)
}

func (f *fakeRouterFriendService) SetFriendTag(ctx context.Context, req *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error) {
	if f.tagFn == nil {
		return &dto.SetFriendTagResponse{}, nil
	}
	return f.tagFn(ctx, req)
}

func (f *fakeRouterFriendService) CheckIsFriend(ctx context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error) {
	if f.checkFn == nil {
		return &dto.CheckIsFriendResponse{}, nil
	}
	return f.checkFn(ctx, req)
}

func (f *fakeRouterFriendService) GetRelationStatus(ctx context.Context, req *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error) {
	if f.getRelationFn == nil {
		return &dto.GetRelationStatusResponse{}, nil
	}
	return f.getRelationFn(ctx, req)
}

type routerFriendResultBody struct {
	Code int `json:"code"`
}

var routerFriendLoggerOnce sync.Once

func initRouterFriendTestLogger() {
	routerFriendLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
		gin.SetMode(gin.TestMode)
	})
}

func decodeRouterFriendCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body routerFriendResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func mustFriendAuthToken(t *testing.T) string {
	t.Helper()
	token, err := util.GenerateToken("u1", "d1")
	require.NoError(t, err)
	return token
}

func newRouterFriendRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newAuthedRouterFriendRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req := newRouterFriendRequest(t, method, target, body)
	req.Header.Set("Authorization", "Bearer "+mustFriendAuthToken(t))
	return req
}

func buildFriendTestRouter(friendSvc service.FriendService) *gin.Engine {
	authHandler := v1.NewAuthHandler(nil)
	userHandler := v1.NewUserHandler(nil)
	friendHandler := v1.NewFriendHandler(friendSvc)
	blacklistHandler := v1.NewBlacklistHandler(nil)
	deviceHandler := v1.NewDeviceHandler(nil)
	return InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, nil)
}

func TestRouterFriendUnauthorized(t *testing.T) {
	initRouterFriendTestLogger()
	r := buildFriendTestRouter(&fakeRouterFriendService{})

	req := newRouterFriendRequest(t, http.MethodGet, "/api/v1/auth/friend/list", "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, consts.CodeUnauthorized, decodeRouterFriendCode(t, w))
}

func TestRouterFriendRoutesAndSuccess(t *testing.T) {
	initRouterFriendTestLogger()

	tests := []struct {
		name   string
		method string
		target string
		body   string
		setup  func(*fakeRouterFriendService, *bool)
	}{
		{
			name:   "send_apply",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/apply",
			body:   `{"targetUuid":"u2","reason":"hi","source":"search"}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.sendApplyFn = func(_ context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
					*called = true
					require.Equal(t, "u2", req.TargetUUID)
					return &dto.SendFriendApplyResponse{}, nil
				}
			},
		},
		{
			name:   "get_apply_list",
			method: http.MethodGet,
			target: "/api/v1/auth/friend/apply-list?Page=1&PageSize=20",
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.applyListFn = func(_ context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					return &dto.GetFriendApplyListResponse{}, nil
				}
			},
		},
		{
			name:   "get_sent_apply_list",
			method: http.MethodGet,
			target: "/api/v1/auth/friend/apply/sent?Page=1&PageSize=20",
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.sentApplyListFn = func(_ context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					return &dto.GetSentApplyListResponse{}, nil
				}
			},
		},
		{
			name:   "handle_apply",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/apply/handle",
			body:   `{"applyId":1,"action":1}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.handleApplyFn = func(_ context.Context, req *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error) {
					*called = true
					require.Equal(t, int64(1), req.ApplyID)
					return &dto.HandleFriendApplyResponse{}, nil
				}
			},
		},
		{
			name:   "get_unread",
			method: http.MethodGet,
			target: "/api/v1/auth/friend/apply/unread",
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.unreadCountFn = func(_ context.Context, _ *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error) {
					*called = true
					return &dto.GetUnreadApplyCountResponse{UnreadCount: 1}, nil
				}
			},
		},
		{
			name:   "mark_apply_read",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/apply/read",
			body:   `{"applyIds":[1,2]}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.markReadFn = func(_ context.Context, req *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error) {
					*called = true
					require.Equal(t, []int64{1, 2}, req.ApplyIDs)
					return &dto.MarkApplyAsReadResponse{}, nil
				}
			},
		},
		{
			name:   "get_friend_list",
			method: http.MethodGet,
			target: "/api/v1/auth/friend/list?Page=1&PageSize=20",
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.friendListFn = func(_ context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					return &dto.GetFriendListResponse{}, nil
				}
			},
		},
		{
			name:   "get_friend_list_default_pagination",
			method: http.MethodGet,
			target: "/api/v1/auth/friend/list",
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.friendListFn = func(_ context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(20), req.PageSize)
					return &dto.GetFriendListResponse{}, nil
				}
			},
		},
		{
			name:   "sync_friend_list",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/sync",
			body:   `{"version":0}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.syncFn = func(_ context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error) {
					*called = true
					require.Equal(t, int32(100), req.Limit)
					return &dto.SyncFriendListResponse{}, nil
				}
			},
		},
		{
			name:   "delete_friend",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/delete",
			body:   `{"userUuid":"u2"}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.deleteFn = func(_ context.Context, req *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error) {
					*called = true
					require.Equal(t, "u2", req.UserUUID)
					return &dto.DeleteFriendResponse{}, nil
				}
			},
		},
		{
			name:   "set_remark",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/remark",
			body:   `{"userUuid":"u2","remark":"r"}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.remarkFn = func(_ context.Context, req *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error) {
					*called = true
					require.Equal(t, "r", req.Remark)
					return &dto.SetFriendRemarkResponse{}, nil
				}
			},
		},
		{
			name:   "set_tag",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/tag",
			body:   `{"userUuid":"u2","groupTag":"work"}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.tagFn = func(_ context.Context, req *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error) {
					*called = true
					require.Equal(t, "work", req.GroupTag)
					return &dto.SetFriendTagResponse{}, nil
				}
			},
		},
		{
			name:   "check_is_friend",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/check",
			body:   `{"userUuid":"u1","peerUuid":"u2"}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.checkFn = func(_ context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error) {
					*called = true
					require.Equal(t, "u2", req.PeerUUID)
					return &dto.CheckIsFriendResponse{IsFriend: true}, nil
				}
			},
		},
		{
			name:   "relation_status",
			method: http.MethodPost,
			target: "/api/v1/auth/friend/relation",
			body:   `{"userUuid":"u1","peerUuid":"u2"}`,
			setup: func(s *fakeRouterFriendService, called *bool) {
				s.getRelationFn = func(_ context.Context, req *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error) {
					*called = true
					require.Equal(t, "u1", req.UserUUID)
					return &dto.GetRelationStatusResponse{Relation: "friend"}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &fakeRouterFriendService{}
			tt.setup(svc, &called)
			r := buildFriendTestRouter(svc)

			req := newAuthedRouterFriendRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeSuccess, decodeRouterFriendCode(t, w))
			assert.True(t, called)
		})
	}
}

func TestRouterFriendParamErrors(t *testing.T) {
	initRouterFriendTestLogger()

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   int
	}{
		{
			name:       "send_apply_invalid_json",
			method:     http.MethodPost,
			target:     "/api/v1/auth/friend/apply",
			body:       "{",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "get_apply_list_invalid_query",
			method:     http.MethodGet,
			target:     "/api/v1/auth/friend/apply-list?Page=abc",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "handle_apply_invalid_json",
			method:     http.MethodPost,
			target:     "/api/v1/auth/friend/apply/handle",
			body:       "{",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "sync_invalid_json",
			method:     http.MethodPost,
			target:     "/api/v1/auth/friend/sync",
			body:       "{",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := buildFriendTestRouter(&fakeRouterFriendService{})
			req := newAuthedRouterFriendRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeRouterFriendCode(t, w))
		})
	}
}

func TestRouterFriendErrorMapping(t *testing.T) {
	initRouterFriendTestLogger()

	t.Run("business_error_passthrough", func(t *testing.T) {
		svc := &fakeRouterFriendService{
			sendApplyFn: func(_ context.Context, _ *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeFriendRequestSent))
			},
			getRelationFn: func(_ context.Context, _ *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeNoPermission))
			},
		}
		r := buildFriendTestRouter(svc)

		w1 := httptest.NewRecorder()
		req1 := newAuthedRouterFriendRequest(t, http.MethodPost, "/api/v1/auth/friend/apply", `{"targetUuid":"u2"}`)
		r.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)
		assert.Equal(t, consts.CodeFriendRequestSent, decodeRouterFriendCode(t, w1))

		w2 := httptest.NewRecorder()
		req2 := newAuthedRouterFriendRequest(t, http.MethodPost, "/api/v1/auth/friend/relation", `{"userUuid":"u1","peerUuid":"u2"}`)
		r.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusOK, w2.Code)
		assert.Equal(t, consts.CodeNoPermission, decodeRouterFriendCode(t, w2))
	})

	t.Run("internal_error_to_code_internal", func(t *testing.T) {
		svc := &fakeRouterFriendService{
			deleteFn: func(_ context.Context, _ *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error) {
				return nil, errors.New("internal")
			},
			friendListFn: func(_ context.Context, _ *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
				return nil, errors.New("internal")
			},
		}
		r := buildFriendTestRouter(svc)

		w1 := httptest.NewRecorder()
		req1 := newAuthedRouterFriendRequest(t, http.MethodPost, "/api/v1/auth/friend/delete", `{"userUuid":"u2"}`)
		r.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusInternalServerError, w1.Code)
		assert.Equal(t, consts.CodeInternalError, decodeRouterFriendCode(t, w1))

		w2 := httptest.NewRecorder()
		req2 := newAuthedRouterFriendRequest(t, http.MethodGet, "/api/v1/auth/friend/list?Page=1&PageSize=20", "")
		r.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusInternalServerError, w2.Code)
		assert.Equal(t, consts.CodeInternalError, decodeRouterFriendCode(t, w2))
	})
}
