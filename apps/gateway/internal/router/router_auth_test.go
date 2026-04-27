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

type fakeRouterAuthService struct {
	loginFn          func(context.Context, *dto.LoginRequest, string) (*dto.LoginResponse, error)
	registerFn       func(context.Context, *dto.RegisterRequest) (*dto.RegisterResponse, error)
	sendVerifyCodeFn func(context.Context, *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error)
	loginByCodeFn    func(context.Context, *dto.LoginByCodeRequest, string) (*dto.LoginByCodeResponse, error)
	logoutFn         func(context.Context, *dto.LogoutRequest) (*dto.LogoutResponse, error)
	resetPasswordFn  func(context.Context, *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error)
	refreshTokenFn   func(context.Context, *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error)
	verifyCodeFn     func(context.Context, *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error)
}

var _ service.AuthService = (*fakeRouterAuthService)(nil)

func (f *fakeRouterAuthService) Login(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
	if f.loginFn == nil {
		return &dto.LoginResponse{}, nil
	}
	return f.loginFn(ctx, req, deviceID)
}

func (f *fakeRouterAuthService) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
	if f.registerFn == nil {
		return &dto.RegisterResponse{}, nil
	}
	return f.registerFn(ctx, req)
}

func (f *fakeRouterAuthService) SendVerifyCode(ctx context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
	if f.sendVerifyCodeFn == nil {
		return &dto.SendVerifyCodeResponse{}, nil
	}
	return f.sendVerifyCodeFn(ctx, req)
}

func (f *fakeRouterAuthService) LoginByCode(ctx context.Context, req *dto.LoginByCodeRequest, deviceID string) (*dto.LoginByCodeResponse, error) {
	if f.loginByCodeFn == nil {
		return &dto.LoginByCodeResponse{}, nil
	}
	return f.loginByCodeFn(ctx, req, deviceID)
}

func (f *fakeRouterAuthService) Logout(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	if f.logoutFn == nil {
		return &dto.LogoutResponse{}, nil
	}
	return f.logoutFn(ctx, req)
}

func (f *fakeRouterAuthService) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
	if f.resetPasswordFn == nil {
		return &dto.ResetPasswordResponse{}, nil
	}
	return f.resetPasswordFn(ctx, req)
}

func (f *fakeRouterAuthService) RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
	if f.refreshTokenFn == nil {
		return &dto.RefreshTokenResponse{}, nil
	}
	return f.refreshTokenFn(ctx, req)
}

func (f *fakeRouterAuthService) VerifyCode(ctx context.Context, req *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
	if f.verifyCodeFn == nil {
		return &dto.VerifyCodeResponse{}, nil
	}
	return f.verifyCodeFn(ctx, req)
}

type routerAuthResultBody struct {
	Code int `json:"code"`
}

var routerAuthLoggerOnce sync.Once

func initRouterAuthTestLogger() {
	routerAuthLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
		gin.SetMode(gin.TestMode)
	})
}

func decodeRouterAuthCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body routerAuthResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func mustAuthTokenForAuth(t *testing.T) string {
	t.Helper()
	token, err := util.GenerateToken("u1", "d1")
	require.NoError(t, err)
	return token
}

func newRouterJSONRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newAuthedRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req := newRouterJSONRequest(t, method, target, body)
	req.Header.Set("Authorization", "Bearer "+mustAuthTokenForAuth(t))
	return req
}

func buildAuthTestRouter(authSvc service.AuthService) *gin.Engine {
	authHandler := v1.NewAuthHandler(authSvc)
	userHandler := v1.NewUserHandler(nil)
	friendHandler := v1.NewFriendHandler(nil)
	blacklistHandler := v1.NewBlacklistHandler(nil)
	deviceHandler := v1.NewDeviceHandler(nil)
	return InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, nil)
}

