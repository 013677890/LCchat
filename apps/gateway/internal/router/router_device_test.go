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

type fakeRouterDeviceService struct {
	getDeviceListFn        func(context.Context) (*dto.GetDeviceListResponse, error)
	kickDeviceFn           func(context.Context, *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error)
	getOnlineStatusFn      func(context.Context, *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error)
	batchGetOnlineStatusFn func(context.Context, *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error)
}

var _ service.DeviceService = (*fakeRouterDeviceService)(nil)

func (f *fakeRouterDeviceService) GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error) {
	if f.getDeviceListFn == nil {
		return &dto.GetDeviceListResponse{}, nil
	}
	return f.getDeviceListFn(ctx)
}

func (f *fakeRouterDeviceService) KickDevice(ctx context.Context, req *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
	if f.kickDeviceFn == nil {
		return &dto.KickDeviceResponse{}, nil
	}
	return f.kickDeviceFn(ctx, req)
}

func (f *fakeRouterDeviceService) GetOnlineStatus(ctx context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
	if f.getOnlineStatusFn == nil {
		return &dto.GetOnlineStatusResponse{}, nil
	}
	return f.getOnlineStatusFn(ctx, req)
}

func (f *fakeRouterDeviceService) BatchGetOnlineStatus(ctx context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
	if f.batchGetOnlineStatusFn == nil {
		return &dto.BatchGetOnlineStatusResponse{}, nil
	}
	return f.batchGetOnlineStatusFn(ctx, req)
}

type routerDeviceResultBody struct {
	Code int `json:"code"`
}

var routerDeviceLoggerOnce sync.Once

func initRouterDeviceTestLogger() {
	routerDeviceLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
		gin.SetMode(gin.TestMode)
	})
}

func decodeRouterDeviceCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body routerDeviceResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func mustDeviceAuthToken(t *testing.T) string {
	t.Helper()
	token, err := util.GenerateToken("u1", "d1")
	require.NoError(t, err)
	return token
}

func newRouterDeviceRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newAuthedRouterDeviceRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req := newRouterDeviceRequest(t, method, target, body)
	req.Header.Set("Authorization", "Bearer "+mustDeviceAuthToken(t))
	return req
}

func buildDeviceTestRouter(deviceSvc service.DeviceService) *gin.Engine {
	authHandler := v1.NewAuthHandler(nil)
	userHandler := v1.NewUserHandler(nil)
	friendHandler := v1.NewFriendHandler(nil)
	blacklistHandler := v1.NewBlacklistHandler(nil)
	deviceHandler := v1.NewDeviceHandler(deviceSvc)
	return InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, nil)
}

func TestRouterDeviceUnauthorized(t *testing.T) {
	initRouterDeviceTestLogger()

	r := buildDeviceTestRouter(&fakeRouterDeviceService{})
	req := newRouterDeviceRequest(t, http.MethodGet, "/api/v1/auth/user/devices", "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, consts.CodeUnauthorized, decodeRouterDeviceCode(t, w))
}

func TestRouterDeviceRoutesAndSuccess(t *testing.T) {
	initRouterDeviceTestLogger()

	tests := []struct {
		name   string
		method string
		target string
		body   string
		setup  func(*fakeRouterDeviceService, *bool)
	}{
		{
			name:   "get_devices",
			method: http.MethodGet,
			target: "/api/v1/auth/user/devices",
			setup: func(s *fakeRouterDeviceService, called *bool) {
				s.getDeviceListFn = func(_ context.Context) (*dto.GetDeviceListResponse, error) {
					*called = true
					return &dto.GetDeviceListResponse{}, nil
				}
			},
		},
		{
			name:   "delete_device",
			method: http.MethodDelete,
			target: "/api/v1/auth/user/devices/d2",
			setup: func(s *fakeRouterDeviceService, called *bool) {
				s.kickDeviceFn = func(_ context.Context, req *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
					*called = true
					require.Equal(t, "d2", req.DeviceID)
					return &dto.KickDeviceResponse{}, nil
				}
			},
		},
		{
			name:   "get_online_status",
			method: http.MethodGet,
			target: "/api/v1/auth/user/online-status/u2",
			setup: func(s *fakeRouterDeviceService, called *bool) {
				s.getOnlineStatusFn = func(_ context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
					*called = true
					require.Equal(t, "u2", req.UserUUID)
					return &dto.GetOnlineStatusResponse{}, nil
				}
			},
		},
		{
			name:   "batch_get_online_status",
			method: http.MethodPost,
			target: "/api/v1/auth/user/batch-online-status",
			body:   `{"userUuids":["u1","u2"]}`,
			setup: func(s *fakeRouterDeviceService, called *bool) {
				s.batchGetOnlineStatusFn = func(_ context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
					*called = true
					require.Equal(t, []string{"u1", "u2"}, req.UserUUIDs)
					return &dto.BatchGetOnlineStatusResponse{}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &fakeRouterDeviceService{}
			tt.setup(svc, &called)
			r := buildDeviceTestRouter(svc)

			req := newAuthedRouterDeviceRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeSuccess, decodeRouterDeviceCode(t, w))
			assert.True(t, called)
		})
	}
}

func TestRouterDeviceParamErrors(t *testing.T) {
	initRouterDeviceTestLogger()

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   int
	}{
		{
			name:       "batch_invalid_json",
			method:     http.MethodPost,
			target:     "/api/v1/auth/user/batch-online-status",
			body:       "{",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "batch_empty_users",
			method:     http.MethodPost,
			target:     "/api/v1/auth/user/batch-online-status",
			body:       `{"userUuids":[]}`,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "batch_too_many_users",
			method:     http.MethodPost,
			target:     "/api/v1/auth/user/batch-online-status",
			body:       `{"userUuids":["u1","u2","u3","u4","u5","u6","u7","u8","u9","u10","u11","u12","u13","u14","u15","u16","u17","u18","u19","u20","u21","u22","u23","u24","u25","u26","u27","u28","u29","u30","u31","u32","u33","u34","u35","u36","u37","u38","u39","u40","u41","u42","u43","u44","u45","u46","u47","u48","u49","u50","u51","u52","u53","u54","u55","u56","u57","u58","u59","u60","u61","u62","u63","u64","u65","u66","u67","u68","u69","u70","u71","u72","u73","u74","u75","u76","u77","u78","u79","u80","u81","u82","u83","u84","u85","u86","u87","u88","u89","u90","u91","u92","u93","u94","u95","u96","u97","u98","u99","u100","u101"]}`,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := buildDeviceTestRouter(&fakeRouterDeviceService{})
			req := newAuthedRouterDeviceRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeRouterDeviceCode(t, w))
		})
	}
}

