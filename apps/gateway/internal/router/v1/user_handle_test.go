package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUserHTTPService struct {
	service.UserService

	getProfileFn      func(context.Context) (*dto.GetProfileResponse, error)
	getOtherProfileFn func(context.Context, *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error)
	searchUserFn      func(context.Context, *dto.SearchUserRequest) (*dto.SearchUserResponse, error)
	updateProfileFn   func(context.Context, *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error)
	uploadAvatarFn    func(context.Context, string) (string, error)
	changePasswordFn  func(context.Context, *dto.ChangePasswordRequest) error
	changeEmailFn     func(context.Context, *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error)
	getQRCodeFn       func(context.Context) (*dto.GetQRCodeResponse, error)
	parseQRCodeFn     func(context.Context, *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error)
	batchGetProfileFn func(context.Context, *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error)
	deleteAccountFn   func(context.Context, *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error)
}

func (f *fakeUserHTTPService) GetProfile(ctx context.Context) (*dto.GetProfileResponse, error) {
	if f.getProfileFn == nil {
		return &dto.GetProfileResponse{}, nil
	}
	return f.getProfileFn(ctx)
}

func (f *fakeUserHTTPService) GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
	if f.getOtherProfileFn == nil {
		return &dto.GetOtherProfileResponse{}, nil
	}
	return f.getOtherProfileFn(ctx, req)
}

func (f *fakeUserHTTPService) SearchUser(ctx context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
	if f.searchUserFn == nil {
		return &dto.SearchUserResponse{}, nil
	}
	return f.searchUserFn(ctx, req)
}

func (f *fakeUserHTTPService) UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
	if f.updateProfileFn == nil {
		return &dto.UpdateProfileResponse{}, nil
	}
	return f.updateProfileFn(ctx, req)
}

func (f *fakeUserHTTPService) UploadAvatar(ctx context.Context, avatarURL string) (string, error) {
	if f.uploadAvatarFn == nil {
		return avatarURL, nil
	}
	return f.uploadAvatarFn(ctx, avatarURL)
}

func (f *fakeUserHTTPService) ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error {
	if f.changePasswordFn == nil {
		return nil
	}
	return f.changePasswordFn(ctx, req)
}

func (f *fakeUserHTTPService) ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	if f.changeEmailFn == nil {
		return &dto.ChangeEmailResponse{}, nil
	}
	return f.changeEmailFn(ctx, req)
}

func (f *fakeUserHTTPService) GetQRCode(ctx context.Context) (*dto.GetQRCodeResponse, error) {
	if f.getQRCodeFn == nil {
		return &dto.GetQRCodeResponse{}, nil
	}
	return f.getQRCodeFn(ctx)
}

func (f *fakeUserHTTPService) ParseQRCode(ctx context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error) {
	if f.parseQRCodeFn == nil {
		return &dto.ParseQRCodeResponse{}, nil
	}
	return f.parseQRCodeFn(ctx, req)
}

func (f *fakeUserHTTPService) BatchGetProfile(ctx context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error) {
	if f.batchGetProfileFn == nil {
		return &dto.BatchGetProfileResponse{}, nil
	}
	return f.batchGetProfileFn(ctx, req)
}

func (f *fakeUserHTTPService) DeleteAccount(ctx context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
	if f.deleteAccountFn == nil {
		return &dto.DeleteAccountResponse{}, nil
	}
	return f.deleteAccountFn(ctx, req)
}

type userHandlerResultBody struct {
	Code int `json:"code"`
}

var userHandlerLoggerOnce sync.Once

func initUserHandlerLogger() {
	userHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}

func decodeUserHandlerCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body userHandlerResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

func newUserJSONRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newUserMultipartRequest(t *testing.T, target, fieldName, fileName string, data []byte, contentType string) *http.Request {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, target, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Override file part content type for handler checks.
	req.Header.Set("X-Test-ContentType", contentType)
	return req
}

func setMultipartFileHeaderContentType(t *testing.T, req *http.Request, want string) {
	t.Helper()
	err := req.ParseMultipartForm(3 * 1024 * 1024)
	require.NoError(t, err)
	files := req.MultipartForm.File["avatar"]
	require.Len(t, files, 1)
	files[0].Header.Set("Content-Type", want)
}