func TestRouterAuthPublicRoutesSuccess(t *testing.T) {
	initRouterAuthTestLogger()

	tests := []struct {
		name             string
		method           string
		target           string
		body             string
		needDeviceHeader bool
		setup            func(*fakeRouterAuthService, *bool)
	}{
		{
			name:             "login",
			method:           http.MethodPost,
			target:           "/api/v1/public/user/login",
			body:             `{"account":"a","password":"pass123"}`,
			needDeviceHeader: true,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.loginFn = func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
					*called = true
					require.Equal(t, "a", req.Account)
					require.Equal(t, "d1", deviceID)
					require.Equal(t, "d1", util.GetDeviceIDFromContext(ctx))
					require.Equal(t, "", util.GetUserUUIDFromContext(ctx))
					return &dto.LoginResponse{}, nil
				}
			},
		},
		{
			name:             "login_by_code",
			method:           http.MethodPost,
			target:           "/api/v1/public/user/login-by-code",
			body:             `{"email":"a@test.com","verifyCode":"123456"}`,
			needDeviceHeader: true,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.loginByCodeFn = func(ctx context.Context, req *dto.LoginByCodeRequest, deviceID string) (*dto.LoginByCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, "d1", deviceID)
					require.Equal(t, "d1", util.GetDeviceIDFromContext(ctx))
					require.Equal(t, "", util.GetUserUUIDFromContext(ctx))
					return &dto.LoginByCodeResponse{}, nil
				}
			},
		},
		{
			name:   "register",
			method: http.MethodPost,
			target: "/api/v1/public/user/register",
			body:   `{"email":"a@test.com","password":"pass123","verifyCode":"123456","nickname":"n1","telephone":"13800138000"}`,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.registerFn = func(_ context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					return &dto.RegisterResponse{}, nil
				}
			},
		},
		{
			name:   "send_verify_code",
			method: http.MethodPost,
			target: "/api/v1/public/user/send-verify-code",
			body:   `{"email":"a@test.com","type":2}`,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.sendVerifyCodeFn = func(_ context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
					*called = true
					require.Equal(t, int32(2), req.Type)
					return &dto.SendVerifyCodeResponse{}, nil
				}
			},
		},
		{
			name:   "reset_password",
			method: http.MethodPost,
			target: "/api/v1/public/user/reset-password",
			body:   `{"email":"a@test.com","verifyCode":"123456","newPassword":"pass999"}`,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.resetPasswordFn = func(_ context.Context, req *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					return nil, nil
				}
			},
		},
		{
			name:   "refresh_token",
			method: http.MethodPost,
			target: "/api/v1/public/user/refresh-token",
			body:   `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.refreshTokenFn = func(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
					*called = true
					require.Equal(t, "u1", req.UserUUID)
					require.Equal(t, "u1", util.GetUserUUIDFromContext(ctx))
					require.Equal(t, "d1", util.GetDeviceIDFromContext(ctx))
					return &dto.RefreshTokenResponse{}, nil
				}
			},
		},
		{
			name:   "verify_code",
			method: http.MethodPost,
			target: "/api/v1/public/user/verify-code",
			body:   `{"email":"a@test.com","verifyCode":"123456","type":2}`,
			setup: func(s *fakeRouterAuthService, called *bool) {
				s.verifyCodeFn = func(_ context.Context, req *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					return &dto.VerifyCodeResponse{}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &fakeRouterAuthService{}
			tt.setup(svc, &called)
			r := buildAuthTestRouter(svc)

			req := newRouterJSONRequest(t, tt.method, tt.target, tt.body)
			if tt.needDeviceHeader {
				req.Header.Set("X-Device-ID", "d1")
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeSuccess, decodeRouterAuthCode(t, w))
			assert.True(t, called)
		})
	}
}

func TestRouterAuthLogoutUnauthorized(t *testing.T) {
	initRouterAuthTestLogger()
	r := buildAuthTestRouter(&fakeRouterAuthService{})

	req := newRouterJSONRequest(t, http.MethodPost, "/api/v1/auth/user/logout", `{"deviceId":"d1"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRouterAuthLogoutAuthorizedSuccess(t *testing.T) {
	initRouterAuthTestLogger()

	called := false
	r := buildAuthTestRouter(&fakeRouterAuthService{
		logoutFn: func(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
			called = true
			require.Equal(t, "d1", req.DeviceID)
			require.Equal(t, "u1", util.GetUserUUIDFromContext(ctx))
			require.Equal(t, "d1", util.GetDeviceIDFromContext(ctx))
			return nil, nil
		},
	})

	req := newAuthedRequest(t, http.MethodPost, "/api/v1/auth/user/logout", `{"deviceId":"d1"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, decodeRouterAuthCode(t, w))
	assert.True(t, called)
}

func TestRouterAuthParamErrors(t *testing.T) {
	initRouterAuthTestLogger()

	tests := []struct {
		name       string
		reqBuilder func(*testing.T) *http.Request
		wantStatus int
		wantCode   int
	}{
		{
			name: "login_missing_device_header",
			reqBuilder: func(t *testing.T) *http.Request {
				return newRouterJSONRequest(t, http.MethodPost, "/api/v1/public/user/login", `{"account":"a","password":"pass123"}`)
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name: "login_invalid_json",
			reqBuilder: func(t *testing.T) *http.Request {
				req := newRouterJSONRequest(t, http.MethodPost, "/api/v1/public/user/login", `{`)
				req.Header.Set("X-Device-ID", "d1")
				return req
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name: "login_by_code_missing_device_header",
			reqBuilder: func(t *testing.T) *http.Request {
				return newRouterJSONRequest(t, http.MethodPost, "/api/v1/public/user/login-by-code", `{"email":"a@test.com","verifyCode":"123456"}`)
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name: "refresh_token_invalid_json",
			reqBuilder: func(t *testing.T) *http.Request {
				return newRouterJSONRequest(t, http.MethodPost, "/api/v1/public/user/refresh-token", `{`)
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name: "refresh_token_device_header_mismatch",
			reqBuilder: func(t *testing.T) *http.Request {
				req := newRouterJSONRequest(t, http.MethodPost, "/api/v1/public/user/refresh-token", `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`)
				req.Header.Set("X-Device-ID", "d2")
				return req
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name: "logout_invalid_json",
			reqBuilder: func(t *testing.T) *http.Request {
				return newAuthedRequest(t, http.MethodPost, "/api/v1/auth/user/logout", `{`)
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := buildAuthTestRouter(&fakeRouterAuthService{})
			req := tt.reqBuilder(t)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeRouterAuthCode(t, w))
		})
	}
}

func TestRouterAuthBusinessErrorMapping(t *testing.T) {
	initRouterAuthTestLogger()

	tests := []struct {
		name             string
		method           string
		target           string
		body             string
		needDeviceHeader bool
		needAuth         bool
		bizCode          int
		setupSvc         func(*fakeRouterAuthService, error)
	}{
		{
			name:             "login_business_error",
			method:           http.MethodPost,
			target:           "/api/v1/public/user/login",
			body:             `{"account":"a","password":"pass123"}`,
			needDeviceHeader: true,
			bizCode:          consts.CodePasswordError,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.loginFn = func(_ context.Context, _ *dto.LoginRequest, _ string) (*dto.LoginResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:             "login_by_code_business_error",
			method:           http.MethodPost,
			target:           "/api/v1/public/user/login-by-code",
			body:             `{"email":"a@test.com","verifyCode":"123456"}`,
			needDeviceHeader: true,
			bizCode:          consts.CodeVerifyCodeError,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.loginByCodeFn = func(_ context.Context, _ *dto.LoginByCodeRequest, _ string) (*dto.LoginByCodeResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "register_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/public/user/register",
			body:    `{"email":"a@test.com","password":"pass123","verifyCode":"123456","nickname":"n1","telephone":"13800138000"}`,
			bizCode: consts.CodeUserAlreadyExist,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.registerFn = func(_ context.Context, _ *dto.RegisterRequest) (*dto.RegisterResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "send_verify_code_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/public/user/send-verify-code",
			body:    `{"email":"a@test.com","type":2}`,
			bizCode: consts.CodeSendTooFrequent,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.sendVerifyCodeFn = func(_ context.Context, _ *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "reset_password_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/public/user/reset-password",
			body:    `{"email":"a@test.com","verifyCode":"123456","newPassword":"pass999"}`,
			bizCode: consts.CodeVerifyCodeError,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.resetPasswordFn = func(_ context.Context, _ *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "refresh_token_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/public/user/refresh-token",
			body:    `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`,
			bizCode: consts.CodeInvalidToken,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.refreshTokenFn = func(_ context.Context, _ *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:    "verify_code_business_error",
			method:  http.MethodPost,
			target:  "/api/v1/public/user/verify-code",
			body:    `{"email":"a@test.com","verifyCode":"123456","type":2}`,
			bizCode: consts.CodeVerifyCodeError,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.verifyCodeFn = func(_ context.Context, _ *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
					return nil, bizErr
				}
			},
		},
		{
			name:     "logout_business_error",
			method:   http.MethodPost,
			target:   "/api/v1/auth/user/logout",
			body:     `{"deviceId":"d1"}`,
			needAuth: true,
			bizCode:  consts.CodeInvalidToken,
			setupSvc: func(s *fakeRouterAuthService, bizErr error) {
				s.logoutFn = func(_ context.Context, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
					return nil, bizErr
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bizErr := apperr.ToStatus(apperr.New(tt.bizCode))
			svc := &fakeRouterAuthService{}
			tt.setupSvc(svc, bizErr)
			r := buildAuthTestRouter(svc)

			req := newRouterJSONRequest(t, tt.method, tt.target, tt.body)
			if tt.needDeviceHeader {
				req.Header.Set("X-Device-ID", "d1")
			}
			if tt.needAuth {
				req.Header.Set("Authorization", "Bearer "+mustAuthToken(t))
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tt.bizCode, decodeRouterAuthCode(t, w))
		})
	}
}

func TestRouterAuthInternalErrorMapping(t *testing.T) {
	initRouterAuthTestLogger()

	tests := []struct {
		name             string
		method           string
		target           string
		body             string
		needDeviceHeader bool
		needAuth         bool
		setupSvc         func(*fakeRouterAuthService)
	}{
		{
			name:             "login_internal_error",
			method:           http.MethodPost,
			target:           "/api/v1/public/user/login",
			body:             `{"account":"a","password":"pass123"}`,
			needDeviceHeader: true,
			setupSvc: func(s *fakeRouterAuthService) {
				s.loginFn = func(_ context.Context, _ *dto.LoginRequest, _ string) (*dto.LoginResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:             "login_by_code_internal_error",
			method:           http.MethodPost,
			target:           "/api/v1/public/user/login-by-code",
			body:             `{"email":"a@test.com","verifyCode":"123456"}`,
			needDeviceHeader: true,
			setupSvc: func(s *fakeRouterAuthService) {
				s.loginByCodeFn = func(_ context.Context, _ *dto.LoginByCodeRequest, _ string) (*dto.LoginByCodeResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "register_internal_error",
			method: http.MethodPost,
			target: "/api/v1/public/user/register",
			body:   `{"email":"a@test.com","password":"pass123","verifyCode":"123456","nickname":"n1","telephone":"13800138000"}`,
			setupSvc: func(s *fakeRouterAuthService) {
				s.registerFn = func(_ context.Context, _ *dto.RegisterRequest) (*dto.RegisterResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "send_verify_code_internal_error",
			method: http.MethodPost,
			target: "/api/v1/public/user/send-verify-code",
			body:   `{"email":"a@test.com","type":2}`,
			setupSvc: func(s *fakeRouterAuthService) {
				s.sendVerifyCodeFn = func(_ context.Context, _ *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "reset_password_internal_error",
			method: http.MethodPost,
			target: "/api/v1/public/user/reset-password",
			body:   `{"email":"a@test.com","verifyCode":"123456","newPassword":"pass999"}`,
			setupSvc: func(s *fakeRouterAuthService) {
				s.resetPasswordFn = func(_ context.Context, _ *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "refresh_token_internal_error",
			method: http.MethodPost,
			target: "/api/v1/public/user/refresh-token",
			body:   `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`,
			setupSvc: func(s *fakeRouterAuthService) {
				s.refreshTokenFn = func(_ context.Context, _ *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:   "verify_code_internal_error",
			method: http.MethodPost,
			target: "/api/v1/public/user/verify-code",
			body:   `{"email":"a@test.com","verifyCode":"123456","type":2}`,
			setupSvc: func(s *fakeRouterAuthService) {
				s.verifyCodeFn = func(_ context.Context, _ *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
		{
			name:     "logout_internal_error",
			method:   http.MethodPost,
			target:   "/api/v1/auth/user/logout",
			body:     `{"deviceId":"d1"}`,
			needAuth: true,
			setupSvc: func(s *fakeRouterAuthService) {
				s.logoutFn = func(_ context.Context, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
					return nil, errors.New("internal")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeRouterAuthService{}
			tt.setupSvc(svc)
			r := buildAuthTestRouter(svc)

			req := newRouterJSONRequest(t, tt.method, tt.target, tt.body)
			if tt.needDeviceHeader {
				req.Header.Set("X-Device-ID", "d1")
			}
			if tt.needAuth {
				req.Header.Set("Authorization", "Bearer "+mustAuthToken(t))
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, consts.CodeInternalError, decodeRouterAuthCode(t, w))
		})
	}
}