func TestRouterDeviceErrorMapping(t *testing.T) {
	initRouterDeviceTestLogger()

	t.Run("business_error_passthrough", func(t *testing.T) {
		svc := &fakeRouterDeviceService{
			getDeviceListFn: func(_ context.Context) (*dto.GetDeviceListResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeUnauthorized))
			},
			kickDeviceFn: func(_ context.Context, _ *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeDeviceNotFound))
			},
			getOnlineStatusFn: func(_ context.Context, _ *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeDeviceNotFound))
			},
			batchGetOnlineStatusFn: func(_ context.Context, _ *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeParamError))
			},
		}
		r := buildDeviceTestRouter(svc)

		cases := []struct {
			method string
			target string
			body   string
			code   int
		}{
			{method: http.MethodGet, target: "/api/v1/auth/user/devices", code: consts.CodeUnauthorized},
			{method: http.MethodDelete, target: "/api/v1/auth/user/devices/d2", code: consts.CodeDeviceNotFound},
			{method: http.MethodGet, target: "/api/v1/auth/user/online-status/u2", code: consts.CodeDeviceNotFound},
			{method: http.MethodPost, target: "/api/v1/auth/user/batch-online-status", body: `{"userUuids":["u1"]}`, code: consts.CodeParamError},
		}

		for _, c := range cases {
			w := httptest.NewRecorder()
			req := newAuthedRouterDeviceRequest(t, c.method, c.target, c.body)
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, c.code, decodeRouterDeviceCode(t, w))
		}
	})

	t.Run("internal_error_to_internal_code", func(t *testing.T) {
		svc := &fakeRouterDeviceService{
			getDeviceListFn: func(_ context.Context) (*dto.GetDeviceListResponse, error) {
				return nil, errors.New("internal")
			},
			kickDeviceFn: func(_ context.Context, _ *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
				return nil, errors.New("internal")
			},
			getOnlineStatusFn: func(_ context.Context, _ *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
				return nil, errors.New("internal")
			},
			batchGetOnlineStatusFn: func(_ context.Context, _ *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
				return nil, errors.New("internal")
			},
		}
		r := buildDeviceTestRouter(svc)

		cases := []struct {
			method string
			target string
			body   string
		}{
			{method: http.MethodGet, target: "/api/v1/auth/user/devices"},
			{method: http.MethodDelete, target: "/api/v1/auth/user/devices/d2"},
			{method: http.MethodGet, target: "/api/v1/auth/user/online-status/u2"},
			{method: http.MethodPost, target: "/api/v1/auth/user/batch-online-status", body: `{"userUuids":["u1"]}`},
		}

		for _, c := range cases {
			w := httptest.NewRecorder()
			req := newAuthedRouterDeviceRequest(t, c.method, c.target, c.body)
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, consts.CodeInternalError, decodeRouterDeviceCode(t, w))
		}
	})
}
