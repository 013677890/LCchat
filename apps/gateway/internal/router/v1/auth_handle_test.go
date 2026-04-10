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
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAuthHTTPService struct {
	loginFn          func(context.Context, *dto.LoginRequest, string) (*dto.LoginResponse, error)
	registerFn       func(context.Context, *dto.RegisterRequest) (*dto.RegisterResponse, error)
	sendVerifyCodeFn func(context.Context, *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error)
	loginByCodeFn    func(context.Context, *dto.LoginByCodeRequest, string) (*dto.LoginByCodeResponse, error)
	logoutFn         func(context.Context, *dto.LogoutRequest) (*dto.LogoutResponse, error)
	resetPasswordFn  func(context.Context, *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error)
	refreshTokenFn   func(context.Context, *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error)
	verifyCodeFn     func(context.Context, *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error)
}

func (f *fakeAuthHTTPService) Login(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
	if f.loginFn == nil {
		return &dto.LoginResponse{}, nil
	}
	return f.loginFn(ctx, req, deviceID)
}

func (f *fakeAuthHTTPService) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
	if f.registerFn == nil {
		return &dto.RegisterResponse{}, nil
	}
	return f.registerFn(ctx, req)
}

func (f *fakeAuthHTTPService) SendVerifyCode(ctx context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
	if f.sendVerifyCodeFn == nil {
		return &dto.SendVerifyCodeResponse{}, nil
	}
	return f.sendVerifyCodeFn(ctx, req)
}

func (f *fakeAuthHTTPService) LoginByCode(ctx context.Context, req *dto.LoginByCodeRequest, deviceID string) (*dto.LoginByCodeResponse, error) {
	if f.loginByCodeFn == nil {
		return &dto.LoginByCodeResponse{}, nil
	}
	return f.loginByCodeFn(ctx, req, deviceID)
}

func (f *fakeAuthHTTPService) Logout(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	if f.logoutFn == nil {
		return &dto.LogoutResponse{}, nil
	}
	return f.logoutFn(ctx, req)
}

func (f *fakeAuthHTTPService) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
	if f.resetPasswordFn == nil {
		return &dto.ResetPasswordResponse{}, nil
	}
	return f.resetPasswordFn(ctx, req)
}

func (f *fakeAuthHTTPService) RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
	if f.refreshTokenFn == nil {
		return &dto.RefreshTokenResponse{}, nil
	}
	return f.refreshTokenFn(ctx, req)
}

func (f *fakeAuthHTTPService) VerifyCode(ctx context.Context, req *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
	if f.verifyCodeFn == nil {
		return &dto.VerifyCodeResponse{}, nil
	}
	return f.verifyCodeFn(ctx, req)
}

type authHandlerResultBody struct {
	Code int `json:"code"`
}

var gatewayAuthHandlerLoggerOnce sync.Once

func initGatewayAuthHandlerLogger() {
	gatewayAuthHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}

func decodeAuthHandlerCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body authHandlerResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func newJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestAuthHandlerLogin(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		headerID   string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			name:       "missing_device_id_and_header",
			body:       `{"account":"a","password":"pass123"}`,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "blank_header_device_id",
			body:       `{"account":"a","password":"pass123"}`,
			headerID:   "   ",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:     "success_with_header_device_id",
			body:     `{"account":"a","password":"pass123"}`,
			headerID: "d1",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginFn = func(_ context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
					*called = true
					require.Equal(t, "a", req.Account)
					require.Equal(t, "pass123", req.Password)
					require.Equal(t, "d1", deviceID)
					return &dto.LoginResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:     "success_with_trimmed_header_device_id",
			body:     `{"account":"a","password":"pass123","deviceInfo":{"deviceName":"ios","platform":"ios"}}`,
			headerID: "  d1  ",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginFn = func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
					*called = true
					require.Equal(t, "a", req.Account)
					require.Equal(t, "pass123", req.Password)
					require.Equal(t, "d1", deviceID)
					require.Equal(t, "d1", ctx.Value("device_id"))
					return &dto.LoginResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:     "success_with_header_device_id_context_contains_device_id",
			body:     `{"account":"a","password":"pass123","deviceInfo":{"deviceName":"ios","platform":"ios"}}`,
			headerID: "d1",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginFn = func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
					*called = true
					require.Equal(t, "a", req.Account)
					require.Equal(t, "pass123", req.Password)
					require.Equal(t, "d1", deviceID)
					require.Equal(t, "d1", ctx.Value("device_id"))
					return &dto.LoginResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:     "business_error_passthrough",
			body:     `{"account":"a","password":"pass123"}`,
			headerID: "d1",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginFn = func(_ context.Context, _ *dto.LoginRequest, _ string) (*dto.LoginResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodePasswordError))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodePasswordError,
			wantCalled: true,
		},
		{
			name:     "internal_error",
			body:     `{"account":"a","password":"pass123"}`,
			headerID: "d1",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginFn = func(_ context.Context, _ *dto.LoginRequest, _ string) (*dto.LoginResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/login", tt.body)
			if tt.headerID != "" {
				req.Header.Set("X-Device-ID", tt.headerID)
			}
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.Login(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerLoginByCode(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		headerID   string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			name:       "missing_device_id_and_header",
			body:       `{"email":"a@test.com","verifyCode":"123456"}`,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:       "blank_header_device_id",
			body:       `{"email":"a@test.com","verifyCode":"123456"}`,
			headerID:   "   ",
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
		},
		{
			name:     "success_with_header_device_id",
			body:     `{"email":"a@test.com","verifyCode":"123456"}`,
			headerID: "d2",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginByCodeFn = func(_ context.Context, req *dto.LoginByCodeRequest, deviceID string) (*dto.LoginByCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, "123456", req.VerifyCode)
					require.Equal(t, "d2", deviceID)
					return &dto.LoginByCodeResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:     "success_with_trimmed_header_device_id",
			body:     `{"email":"a@test.com","verifyCode":"123456","deviceInfo":{"deviceName":"ios","platform":"ios"}}`,
			headerID: "  d2  ",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginByCodeFn = func(ctx context.Context, req *dto.LoginByCodeRequest, deviceID string) (*dto.LoginByCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, "123456", req.VerifyCode)
					require.Equal(t, "d2", deviceID)
					require.Equal(t, "d2", ctx.Value("device_id"))
					return &dto.LoginByCodeResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:     "success_with_header_device_id_context_contains_device_id",
			body:     `{"email":"a@test.com","verifyCode":"123456","deviceInfo":{"deviceName":"ios","platform":"ios"}}`,
			headerID: "d2",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginByCodeFn = func(ctx context.Context, req *dto.LoginByCodeRequest, deviceID string) (*dto.LoginByCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, "123456", req.VerifyCode)
					require.Equal(t, "d2", deviceID)
					require.Equal(t, "d2", ctx.Value("device_id"))
					return &dto.LoginByCodeResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:     "business_error_passthrough",
			body:     `{"email":"a@test.com","verifyCode":"123456"}`,
			headerID: "d2",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginByCodeFn = func(_ context.Context, _ *dto.LoginByCodeRequest, _ string) (*dto.LoginByCodeResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeVerifyCodeError))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeVerifyCodeError,
			wantCalled: true,
		},
		{
			name:     "internal_error",
			body:     `{"email":"a@test.com","verifyCode":"123456"}`,
			headerID: "d2",
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.loginByCodeFn = func(_ context.Context, _ *dto.LoginByCodeRequest, _ string) (*dto.LoginByCodeResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/login-by-code", tt.body)
			if tt.headerID != "" {
				req.Header.Set("X-Device-ID", tt.headerID)
			}
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.LoginByCode(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerRegister(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			body: `{"email":"a@test.com","password":"pass123","verifyCode":"123456","nickname":"n1","telephone":"13800138000"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.registerFn = func(_ context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					return &dto.RegisterResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"email":"a@test.com","password":"pass123","verifyCode":"123456","nickname":"n1","telephone":"13800138000"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.registerFn = func(_ context.Context, _ *dto.RegisterRequest) (*dto.RegisterResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeUserAlreadyExist))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeUserAlreadyExist,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"email":"a@test.com","password":"pass123","verifyCode":"123456","nickname":"n1","telephone":"13800138000"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.registerFn = func(_ context.Context, _ *dto.RegisterRequest) (*dto.RegisterResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/register", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.Register(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerSendVerifyCode(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			body: `{"email":"a@test.com","type":2}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.sendVerifyCodeFn = func(_ context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, int32(2), req.Type)
					return &dto.SendVerifyCodeResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"email":"a@test.com","type":2}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.sendVerifyCodeFn = func(_ context.Context, _ *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeSendTooFrequent))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSendTooFrequent,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"email":"a@test.com","type":2}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.sendVerifyCodeFn = func(_ context.Context, _ *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/send-verify-code", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.SendVerifyCode(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerVerifyCode(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			body: `{"email":"a@test.com","verifyCode":"123456","type":2}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.verifyCodeFn = func(_ context.Context, req *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, "123456", req.VerifyCode)
					return &dto.VerifyCodeResponse{Valid: true}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"email":"a@test.com","verifyCode":"123456","type":2}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.verifyCodeFn = func(_ context.Context, _ *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeVerifyCodeError))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeVerifyCodeError,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"email":"a@test.com","verifyCode":"123456","type":2}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.verifyCodeFn = func(_ context.Context, _ *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/verify-code", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.VerifyCode(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerResetPassword(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			body: `{"email":"a@test.com","verifyCode":"123456","newPassword":"pass999"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.resetPasswordFn = func(_ context.Context, req *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.Email)
					require.Equal(t, "123456", req.VerifyCode)
					return nil, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"email":"a@test.com","verifyCode":"123456","newPassword":"pass999"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.resetPasswordFn = func(_ context.Context, _ *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeVerifyCodeError))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeVerifyCodeError,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"email":"a@test.com","verifyCode":"123456","newPassword":"pass999"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.resetPasswordFn = func(_ context.Context, _ *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/reset-password", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.ResetPassword(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerRefreshToken(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			name: "success_and_context_write",
			body: `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.refreshTokenFn = func(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
					*called = true
					require.Equal(t, "u1", req.UserUUID)
					require.Equal(t, "d1", req.DeviceID)
					require.Equal(t, "u1", ctx.Value("user_uuid"))
					require.Equal(t, "d1", ctx.Value("device_id"))
					return &dto.RefreshTokenResponse{AccessToken: "atk"}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.refreshTokenFn = func(_ context.Context, _ *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeInvalidToken))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeInvalidToken,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"uuid":"u1","device_id":"d1","refreshToken":"rtk"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.refreshTokenFn = func(_ context.Context, _ *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/public/user/refresh-token", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.RefreshToken(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestAuthHandlerLogout(t *testing.T) {
	initGatewayAuthHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeAuthHTTPService, *bool)
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
			body: `{"deviceId":"d1"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.logoutFn = func(_ context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
					*called = true
					require.Equal(t, "d1", req.DeviceID)
					return nil, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			body: `{"deviceId":"d1"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.logoutFn = func(_ context.Context, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeInvalidToken))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeInvalidToken,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"deviceId":"d1"}`,
			setupSvc: func(s *fakeAuthHTTPService, called *bool) {
				s.logoutFn = func(_ context.Context, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
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
			svc := &fakeAuthHTTPService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewAuthHandler(svc)

			w := httptest.NewRecorder()
			req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/user/logout", tt.body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.Logout(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeAuthHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}
