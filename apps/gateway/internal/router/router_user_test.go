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
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeRouterUserService struct {
	service.UserService

	getProfileFn      func(context.Context) (*dto.GetProfileResponse, error)
	getOtherProfileFn func(context.Context, *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error)
	searchUserFn      func(context.Context, *dto.SearchUserRequest) (*dto.SearchUserResponse, error)
	updateProfileFn   func(context.Context, *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error)
	changePasswordFn  func(context.Context, *dto.ChangePasswordRequest) error
	changeEmailFn     func(context.Context, *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error)
	uploadAvatarFn    func(context.Context, string) (string, error)
	getQRCodeFn       func(context.Context) (*dto.GetQRCodeResponse, error)
	parseQRCodeFn     func(context.Context, *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error)
	batchGetProfileFn func(context.Context, *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error)
	deleteAccountFn   func(context.Context, *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error)
}

var _ service.UserService = (*fakeRouterUserService)(nil)

func (f *fakeRouterUserService) GetProfile(ctx context.Context) (*dto.GetProfileResponse, error) {
	if f.getProfileFn == nil {
		return &dto.GetProfileResponse{}, nil
	}
	return f.getProfileFn(ctx)
}

func (f *fakeRouterUserService) GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
	if f.getOtherProfileFn == nil {
		return &dto.GetOtherProfileResponse{}, nil
	}
	return f.getOtherProfileFn(ctx, req)
}

func (f *fakeRouterUserService) SearchUser(ctx context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
	if f.searchUserFn == nil {
		return &dto.SearchUserResponse{}, nil
	}
	return f.searchUserFn(ctx, req)
}

func (f *fakeRouterUserService) UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
	if f.updateProfileFn == nil {
		return &dto.UpdateProfileResponse{}, nil
	}
	return f.updateProfileFn(ctx, req)
}

func (f *fakeRouterUserService) ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error {
	if f.changePasswordFn == nil {
		return nil
	}
	return f.changePasswordFn(ctx, req)
}

func (f *fakeRouterUserService) ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	if f.changeEmailFn == nil {
		return &dto.ChangeEmailResponse{}, nil
	}
	return f.changeEmailFn(ctx, req)
}

func (f *fakeRouterUserService) UploadAvatar(ctx context.Context, avatarURL string) (string, error) {
	if f.uploadAvatarFn == nil {
		return avatarURL, nil
	}
	return f.uploadAvatarFn(ctx, avatarURL)
}

func (f *fakeRouterUserService) GetQRCode(ctx context.Context) (*dto.GetQRCodeResponse, error) {
	if f.getQRCodeFn == nil {
		return &dto.GetQRCodeResponse{}, nil
	}
	return f.getQRCodeFn(ctx)
}

func (f *fakeRouterUserService) ParseQRCode(ctx context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error) {
	if f.parseQRCodeFn == nil {
		return &dto.ParseQRCodeResponse{}, nil
	}
	return f.parseQRCodeFn(ctx, req)
}

func (f *fakeRouterUserService) BatchGetProfile(ctx context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error) {
	if f.batchGetProfileFn == nil {
		return &dto.BatchGetProfileResponse{}, nil
	}
	return f.batchGetProfileFn(ctx, req)
}

func (f *fakeRouterUserService) DeleteAccount(ctx context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
	if f.deleteAccountFn == nil {
		return &dto.DeleteAccountResponse{}, nil
	}
	return f.deleteAccountFn(ctx, req)
}

type routerUserResultBody struct {
	Code int `json:"code"`
}

var routerUserLoggerOnce sync.Once

func initRouterUserTestLogger() {
	routerUserLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
		gin.SetMode(gin.TestMode)
	})
}

func mustUserAuthToken(t *testing.T) string {
	t.Helper()
	token, err := util.GenerateToken("u1", "d1")
	require.NoError(t, err)
	return token
}

func newRouterUserJSONRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newRouterUserAuthedRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req := newRouterUserJSONRequest(t, method, target, body)
	req.Header.Set("Authorization", "Bearer "+mustUserAuthToken(t))
	return req
}

func decodeRouterUserCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body routerUserResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func buildRouterUserTestRouter(userSvc service.UserService) *gin.Engine {
	authHandler := v1.NewAuthHandler(nil)
	userHandler := v1.NewUserHandler(userSvc)
	friendHandler := v1.NewFriendHandler(nil)
	blacklistHandler := v1.NewBlacklistHandler(nil)
	deviceHandler := v1.NewDeviceHandler(nil)
	return InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, nil)
}

func TestRouterUserUnauthorized(t *testing.T) {
	initRouterUserTestLogger()

	r := buildRouterUserTestRouter(&fakeRouterUserService{})
	req := newRouterUserJSONRequest(t, http.MethodGet, "/api/v1/auth/user/profile", "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRouterUserPublicParseQRCode(t *testing.T) {
	initRouterUserTestLogger()

	called := false
	r := buildRouterUserTestRouter(&fakeRouterUserService{
		parseQRCodeFn: func(_ context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error) {
			called = true
			require.Equal(t, "tk1", req.Token)
			return &dto.ParseQRCodeResponse{UserUUID: "u1"}, nil
		},
	})

	req := newRouterUserJSONRequest(t, http.MethodPost, "/api/v1/public/user/parse-qrcode", `{"token":"tk1"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, decodeRouterUserCode(t, w))
	assert.True(t, called)
}

func TestRouterUserAuthRoutesSuccess(t *testing.T) {
	initRouterUserTestLogger()

	tests := []struct {
		name   string
		method string
		target string
		body   string
		setup  func(*fakeRouterUserService, *bool)
	}{
		{
			name:   "get_profile",
			method: http.MethodGet,
			target: "/api/v1/auth/user/profile",
			setup: func(s *fakeRouterUserService, called *bool) {
				s.getProfileFn = func(_ context.Context) (*dto.GetProfileResponse, error) {
					*called = true
					return &dto.GetProfileResponse{}, nil
				}
			},
		},
		{
			name:   "get_other_profile",
			method: http.MethodGet,
			target: "/api/v1/auth/user/profile/u2",
			setup: func(s *fakeRouterUserService, called *bool) {
				s.getOtherProfileFn = func(_ context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
					*called = true
					require.Equal(t, "u2", req.UserUUID)
					return &dto.GetOtherProfileResponse{}, nil
				}
			},
		},
		{
			name:   "search_user",
			method: http.MethodGet,
			target: "/api/v1/auth/user/search?keyword=alice&page=1&pageSize=20",
			setup: func(s *fakeRouterUserService, called *bool) {
				s.searchUserFn = func(_ context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
					*called = true
					require.Equal(t, "alice", req.Keyword)
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(20), req.PageSize)
					return &dto.SearchUserResponse{}, nil
				}
			},
		},
		{
			name:   "search_user_with_default_pagination",
			method: http.MethodGet,
			target: "/api/v1/auth/user/search?keyword=alice",
			setup: func(s *fakeRouterUserService, called *bool) {
				s.searchUserFn = func(_ context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
					*called = true
					require.Equal(t, "alice", req.Keyword)
					require.Equal(t, int32(1), req.Page)
					require.Equal(t, int32(100), req.PageSize)
					return &dto.SearchUserResponse{}, nil
				}
			},
		},
		{
			name:   "update_profile",
			method: http.MethodPut,
			target: "/api/v1/auth/user/profile",
			body:   `{"nickname":"new-nick"}`,
			setup: func(s *fakeRouterUserService, called *bool) {
				s.updateProfileFn = func(_ context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
					*called = true
					require.Equal(t, "new-nick", req.Nickname)
					return &dto.UpdateProfileResponse{}, nil
				}
			},
		},
		{
			name:   "change_password",
			method: http.MethodPost,
			target: "/api/v1/auth/user/change-password",
			body:   `{"oldPassword":"oldpass123","newPassword":"newpass123"}`,
			setup: func(s *fakeRouterUserService, called *bool) {
				s.changePasswordFn = func(_ context.Context, req *dto.ChangePasswordRequest) error {
					*called = true
					require.Equal(t, "oldpass123", req.OldPassword)
					return nil
				}
			},
		},
		{
			name:   "change_email",
			method: http.MethodPost,
			target: "/api/v1/auth/user/change-email",
			body:   `{"newEmail":"a@test.com","verifyCode":"123456"}`,
			setup: func(s *fakeRouterUserService, called *bool) {
				s.changeEmailFn = func(_ context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
					*called = true
					require.Equal(t, "a@test.com", req.NewEmail)
					return &dto.ChangeEmailResponse{}, nil
				}
			},
		},
		{
			name:   "get_qrcode",
			method: http.MethodGet,
			target: "/api/v1/auth/user/qrcode",
			setup: func(s *fakeRouterUserService, called *bool) {
				s.getQRCodeFn = func(_ context.Context) (*dto.GetQRCodeResponse, error) {
					*called = true
					return &dto.GetQRCodeResponse{}, nil
				}
			},
		},
		{
			name:   "batch_profile",
			method: http.MethodPost,
			target: "/api/v1/auth/user/batch-profile",
			body:   `{"userUuids":["u1","u2"]}`,
			setup: func(s *fakeRouterUserService, called *bool) {
				s.batchGetProfileFn = func(_ context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error) {
					*called = true
					require.Equal(t, []string{"u1", "u2"}, req.UserUUIDs)
					return &dto.BatchGetProfileResponse{}, nil
				}
			},
		},
		{
			name:   "delete_account",
			method: http.MethodPost,
			target: "/api/v1/auth/user/delete-account",
			body:   `{"password":"pass123456","reason":"bye"}`,
			setup: func(s *fakeRouterUserService, called *bool) {
				s.deleteAccountFn = func(_ context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
					*called = true
					require.Equal(t, "pass123456", req.Password)
					return &dto.DeleteAccountResponse{}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &fakeRouterUserService{}
			tt.setup(svc, &called)
			r := buildRouterUserTestRouter(svc)

			req := newRouterUserAuthedRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeSuccess, decodeRouterUserCode(t, w))
			assert.True(t, called)
		})
	}
}

func TestRouterUserParamErrors(t *testing.T) {
	initRouterUserTestLogger()
	r := buildRouterUserTestRouter(&fakeRouterUserService{})

	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "search_query_invalid",
			method: http.MethodGet,
			target: "/api/v1/auth/user/search?keyword=",
		},
		{
			name:   "search_query_page_invalid_zero",
			method: http.MethodGet,
			target: "/api/v1/auth/user/search?keyword=alice&page=0",
		},
		{
			name:   "search_query_page_size_invalid_zero",
			method: http.MethodGet,
			target: "/api/v1/auth/user/search?keyword=alice&pageSize=0",
		},
		{
			name:   "search_query_page_size_invalid_too_large",
			method: http.MethodGet,
			target: "/api/v1/auth/user/search?keyword=alice&pageSize=101",
		},
		{
			name:   "update_profile_no_fields",
			method: http.MethodPut,
			target: "/api/v1/auth/user/profile",
			body:   `{}`,
		},
		{
			name:   "batch_profile_empty_list",
			method: http.MethodPost,
			target: "/api/v1/auth/user/batch-profile",
			body:   `{"userUuids":[]}`,
		},
		{
			name:   "avatar_missing_file",
			method: http.MethodPost,
			target: "/api/v1/auth/user/avatar",
			body:   `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newRouterUserAuthedRequest(t, tt.method, tt.target, tt.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, consts.CodeParamError, decodeRouterUserCode(t, w))
		})
	}
}

func TestRouterUserErrorMapping(t *testing.T) {
	initRouterUserTestLogger()

	t.Run("business_error_passthrough", func(t *testing.T) {
		r := buildRouterUserTestRouter(&fakeRouterUserService{
			getProfileFn: func(_ context.Context) (*dto.GetProfileResponse, error) {
				return nil, status.Error(codes.Code(consts.CodeUserNotFound), "biz")
			},
		})

		req := newRouterUserAuthedRequest(t, http.MethodGet, "/api/v1/auth/user/profile", "")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeUserNotFound, decodeRouterUserCode(t, w))
	})

	t.Run("internal_error_mapping", func(t *testing.T) {
		r := buildRouterUserTestRouter(&fakeRouterUserService{
			getProfileFn: func(_ context.Context) (*dto.GetProfileResponse, error) {
				return nil, errors.New("internal")
			},
		})

		req := newRouterUserAuthedRequest(t, http.MethodGet, "/api/v1/auth/user/profile", "")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, consts.CodeInternalError, decodeRouterUserCode(t, w))
	})
}
