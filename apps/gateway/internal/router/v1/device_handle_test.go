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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDeviceHTTPService struct {
	getDeviceListFn        func(context.Context) (*dto.GetDeviceListResponse, error)
	kickDeviceFn           func(context.Context, *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error)
	getOnlineStatusFn      func(context.Context, *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error)
	batchGetOnlineStatusFn func(context.Context, *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error)
}

var _ service.DeviceService = (*fakeDeviceHTTPService)(nil)

func (f *fakeDeviceHTTPService) GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error) {
	if f.getDeviceListFn == nil {
		return &dto.GetDeviceListResponse{}, nil
	}
	return f.getDeviceListFn(ctx)
}

func (f *fakeDeviceHTTPService) KickDevice(ctx context.Context, req *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
	if f.kickDeviceFn == nil {
		return &dto.KickDeviceResponse{}, nil
	}
	return f.kickDeviceFn(ctx, req)
}

func (f *fakeDeviceHTTPService) GetOnlineStatus(ctx context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
	if f.getOnlineStatusFn == nil {
		return &dto.GetOnlineStatusResponse{}, nil
	}
	return f.getOnlineStatusFn(ctx, req)
}

func (f *fakeDeviceHTTPService) BatchGetOnlineStatus(ctx context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
	if f.batchGetOnlineStatusFn == nil {
		return &dto.BatchGetOnlineStatusResponse{}, nil
	}
	return f.batchGetOnlineStatusFn(ctx, req)
}

type deviceHandlerResultBody struct {
	Code int `json:"code"`
}

var gatewayDeviceHandlerLoggerOnce sync.Once

func initGatewayDeviceHandlerLogger() {
	gatewayDeviceHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}

func decodeDeviceHandlerCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body deviceHandlerResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func newDeviceJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestDeviceHandlerGetDeviceList(t *testing.T) {
	initGatewayDeviceHandlerLogger()

	tests := []struct {
		name       string
		setupSvc   func(*fakeDeviceHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name: "success",
			setupSvc: func(s *fakeDeviceHTTPService, called *bool) {
				s.getDeviceListFn = func(_ context.Context) (*dto.GetDeviceListResponse, error) {
					*called = true
					return &dto.GetDeviceListResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			setupSvc: func(s *fakeDeviceHTTPService, called *bool) {
				s.getDeviceListFn = func(_ context.Context) (*dto.GetDeviceListResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeUnauthorized))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeUnauthorized,
			wantCalled: true,
		},
		{
			name: "internal_error",
			setupSvc: func(s *fakeDeviceHTTPService, called *bool) {
				s.getDeviceListFn = func(_ context.Context) (*dto.GetDeviceListResponse, error) {
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
			svc := &fakeDeviceHTTPService{}
			tt.setupSvc(svc, &called)
			h := NewDeviceHandler(svc)

			w := httptest.NewRecorder()
			req := newDeviceJSONRequest(t, http.MethodGet, "/api/v1/auth/user/devices", "")
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.GetDeviceList(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeDeviceHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestDeviceHandlerKickDevice(t *testing.T) {
	initGatewayDeviceHandlerLogger()

	t.Run("missing_path_param", func(t *testing.T) {
		called := false
		h := NewDeviceHandler(&fakeDeviceHTTPService{
			kickDeviceFn: func(_ context.Context, _ *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
				called = true
				return &dto.KickDeviceResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newDeviceJSONRequest(t, http.MethodDelete, "/api/v1/auth/user/devices/", "")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.KickDevice(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeDeviceHandlerCode(t, w))
		assert.False(t, called)
	})

	t.Run("success_and_error_mapping", func(t *testing.T) {
		tests := []struct {
			name       string
			setupFn    func(*fakeDeviceHTTPService)
			wantStatus int
			wantCode   int
		}{
			{
				name: "success",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.kickDeviceFn = func(_ context.Context, req *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
						require.Equal(t, "d1", req.DeviceID)
						return &dto.KickDeviceResponse{}, nil
					}
				},
				wantStatus: http.StatusOK,
				wantCode:   consts.CodeSuccess,
			},
			{
				name: "business_error",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.kickDeviceFn = func(_ context.Context, _ *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
						return nil, apperr.ToStatus(apperr.New(consts.CodeDeviceNotFound))
					}
				},
				wantStatus: http.StatusOK,
				wantCode:   consts.CodeDeviceNotFound,
			},
			{
				name: "internal_error",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.kickDeviceFn = func(_ context.Context, _ *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {
						return nil, errors.New("internal")
					}
				},
				wantStatus: http.StatusInternalServerError,
				wantCode:   consts.CodeInternalError,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := &fakeDeviceHTTPService{}
				tt.setupFn(svc)
				h := NewDeviceHandler(svc)

				w := httptest.NewRecorder()
				req := newDeviceJSONRequest(t, http.MethodDelete, "/api/v1/auth/user/devices/d1", "")
				c, _ := gin.CreateTestContext(w)
				c.Request = req
				c.Params = gin.Params{{Key: "deviceId", Value: "d1"}}

				h.KickDevice(c)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Equal(t, tt.wantCode, decodeDeviceHandlerCode(t, w))
			})
		}
	})
}

func TestDeviceHandlerGetOnlineStatus(t *testing.T) {
	initGatewayDeviceHandlerLogger()

	t.Run("missing_path_param", func(t *testing.T) {
		called := false
		h := NewDeviceHandler(&fakeDeviceHTTPService{
			getOnlineStatusFn: func(_ context.Context, _ *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
				called = true
				return &dto.GetOnlineStatusResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newDeviceJSONRequest(t, http.MethodGet, "/api/v1/auth/user/online-status/", "")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetOnlineStatus(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeDeviceHandlerCode(t, w))
		assert.False(t, called)
	})

	t.Run("success_and_error_mapping", func(t *testing.T) {
		tests := []struct {
			name       string
			setupFn    func(*fakeDeviceHTTPService)
			wantStatus int
			wantCode   int
		}{
			{
				name: "success",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.getOnlineStatusFn = func(_ context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
						require.Equal(t, "u2", req.UserUUID)
						return &dto.GetOnlineStatusResponse{UserUUID: "u2", IsOnline: true}, nil
					}
				},
				wantStatus: http.StatusOK,
				wantCode:   consts.CodeSuccess,
			},
			{
				name: "business_error",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.getOnlineStatusFn = func(_ context.Context, _ *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
						return nil, apperr.ToStatus(apperr.New(consts.CodeDeviceNotFound))
					}
				},
				wantStatus: http.StatusOK,
				wantCode:   consts.CodeDeviceNotFound,
			},
			{
				name: "internal_error",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.getOnlineStatusFn = func(_ context.Context, _ *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
						return nil, errors.New("internal")
					}
				},
				wantStatus: http.StatusInternalServerError,
				wantCode:   consts.CodeInternalError,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := &fakeDeviceHTTPService{}
				tt.setupFn(svc)
				h := NewDeviceHandler(svc)

				w := httptest.NewRecorder()
				req := newDeviceJSONRequest(t, http.MethodGet, "/api/v1/auth/user/online-status/u2", "")
				c, _ := gin.CreateTestContext(w)
				c.Request = req
				c.Params = gin.Params{{Key: "userUuid", Value: "u2"}}

				h.GetOnlineStatus(c)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Equal(t, tt.wantCode, decodeDeviceHandlerCode(t, w))
			})
		}
	})
}

func TestDeviceHandlerBatchGetOnlineStatus(t *testing.T) {
	initGatewayDeviceHandlerLogger()

	t.Run("bind_failed", func(t *testing.T) {
		called := false
		h := NewDeviceHandler(&fakeDeviceHTTPService{
			batchGetOnlineStatusFn: func(_ context.Context, _ *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
				called = true
				return &dto.BatchGetOnlineStatusResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newDeviceJSONRequest(t, http.MethodPost, "/api/v1/auth/user/batch-online-status", "{")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.BatchGetOnlineStatus(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeDeviceHandlerCode(t, w))
		assert.False(t, called)
	})

	t.Run("invalid_user_uuids", func(t *testing.T) {
		for _, body := range []string{
			`{"userUuids":[]}`,
			`{"userUuids":["u1","u2","u3","u4","u5","u6","u7","u8","u9","u10","u11","u12","u13","u14","u15","u16","u17","u18","u19","u20","u21","u22","u23","u24","u25","u26","u27","u28","u29","u30","u31","u32","u33","u34","u35","u36","u37","u38","u39","u40","u41","u42","u43","u44","u45","u46","u47","u48","u49","u50","u51","u52","u53","u54","u55","u56","u57","u58","u59","u60","u61","u62","u63","u64","u65","u66","u67","u68","u69","u70","u71","u72","u73","u74","u75","u76","u77","u78","u79","u80","u81","u82","u83","u84","u85","u86","u87","u88","u89","u90","u91","u92","u93","u94","u95","u96","u97","u98","u99","u100","u101"]}`,
		} {
			called := false
			h := NewDeviceHandler(&fakeDeviceHTTPService{
				batchGetOnlineStatusFn: func(_ context.Context, _ *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
					called = true
					return &dto.BatchGetOnlineStatusResponse{}, nil
				},
			})
			w := httptest.NewRecorder()
			req := newDeviceJSONRequest(t, http.MethodPost, "/api/v1/auth/user/batch-online-status", body)
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.BatchGetOnlineStatus(c)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeParamError, decodeDeviceHandlerCode(t, w))
			assert.False(t, called)
		}
	})

	t.Run("success_and_error_mapping", func(t *testing.T) {
		tests := []struct {
			name       string
			setupFn    func(*fakeDeviceHTTPService)
			wantStatus int
			wantCode   int
		}{
			{
				name: "success",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.batchGetOnlineStatusFn = func(_ context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
						require.Equal(t, []string{"u1", "u2"}, req.UserUUIDs)
						return &dto.BatchGetOnlineStatusResponse{}, nil
					}
				},
				wantStatus: http.StatusOK,
				wantCode:   consts.CodeSuccess,
			},
			{
				name: "business_error",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.batchGetOnlineStatusFn = func(_ context.Context, _ *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
						return nil, apperr.ToStatus(apperr.New(consts.CodeParamError))
					}
				},
				wantStatus: http.StatusOK,
				wantCode:   consts.CodeParamError,
			},
			{
				name: "internal_error",
				setupFn: func(s *fakeDeviceHTTPService) {
					s.batchGetOnlineStatusFn = func(_ context.Context, _ *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
						return nil, errors.New("internal")
					}
				},
				wantStatus: http.StatusInternalServerError,
				wantCode:   consts.CodeInternalError,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := &fakeDeviceHTTPService{}
				tt.setupFn(svc)
				h := NewDeviceHandler(svc)

				w := httptest.NewRecorder()
				req := newDeviceJSONRequest(t, http.MethodPost, "/api/v1/auth/user/batch-online-status", `{"userUuids":["u1","u2"]}`)
				c, _ := gin.CreateTestContext(w)
				c.Request = req

				h.BatchGetOnlineStatus(c)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Equal(t, tt.wantCode, decodeDeviceHandlerCode(t, w))
			})
		}
	})
}