func TestUserHandlerGetProfile(t *testing.T) {
	initUserHandlerLogger()

	tests := []struct {
		name       string
		setup      func(*fakeUserHTTPService, *bool)
		wantStatus int
		wantCode   int
		wantCalled bool
	}{
		{
			name: "success",
			setup: func(s *fakeUserHTTPService, called *bool) {
				s.getProfileFn = func(_ context.Context) (*dto.GetProfileResponse, error) {
					*called = true
					return &dto.GetProfileResponse{}, nil
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeSuccess,
			wantCalled: true,
		},
		{
			name: "business_error",
			setup: func(s *fakeUserHTTPService, called *bool) {
				s.getProfileFn = func(_ context.Context) (*dto.GetProfileResponse, error) {
					*called = true
					return nil, apperr.ToStatus(apperr.New(consts.CodeUserNotFound))
				}
			},
			wantStatus: http.StatusOK,
			wantCode:   consts.CodeUserNotFound,
			wantCalled: true,
		},
		{
			name: "internal_error",
			setup: func(s *fakeUserHTTPService, called *bool) {
				s.getProfileFn = func(_ context.Context) (*dto.GetProfileResponse, error) {
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
			svc := &fakeUserHTTPService{}
			tt.setup(svc, &called)
			h := NewUserHandler(svc)

			w := httptest.NewRecorder()
			req := newUserJSONRequest(t, http.MethodGet, "/api/v1/auth/user/profile", "")
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			h.GetProfile(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeUserHandlerCode(t, w))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}

func TestUserHandlerGetOtherProfile(t *testing.T) {
	initUserHandlerLogger()

	t.Run("missing_path_param", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodGet, "/api/v1/auth/user/profile/", "")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetOtherProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			getOtherProfileFn: func(_ context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
				require.Equal(t, "u2", req.UserUUID)
				return &dto.GetOtherProfileResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodGet, "/api/v1/auth/user/profile/u2", "")
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = gin.Params{{Key: "userUuid", Value: "u2"}}

		h.GetOtherProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})
}

func TestUserHandlerSearchUser(t *testing.T) {
	initUserHandlerLogger()

	t.Run("bind_query_failed", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/user/search?keyword=", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SearchUser(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			searchUserFn: func(_ context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
				require.Equal(t, "alice", req.Keyword)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return &dto.SearchUserResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/user/search?keyword=alice&page=1&pageSize=20", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SearchUser(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})

	t.Run("success_with_default_pagination", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			searchUserFn: func(_ context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
				require.Equal(t, "alice", req.Keyword)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(100), req.PageSize)
				return &dto.SearchUserResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/user/search?keyword=alice", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SearchUser(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})

	t.Run("page_zero_failed", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/user/search?keyword=alice&page=0", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SearchUser(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("page_size_zero_failed", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/user/search?keyword=alice&pageSize=0", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SearchUser(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("page_size_too_large_failed", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/user/search?keyword=alice&pageSize=101", nil)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.SearchUser(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})
}

func TestUserHandlerChangePasswordAndUpdateProfile(t *testing.T) {
	initUserHandlerLogger()

	t.Run("change_password_bind_failed", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/change-password", "{")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.ChangePassword(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("change_password_business_error", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			changePasswordFn: func(_ context.Context, _ *dto.ChangePasswordRequest) error {
				return apperr.ToStatus(apperr.New(consts.CodePasswordError))
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/change-password", `{"oldPassword":"oldpass123","newPassword":"newpass123"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.ChangePassword(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodePasswordError, decodeUserHandlerCode(t, w))
	})

	t.Run("update_profile_no_fields", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPut, "/api/v1/auth/user/profile", `{}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.UpdateProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("update_profile_success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			updateProfileFn: func(_ context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
				require.Equal(t, "new-nick", req.Nickname)
				require.Nil(t, req.Gender)
				return &dto.UpdateProfileResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPut, "/api/v1/auth/user/profile", `{"nickname":"new-nick"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.UpdateProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})

	t.Run("update_profile_gender_only_success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			updateProfileFn: func(_ context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
				require.NotNil(t, req.Gender)
				require.Equal(t, int32(3), *req.Gender)
				return &dto.UpdateProfileResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPut, "/api/v1/auth/user/profile", `{"gender":3}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.UpdateProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})
}

func TestUserHandlerChangeEmailBatchGetProfileAndQRCode(t *testing.T) {
	initUserHandlerLogger()

	t.Run("change_email_internal_error", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			changeEmailFn: func(_ context.Context, _ *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
				return nil, errors.New("internal")
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/change-email", `{"newEmail":"a@test.com","verifyCode":"123456"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.ChangeEmail(c)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, consts.CodeInternalError, decodeUserHandlerCode(t, w))
	})

	t.Run("batch_get_profile_param_error", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/batch-profile", `{"userUuids":[]}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.BatchGetProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("batch_get_profile_success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			batchGetProfileFn: func(_ context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error) {
				require.Equal(t, []string{"u1", "u2"}, req.UserUUIDs)
				return &dto.BatchGetProfileResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/batch-profile", `{"userUuids":["u1","u2"]}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.BatchGetProfile(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})

	t.Run("get_qrcode_business_error", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			getQRCodeFn: func(_ context.Context) (*dto.GetQRCodeResponse, error) {
				return nil, apperr.ToStatus(apperr.New(consts.CodeQRCodeExpired))
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodGet, "/api/v1/auth/user/qrcode", "")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.GetQRCode(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeQRCodeExpired, decodeUserHandlerCode(t, w))
	})

	t.Run("parse_qrcode_success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			parseQRCodeFn: func(_ context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error) {
				require.Equal(t, "token-1", req.Token)
				return &dto.ParseQRCodeResponse{UserUUID: "u1"}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/public/user/parse-qrcode", `{"token":"token-1"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.ParseQRCode(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})
}

func TestUserHandlerDeleteAccount(t *testing.T) {
	initUserHandlerLogger()

	t.Run("bind_failed", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/delete-account", "{")
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.DeleteAccount(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("success", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			deleteAccountFn: func(_ context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
				require.Equal(t, "pass123456", req.Password)
				return &dto.DeleteAccountResponse{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/delete-account", `{"password":"pass123456","reason":"bye"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.DeleteAccount(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, decodeUserHandlerCode(t, w))
	})

	t.Run("internal_error", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			deleteAccountFn: func(_ context.Context, _ *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
				return nil, errors.New("internal")
			},
		})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/delete-account", `{"password":"pass123456","reason":"bye"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.DeleteAccount(c)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, consts.CodeInternalError, decodeUserHandlerCode(t, w))
	})
}

func TestUserHandlerUploadAvatar(t *testing.T) {
	initUserHandlerLogger()

	origin := pkgminio.Client()
	t.Cleanup(func() {
		pkgminio.ReplaceGlobal(origin)
	})
	pkgminio.ReplaceGlobal(nil)

	t.Run("missing_file", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		w := httptest.NewRecorder()
		req := newUserJSONRequest(t, http.MethodPost, "/api/v1/auth/user/avatar", `{}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		h.UploadAvatar(c)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeParamError, decodeUserHandlerCode(t, w))
	})

	t.Run("file_too_large", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		large := bytes.Repeat([]byte("a"), 2*1024*1024+1)
		req := newUserMultipartRequest(t, "/api/v1/auth/user/avatar", "avatar", "big.png", large, "image/png")
		setMultipartFileHeaderContentType(t, req, "image/png")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		h.UploadAvatar(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeBodyTooLarge, decodeUserHandlerCode(t, w))
	})

	t.Run("unsupported_type", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{})
		req := newUserMultipartRequest(t, "/api/v1/auth/user/avatar", "avatar", "a.txt", []byte("plain"), "text/plain")
		setMultipartFileHeaderContentType(t, req, "text/plain")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		h.UploadAvatar(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeFileFormatNotSupport, decodeUserHandlerCode(t, w))
	})

	t.Run("minio_not_initialized", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHTTPService{
			uploadAvatarFn: func(_ context.Context, _ string) (string, error) {
				return "", errors.New("should not be called")
			},
		})

		png := []byte(strings.Repeat("a", 1024))
		req := newUserMultipartRequest(t, "/api/v1/auth/user/avatar", "avatar", "a.png", png, "image/png")
		setMultipartFileHeaderContentType(t, req, "image/png")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		h.UploadAvatar(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, consts.CodeInternalError, decodeUserHandlerCode(t, w))
	})
}
