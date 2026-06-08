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
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBlacklistHTTPService struct {
	addFn    func(context.Context, *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error)
	removeFn func(context.Context, *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error)
	listFn   func(context.Context, *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error)
	checkFn  func(context.Context, *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error)
}

func (f *fakeBlacklistHTTPService) AddBlacklist(ctx context.Context, req *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
	if f.addFn == nil {
		return &dto.AddBlacklistResponse{}, nil
	}
	return f.addFn(ctx, req)
}

func (f *fakeBlacklistHTTPService) RemoveBlacklist(ctx context.Context, req *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
	if f.removeFn == nil {
		return &dto.RemoveBlacklistResponse{}, nil
	}
	return f.removeFn(ctx, req)
}

func (f *fakeBlacklistHTTPService) GetBlacklistList(ctx context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
	if f.listFn == nil {
		return &dto.GetBlacklistListResponse{}, nil
	}
	return f.listFn(ctx, req)
}

func (f *fakeBlacklistHTTPService) CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
	if f.checkFn == nil {
		return &dto.CheckIsBlacklistResponse{}, nil
	}
	return f.checkFn(ctx, req)
}

type gatewayResultBody struct {
	Code int `json:"code"`
}

var gatewayBlacklistHandlerLoggerOnce sync.Once

func initGatewayBlacklistHandlerLogger() {
	gatewayBlacklistHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}

func decodeGatewayResultCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body gatewayResultBody
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	return body.Code
}

func TestBlacklistHandlerAddBlacklist(t *testing.T) {
	initGatewayBlacklistHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeBlacklistHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name:       "bind_json_failed",
			body:       "{",
			setupSvc:   nil,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
			wantCalled: false,
		},
		{
			name: "success",
			body: `{"targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.addFn = func(_ context.Context, req *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u2", req.TargetUUID)
					return &dto.AddBlacklistResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error_passthrough",
			body: `{"targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.addFn = func(_ context.Context, _ *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeCannotBlacklistSelf))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeCannotBlacklistSelf,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.addFn = func(_ context.Context, _ *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
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
			svc := &fakeBlacklistHTTPService{
				addFn: func(_ context.Context, _ *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {
					called = true
					return &dto.AddBlacklistResponse{}, nil
				},
			}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}

			h := NewBlacklistHandler(svc)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/blacklist", bytes.NewBufferString(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.AddBlacklist(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeGatewayResultCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestBlacklistHandlerRemoveBlacklist(t *testing.T) {
	initGatewayBlacklistHandlerLogger()

	tests := []struct {
		name       string
		pathValue  string
		setPath    bool
		setupSvc   func(*fakeBlacklistHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name:       "missing_path_param",
			setPath:    false,
			setupSvc:   nil,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
			wantCalled: false,
		},
		{
			name:      "success",
			pathValue: "u2",
			setPath:   true,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.removeFn = func(_ context.Context, req *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u2", req.UserUUID)
					return &dto.RemoveBlacklistResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:      "business_error_passthrough",
			pathValue: "u2",
			setPath:   true,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.removeFn = func(_ context.Context, _ *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeNotInBlacklist))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeNotInBlacklist,
			wantCalled: true,
		},
		{
			name:      "internal_error",
			pathValue: "u2",
			setPath:   true,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.removeFn = func(_ context.Context, _ *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
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
			svc := &fakeBlacklistHTTPService{
				removeFn: func(_ context.Context, _ *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {
					called = true
					return &dto.RemoveBlacklistResponse{}, nil
				},
			}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}
			h := NewBlacklistHandler(svc)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/api/v1/auth/blacklist/"+tt.pathValue, nil)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			if tt.setPath {
				c.Params = gin.Params{{Key: "userUuid", Value: tt.pathValue}}
			}

			h.RemoveBlacklist(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeGatewayResultCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestBlacklistHandlerGetBlacklistList(t *testing.T) {
	initGatewayBlacklistHandlerLogger()

	tests := []struct {
		name       string
		targetURL  string
		setupSvc   func(*fakeBlacklistHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name:       "bind_query_failed",
			targetURL:  "/api/v1/auth/blacklist?page=abc&pageSize=20",
			setupSvc:   nil,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
			wantCalled: false,
		},
		{
			name:      "missing_page_and_page_size",
			targetURL: "/api/v1/auth/blacklist",
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.listFn = func(_ context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(20), req.PageSize)
					return &dto.GetBlacklistListResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:      "success_with_valid_query",
			targetURL: "/api/v1/auth/blacklist?page=1&pageSize=20",
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.listFn = func(_ context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					*called = true
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(20), req.PageSize)
					return &dto.GetBlacklistListResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name:      "business_error_passthrough",
			targetURL: "/api/v1/auth/blacklist?page=1&pageSize=20",
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.listFn = func(_ context.Context, _ *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeParamError))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
			wantCalled: true,
		},
		{
			name:      "internal_error",
			targetURL: "/api/v1/auth/blacklist?page=1&pageSize=20",
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.listFn = func(_ context.Context, _ *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
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
			svc := &fakeBlacklistHTTPService{
				listFn: func(_ context.Context, _ *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
					called = true
					return &dto.GetBlacklistListResponse{}, nil
				},
			}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}

			h := NewBlacklistHandler(svc)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, tt.targetURL, nil)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.GetBlacklistList(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeGatewayResultCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestBlacklistHandlerCheckIsBlacklist(t *testing.T) {
	initGatewayBlacklistHandlerLogger()

	tests := []struct {
		name       string
		body       string
		setupSvc   func(*fakeBlacklistHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name:       "bind_json_failed",
			body:       "{",
			setupSvc:   nil,
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
			wantCalled: false,
		},
		{
			name: "success",
			body: `{"userUuid":"u1","targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.checkFn = func(_ context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u1", req.UserUUID)
					require.Equal(t, "u2", req.TargetUUID)
					return &dto.CheckIsBlacklistResponse{IsBlacklist: true}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			// 安全回归：body 里伪造的 userUuid 必须被 JWT 身份覆盖，禁止探测他人关系。
			name: "spoofed_user_uuid_overridden_by_jwt",
			body: `{"userUuid":"attacker","targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.checkFn = func(_ context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					*called = true
					require.Equal(t, "u1", req.UserUUID) // 被 ctx 中的登录用户 u1 覆盖，而非 body 的 "attacker"
					require.Equal(t, "u2", req.TargetUUID)
					return &dto.CheckIsBlacklistResponse{IsBlacklist: false}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error_passthrough",
			body: `{"userUuid":"u1","targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.checkFn = func(_ context.Context, _ *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeParamError))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeParamError,
			wantCalled: true,
		},
		{
			name: "internal_error",
			body: `{"userUuid":"u1","targetUuid":"u2"}`,
			setupSvc: func(svc *fakeBlacklistHTTPService, called *bool) {
				svc.checkFn = func(_ context.Context, _ *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
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
			svc := &fakeBlacklistHTTPService{
				checkFn: func(_ context.Context, _ *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
					called = true
					return &dto.CheckIsBlacklistResponse{IsBlacklist: false}, nil
				},
			}
			if tt.setupSvc != nil {
				tt.setupSvc(svc, &called)
			}

			h := NewBlacklistHandler(svc)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/blacklist/check", bytes.NewBufferString(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			// 模拟鉴权中间件已注入当前登录用户身份；CheckIsBlacklist 会以该身份覆盖 body 的 userUuid。
			ctxmeta.SetUserUUID(c, "u1")

			h.CheckIsBlacklist(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeGatewayResultCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}
