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

type fakeRouterBlacklistService struct {
	addFn    func(context.Context, *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error)
	removeFn func(context.Context, *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error)
	listFn   func(context.Context, *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error)
	checkFn  func(context.Context, *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error)
}

var _ service.BlacklistService = (*fakeRouterBlacklistService)(nil)

func (f *fakeRouterBlacklistService) AddBlacklist(ctx context.Context, req *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
	if f.addFn == nil {
		return &dto.AddBlacklistResponse{}, nil
	}
	return f.addFn(ctx, req)
}

func (f *fakeRouterBlacklistService) RemoveBlacklist(ctx context.Context, req *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
	if f.removeFn == nil {
		return &dto.RemoveBlacklistResponse{}, nil
	}
	return f.removeFn(ctx, req)
}

func (f *fakeRouterBlacklistService) GetBlacklistList(ctx context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
	if f.listFn == nil {
		return &dto.GetBlacklistListResponse{}, nil
	}
	return f.listFn(ctx, req)
}

func (f *fakeRouterBlacklistService) CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
	if f.checkFn == nil {
		return &dto.CheckIsBlacklistResponse{IsBlacklist: false}, nil
	}
	return f.checkFn(ctx, req)
}

type routerResultBody struct {
	Code int `json:"code"`
}

var routerBlacklistLoggerOnce sync.Once

func initRouterBlacklistTestLogger() {
	routerBlacklistLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
		gin.SetMode(gin.TestMode)
	})
}

func mustAuthToken(t *testing.T) string {
	t.Helper()
	token, err := util.GenerateToken("u1", "d1")
	require.NoError(t, err)
	return token
}

func newAuthedJSONRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+mustAuthToken(t))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeRouterResultCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body routerResultBody
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	return body.Code
}

func buildBlacklistTestRouter(blacklistSvc service.BlacklistService) *gin.Engine {
	authHandler := v1.NewAuthHandler(nil)
	userHandler := v1.NewUserHandler(nil)
	friendHandler := v1.NewFriendHandler(nil)
	deviceHandler := v1.NewDeviceHandler(nil)
	blacklistHandler := v1.NewBlacklistHandler(blacklistSvc)
	return InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, nil)
}

func TestRouterBlacklistUnauthorized(t *testing.T) {
	initRouterBlacklistTestLogger()

	r := buildBlacklistTestRouter(&fakeRouterBlacklistService{})
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/blacklist", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, consts.CodeUnauthorized, decodeRouterResultCode(t, w))
}

func TestRouterBlacklistRoutesAndSuccess(t *testing.T) {
	initRouterBlacklistTestLogger()

	tests := []struct {
		name   string
		method string
		target string
		body   string
		setup  func(*fakeRouterBlacklistService, *bool)
	}{
		{
			name:   "post_blacklist",
			method: http.MethodPost,
			target: "/api/v1/auth/blacklist",
			body:   `{"targetUuid":"u2"}`,
			setup: func(s *fakeRouterBlacklistService, called *bool) {
				s.addFn = func(_ context.Context, req *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u2", req.TargetUUID)
					return &dto.AddBlacklistResponse{}, nil
				}
			},
		},
		{
			name:   "get_blacklist",
			method: http.MethodGet,
			target: "/api/v1/auth/blacklist?page=1&pageSize=20",
			body:   "",
			setup: func(s *fakeRouterBlacklistService, called *bool) {
				s.listFn = func(_ context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(20), req.PageSize)
					return &dto.GetBlacklistListResponse{}, nil
				}
			},
		},
		{
			name:   "get_blacklist_default_pagination",
			method: http.MethodGet,
			target: "/api/v1/auth/blacklist",
			body:   "",
			setup: func(s *fakeRouterBlacklistService, called *bool) {
				s.listFn = func(_ context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(20), req.PageSize)
					return &dto.GetBlacklistListResponse{}, nil
				}
			},
		},
		{
			name:   "delete_blacklist",
			method: http.MethodDelete,
			target: "/api/v1/auth/blacklist/u2",
			body:   "",
			setup: func(s *fakeRouterBlacklistService, called *bool) {
				s.removeFn = func(_ context.Context, req *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u2", req.UserUUID)
					return &dto.RemoveBlacklistResponse{}, nil
				}
			},
		},
		{
			name:   "post_check_blacklist",
			method: http.MethodPost,
			target: "/api/v1/auth/blacklist/check",
			body:   `{"userUuid":"u1","targetUuid":"u2"}`,
			setup: func(s *fakeRouterBlacklistService, called *bool) {
				s.checkFn = func(_ context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u1", req.UserUUID)
					require.Equal(t, "u2", req.TargetUUID)
					return &dto.CheckIsBlacklistResponse{IsBlacklist: true}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &fakeRouterBlacklistService{}
			tt.setup(svc, &called)
			r := buildBlacklistTestRouter(svc)

			req := newAuthedJSONRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeSuccess, decodeRouterResultCode(t, w))
			assert.True(t, called)
		})
	}
}

func TestRouterBlacklistParamErrors(t *testing.T) {
	initRouterBlacklistTestLogger()

	tests := []struct {
		name           string
		method         string
		target         string
		body           string
		wantStatus     int
		wantCode       int
		skipCodeAssert bool
	}{
		{
			name:           "add_blacklist_invalid_json",
			method:         http.MethodPost,
			target:         "/api/v1/auth/blacklist",
			body:           "{",
			wantStatus:     http.StatusOK,
			wantCode:       consts.CodeParamError,
			skipCodeAssert: false,
		},
		{
			name:           "get_blacklist_invalid_query",
			method:         http.MethodGet,
			target:         "/api/v1/auth/blacklist?page=abc",
			body:           "",
			wantStatus:     http.StatusOK,
			wantCode:       consts.CodeParamError,
			skipCodeAssert: false,
		},
		{
			name:           "delete_blacklist_missing_path_param",
			method:         http.MethodDelete,
			target:         "/api/v1/auth/blacklist/",
			body:           "",
			wantStatus:     http.StatusNotFound,
			skipCodeAssert: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := buildBlacklistTestRouter(&fakeRouterBlacklistService{})
			req := newAuthedJSONRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if !tt.skipCodeAssert {
				assert.Equal(t, tt.wantCode, decodeRouterResultCode(t, w))
			}
		})
	}
}

func TestRouterBlacklistBusinessErrorMapping(t *testing.T) {
	initRouterBlacklistTestLogger()

	tests := []struct {
		name     string
		method   string
		target   string
		body     string
		bizCode  int
		setupSvc func(*fakeRouterBlacklistService, error)
	}{
		{
			name:    "add_blacklist_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/auth/blacklist",
			body:    `{"targetUuid":"u2"}`,
			bizCode: consts.CodeCannotBlacklistSelf,
			setupSvc: func(s *fakeRouterBlacklistService, bizErr error) {
				s.addFn = func(_ context.Context, _ *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "remove_blacklist_business_error",
			method:  http.MethodDelete,
			target:  "/api/v1/auth/blacklist/u2",
			body:    "",
			bizCode: consts.CodeNotInBlacklist,
			setupSvc: func(s *fakeRouterBlacklistService, bizErr error) {
				s.removeFn = func(_ context.Context, _ *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "get_blacklist_business_error",
			method:  http.MethodGet,
			target:  "/api/v1/auth/blacklist?page=1&pageSize=20",
			body:    "",
			bizCode: consts.CodeParamError,
			setupSvc: func(s *fakeRouterBlacklistService, bizErr error) {
				s.listFn = func(_ context.Context, _ *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "check_blacklist_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/auth/blacklist/check",
			body:    `{"userUuid":"u1","targetUuid":"u2"}`,
			bizCode: consts.CodeParamError,
			setupSvc: func(s *fakeRouterBlacklistService, bizErr error) {
				s.checkFn = func(_ context.Context, _ *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					return nil, bizErr
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bizErr := apperr.ToStatus(apperr.New(tt.bizCode))
			svc := &fakeRouterBlacklistService{}
			tt.setupSvc(svc, bizErr)
			r := buildBlacklistTestRouter(svc)

			req := newAuthedJSONRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tt.bizCode, decodeRouterResultCode(t, w))
		})
	}
}

func TestRouterBlacklistInternalErrorMapping(t *testing.T) {
	initRouterBlacklistTestLogger()

	tests := []struct {
		name     string
		method   string
		target   string
		body     string
		setupSvc func(*fakeRouterBlacklistService)
	}{
		{
			name:   "add_blacklist_internal_error",
			method: http.MethodPost,
			target: "/api/v1/auth/blacklist",
			body:   `{"targetUuid":"u2"}`,
			setupSvc: func(s *fakeRouterBlacklistService) {
				s.addFn = func(_ context.Context, _ *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "remove_blacklist_internal_error",
			method: http.MethodDelete,
			target: "/api/v1/auth/blacklist/u2",
			body:   "",
			setupSvc: func(s *fakeRouterBlacklistService) {
				s.removeFn = func(_ context.Context, _ *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "get_blacklist_internal_error",
			method: http.MethodGet,
			target: "/api/v1/auth/blacklist?page=1&pageSize=20",
			body:   "",
			setupSvc: func(s *fakeRouterBlacklistService) {
				s.listFn = func(_ context.Context, _ *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "check_blacklist_internal_error",
			method: http.MethodPost,
			target: "/api/v1/auth/blacklist/check",
			body:   `{"userUuid":"u1","targetUuid":"u2"}`,
			setupSvc: func(s *fakeRouterBlacklistService) {
				s.checkFn = func(_ context.Context, _ *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeRouterBlacklistService{}
			tt.setupSvc(svc)
			r := buildBlacklistTestRouter(svc)

			req := newAuthedJSONRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, consts.CodeInternalError, decodeRouterResultCode(t, w))
		})
	}
}
