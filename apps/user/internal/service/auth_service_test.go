package service

import (
	"context"
	"errors"
		"sync"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var userAuthLoggerOnce sync.Once

func initUserAuthTestLogger() {
	userAuthLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

type fakeAuthRepo struct {
	repository.IAuthRepository

	getByEmailFn                func(ctx context.Context, email string) (*model.UserInfo, error)
	verifyVerifyCodeFn          func(ctx context.Context, email, verifyCode string, codeType int32) (bool, error)
	createFn                    func(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error)
	verifyVerifyCodeRateLimitFn func(ctx context.Context, email, ip string) (bool, error)
	storeVerifyCodeFn           func(ctx context.Context, email, verifyCode string, codeType int32, expireDuration time.Duration) error
	incrementVerifyCodeCountFn  func(ctx context.Context, email, ip string) error
	deleteVerifyCodeFn          func(ctx context.Context, email string, codeType int32) error
	updatePasswordFn            func(ctx context.Context, userUUID, password string) error
}

var _ repository.IAuthRepository = (*fakeAuthRepo)(nil)

func (f *fakeAuthRepo) GetByEmail(ctx context.Context, email string) (*model.UserInfo, error) {
	if f.getByEmailFn == nil {
		return nil, errors.New("unexpected GetByEmail call")
	}
	return f.getByEmailFn(ctx, email)
}

func (f *fakeAuthRepo) VerifyVerifyCode(ctx context.Context, email, verifyCode string, codeType int32) (bool, error) {
	if f.verifyVerifyCodeFn == nil {
		return false, errors.New("unexpected VerifyVerifyCode call")
	}
	return f.verifyVerifyCodeFn(ctx, email, verifyCode, codeType)
}

func (f *fakeAuthRepo) Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	if f.createFn == nil {
		return nil, errors.New("unexpected Create call")
	}
	return f.createFn(ctx, user)
}

func (f *fakeAuthRepo) VerifyVerifyCodeRateLimit(ctx context.Context, email string, ip string) (bool, error) {
	if f.verifyVerifyCodeRateLimitFn == nil {
		return false, errors.New("unexpected VerifyVerifyCodeRateLimit call")
	}
	return f.verifyVerifyCodeRateLimitFn(ctx, email, ip)
}

func (f *fakeAuthRepo) StoreVerifyCode(ctx context.Context, email, verifyCode string, codeType int32, expireDuration time.Duration) error {
	if f.storeVerifyCodeFn == nil {
		return errors.New("unexpected StoreVerifyCode call")
	}
	return f.storeVerifyCodeFn(ctx, email, verifyCode, codeType, expireDuration)
}

func (f *fakeAuthRepo) IncrementVerifyCodeCount(ctx context.Context, email, ip string) error {
	if f.incrementVerifyCodeCountFn == nil {
		return nil
	}
	return f.incrementVerifyCodeCountFn(ctx, email, ip)
}

func (f *fakeAuthRepo) DeleteVerifyCode(ctx context.Context, email string, codeType int32) error {
	if f.deleteVerifyCodeFn == nil {
		return nil
	}
	return f.deleteVerifyCodeFn(ctx, email, codeType)
}

func (f *fakeAuthRepo) UpdatePassword(ctx context.Context, userUUID, password string) error {
	if f.updatePasswordFn == nil {
		return errors.New("unexpected UpdatePassword call")
	}
	return f.updatePasswordFn(ctx, userUUID, password)
}

type fakeAuthDeviceRepo struct {
	repository.IDeviceRepository

	storeAccessTokenFn   func(ctx context.Context, userUUID, deviceID, accessToken string, expireDuration time.Duration) error
	storeRefreshTokenFn  func(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error
	upsertSessionFn      func(ctx context.Context, session *model.DeviceSession) error
	setActiveTsFn        func(ctx context.Context, userUUID, deviceID string, ts int64) error
	getRefreshTokenFn    func(ctx context.Context, userUUID, deviceID string) (string, error)
	touchDeviceInfoFn    func(ctx context.Context, userUUID string) error
	deleteTokensFn       func(ctx context.Context, userUUID, deviceID string) error
	updateOnlineStatusFn func(ctx context.Context, userUUID, deviceID string, status int8) error
}

var _ repository.IDeviceRepository = (*fakeAuthDeviceRepo)(nil)

func (f *fakeAuthDeviceRepo) StoreAccessToken(ctx context.Context, userUUID, deviceID, accessToken string, expireDuration time.Duration) error {
	if f.storeAccessTokenFn == nil {
		return nil
	}
	return f.storeAccessTokenFn(ctx, userUUID, deviceID, accessToken, expireDuration)
}

func (f *fakeAuthDeviceRepo) StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error {
	if f.storeRefreshTokenFn == nil {
		return nil
	}
	return f.storeRefreshTokenFn(ctx, userUUID, deviceID, refreshToken, expireDuration)
}

func (f *fakeAuthDeviceRepo) UpsertSession(ctx context.Context, session *model.DeviceSession) error {
	if f.upsertSessionFn == nil {
		return nil
	}
	return f.upsertSessionFn(ctx, session)
}

func (f *fakeAuthDeviceRepo) SetActiveTimestamp(ctx context.Context, userUUID, deviceID string, ts int64) error {
	if f.setActiveTsFn == nil {
		return nil
	}
	return f.setActiveTsFn(ctx, userUUID, deviceID, ts)
}

func (f *fakeAuthDeviceRepo) GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error) {
	if f.getRefreshTokenFn == nil {
		return "", errors.New("unexpected GetRefreshToken call")
	}
	return f.getRefreshTokenFn(ctx, userUUID, deviceID)
}

func (f *fakeAuthDeviceRepo) TouchDeviceInfoTTL(ctx context.Context, userUUID string) error {
	if f.touchDeviceInfoFn == nil {
		return nil
	}
	return f.touchDeviceInfoFn(ctx, userUUID)
}

func (f *fakeAuthDeviceRepo) DeleteTokens(ctx context.Context, userUUID, deviceID string) error {
	if f.deleteTokensFn == nil {
		return nil
	}
	return f.deleteTokensFn(ctx, userUUID, deviceID)
}

func (f *fakeAuthDeviceRepo) UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error {
	if f.updateOnlineStatusFn == nil {
		return nil
	}
	return f.updateOnlineStatusFn(ctx, userUUID, deviceID, status)
}

func requireAuthStatusCode(t *testing.T, err error, _ codes.Code, wantBizCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantBizCode, apperr.Code(err))
	stErr := apperr.ToStatus(err)
	st, ok := status.FromError(stErr)
	require.True(t, ok)
	require.Equal(t, wantBizCode, apperr.Code(apperr.FromStatus(stErr)))
	require.NotEmpty(t, st.Message())
}

func mustHashPassword(t *testing.T, raw string) string {
	t.Helper()
	pwd, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	require.NoError(t, err)
	return string(pwd)
}

func TestUserAuthServiceRegister(t *testing.T) {
	initUserAuthTestLogger()

	t.Run("verify_code_expired", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(1), codeType)
				return false, repository.ErrRedisNil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Register(context.Background(), &pb.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeError)
	})

	t.Run("verify_code_invalid", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(1), codeType)
				return false, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Register(context.Background(), &pb.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeError)
	})

	t.Run("verify_code_internal_error", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return false, errors.New("redis error")
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Register(context.Background(), &pb.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("duplicate_user", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
			createFn: func(_ context.Context, _ *model.UserInfo) (*model.UserInfo, error) {
				return nil, repository.ErrDuplicateKey
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Register(context.Background(), &pb.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.AlreadyExists, consts.CodeUserAlreadyExist)
	})

	t.Run("success", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(1), codeType)
				return true, nil
			},
			createFn: func(_ context.Context, user *model.UserInfo) (*model.UserInfo, error) {
				require.Equal(t, "a@test.com", user.Email)
				require.NotEmpty(t, user.Password)
				return &model.UserInfo{
					Uuid:      "u1",
					Email:     user.Email,
					Nickname:  "n1",
					Telephone: "13800138000",
				}, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Register(context.Background(), &pb.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
			Nickname:   "n1",
			Telephone:  "13800138000",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "u1", resp.UserUuid)
		assert.Equal(t, "n1", resp.Nickname)
	})

	t.Run("success_without_telephone", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(1), codeType)
				return true, nil
			},
			createFn: func(_ context.Context, user *model.UserInfo) (*model.UserInfo, error) {
				require.Equal(t, "a@test.com", user.Email)
				require.NotEmpty(t, user.Password)
				require.Equal(t, "", user.Telephone)
				return &model.UserInfo{
					Uuid:      user.Uuid,
					Email:     user.Email,
					Nickname:  "n1",
					Telephone: user.Telephone,
				}, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Register(context.Background(), &pb.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
			Nickname:   "n1",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.Telephone)
	})
}

func TestUserAuthServiceLogin(t *testing.T) {
	initUserAuthTestLogger()

	validUser := &model.UserInfo{
		Uuid:      "u1",
		Email:     "a@test.com",
		Password:  mustHashPassword(t, "pass123"),
		Nickname:  "n1",
		Telephone: "13800138000",
		Status:    0,
	}

	t.Run("user_not_found", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return nil, repository.ErrRecordNotFound
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Login(context.Background(), &pb.LoginRequest{
			Account:  "a@test.com",
			Password: "pass123",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.NotFound, consts.CodeUserNotFound)
	})

	t.Run("user_disabled", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				u.Status = 1
				return &u, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Login(context.Background(), &pb.LoginRequest{
			Account:    "a@test.com",
			Password:   "pass123",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.PermissionDenied, consts.CodeUserDisabled)
	})

	t.Run("password_error", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Login(context.Background(), &pb.LoginRequest{
			Account:    "a@test.com",
			Password:   "wrong",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodePasswordError)
	})

	t.Run("missing_device_id", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.Login(context.Background(), &pb.LoginRequest{
			Account:    "a@test.com",
			Password:   "pass123",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("store_access_token_failed", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
		}
		deviceRepo := &fakeAuthDeviceRepo{
			storeAccessTokenFn: func(_ context.Context, _, _, _ string, _ time.Duration) error {
				return errors.New("redis write error")
			},
		}
		svc := NewAuthService(repo, deviceRepo)

		ctx := context.WithValue(context.Background(), "device_id", "d1")
		resp, err := svc.Login(ctx, &pb.LoginRequest{
			Account:    "a@test.com",
			Password:   "pass123",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("store_refresh_token_failed", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
		}
		deviceRepo := &fakeAuthDeviceRepo{
			storeRefreshTokenFn: func(_ context.Context, _, _, _ string, _ time.Duration) error {
				return errors.New("redis write error")
			},
		}
		svc := NewAuthService(repo, deviceRepo)

		ctx := context.WithValue(context.Background(), "device_id", "d1")
		resp, err := svc.Login(ctx, &pb.LoginRequest{
			Account:    "a@test.com",
			Password:   "pass123",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_with_degraded_session_updates", func(t *testing.T) {
		var upsertCalled bool
		var activeCalled bool
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
		}
		deviceRepo := &fakeAuthDeviceRepo{
			upsertSessionFn: func(_ context.Context, session *model.DeviceSession) error {
				upsertCalled = true
				require.Equal(t, "u1", session.UserUuid)
				require.Equal(t, "d1", session.DeviceId)
				return errors.New("db temporary error")
			},
			setActiveTsFn: func(_ context.Context, userUUID, deviceID string, _ int64) error {
				activeCalled = true
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "d1", deviceID)
				return errors.New("redis temporary error")
			},
		}
		svc := NewAuthService(repo, deviceRepo)

		ctx := context.WithValue(context.Background(), "device_id", "d1")
		resp, err := svc.Login(ctx, &pb.LoginRequest{
			Account:  "a@test.com",
			Password: "pass123",
			DeviceInfo: &pb.DeviceInfo{
				DeviceName: "iphone",
				Platform:   "ios",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		require.NotNil(t, resp.UserInfo)
		assert.Equal(t, "u1", resp.UserInfo.Uuid)
		assert.True(t, upsertCalled)
		assert.True(t, activeCalled)
	})
}

func TestUserAuthServiceLoginByCode(t *testing.T) {
	initUserAuthTestLogger()

	validUser := &model.UserInfo{
		Uuid:      "u1",
		Email:     "a@test.com",
		Password:  mustHashPassword(t, "pass123"),
		Nickname:  "n1",
		Telephone: "13800138000",
		Status:    0,
	}

	t.Run("user_not_found", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return nil, repository.ErrRecordNotFound
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.LoginByCode(context.Background(), &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.NotFound, consts.CodeUserNotFound)
	})

	t.Run("user_disabled", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				u.Status = 1
				return &u, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.LoginByCode(context.Background(), &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.PermissionDenied, consts.CodeUserDisabled)
	})

	t.Run("verify_code_expired", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(2), codeType)
				return false, repository.ErrRedisNil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})
		ctx := context.WithValue(context.Background(), "device_id", "d1")

		resp, err := svc.LoginByCode(ctx, &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeExpire)
	})

	t.Run("verify_code_invalid", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(2), codeType)
				return false, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})
		ctx := context.WithValue(context.Background(), "device_id", "d1")

		resp, err := svc.LoginByCode(ctx, &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeError)
	})

	t.Run("missing_device_id", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.LoginByCode(context.Background(), &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("store_access_token_failed", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
		}
		deviceRepo := &fakeAuthDeviceRepo{
			storeAccessTokenFn: func(_ context.Context, _, _, _ string, _ time.Duration) error {
				return errors.New("redis error")
			},
		}
		svc := NewAuthService(repo, deviceRepo)

		ctx := context.WithValue(context.Background(), "device_id", "d1")
		resp, err := svc.LoginByCode(ctx, &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_with_delete_code_failure", func(t *testing.T) {
		var deleteCalled bool
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				u := *validUser
				return &u, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
			deleteVerifyCodeFn: func(_ context.Context, _ string, _ int32) error {
				deleteCalled = true
				return errors.New("delete failed")
			},
		}
		deviceRepo := &fakeAuthDeviceRepo{
			upsertSessionFn: func(_ context.Context, _ *model.DeviceSession) error {
				return errors.New("db temporary error")
			},
			setActiveTsFn: func(_ context.Context, _, _ string, _ int64) error {
				return errors.New("redis temporary error")
			},
		}
		svc := NewAuthService(repo, deviceRepo)

		ctx := context.WithValue(context.Background(), "device_id", "d1")
		resp, err := svc.LoginByCode(ctx, &pb.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			DeviceInfo: &pb.DeviceInfo{DeviceName: "iphone"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.True(t, deleteCalled)
	})
}

func TestUserAuthServiceSendVerifyCode(t *testing.T) {
	initUserAuthTestLogger()

	originalCfg := util.GetEmailConfig()
	t.Cleanup(func() {
		util.SetEmailConfig(originalCfg)
	})
	util.SetEmailConfig(util.EmailConfig{})

	t.Run("invalid_email", func(t *testing.T) {
		svc := NewAuthService(&fakeAuthRepo{}, &fakeAuthDeviceRepo{})

		resp, err := svc.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{
			Email: "invalid",
			Type:  2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeInvalidEmail)
	})

	t.Run("rate_limited", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeRateLimitFn: func(_ context.Context, _, _ string) (bool, error) {
				return true, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{
			Email: "a@test.com",
			Type:  2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.ResourceExhausted, consts.CodeSendTooFrequent)
	})

	t.Run("rate_limit_check_error", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeRateLimitFn: func(_ context.Context, _, _ string) (bool, error) {
				return false, errors.New("redis error")
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{
			Email: "a@test.com",
			Type:  2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("store_verify_code_error", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeRateLimitFn: func(_ context.Context, _, _ string) (bool, error) {
				return false, nil
			},
			storeVerifyCodeFn: func(_ context.Context, _, _ string, _ int32, _ time.Duration) error {
				return errors.New("redis error")
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{
			Email: "a@test.com",
			Type:  2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("email_send_failed", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeRateLimitFn: func(_ context.Context, _, _ string) (bool, error) {
				return false, nil
			},
			storeVerifyCodeFn: func(_ context.Context, _, _ string, _ int32, _ time.Duration) error {
				return nil
			},
			incrementVerifyCodeCountFn: func(_ context.Context, _, _ string) error {
				return nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{
			Email: "a@test.com",
			Type:  2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})
}

func TestUserAuthServiceVerifyCode(t *testing.T) {
	initUserAuthTestLogger()

	t.Run("verify_code_expired", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return false, repository.ErrRedisNil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.VerifyCode(context.Background(), &pb.VerifyCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			Type:       2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeExpire)
	})

	t.Run("verify_code_internal_error", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return false, errors.New("redis error")
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.VerifyCode(context.Background(), &pb.VerifyCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			Type:       2,
		})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("valid_true", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.VerifyCode(context.Background(), &pb.VerifyCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			Type:       2,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Valid)
	})

	t.Run("valid_false", func(t *testing.T) {
		repo := &fakeAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return false, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		resp, err := svc.VerifyCode(context.Background(), &pb.VerifyCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			Type:       2,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Valid)
	})
}

func TestUserAuthServiceRefreshToken(t *testing.T) {
	initUserAuthTestLogger()

	t.Run("missing_user_uuid", func(t *testing.T) {
		svc := NewAuthService(&fakeAuthRepo{}, &fakeAuthDeviceRepo{})
		resp, err := svc.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeInvalidToken)
	})

	t.Run("missing_device_id", func(t *testing.T) {
		svc := NewAuthService(&fakeAuthRepo{}, &fakeAuthDeviceRepo{})
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		resp, err := svc.RefreshToken(ctx, &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeInvalidToken)
	})

	t.Run("refresh_token_not_found", func(t *testing.T) {
		deviceRepo := &fakeAuthDeviceRepo{
			getRefreshTokenFn: func(_ context.Context, _, _ string) (string, error) {
				return "", repository.ErrRedisNil
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		ctx = context.WithValue(ctx, "device_id", "d1")

		resp, err := svc.RefreshToken(ctx, &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.NotFound, consts.CodeDeviceNotFound)
	})

	t.Run("refresh_token_mismatch", func(t *testing.T) {
		deviceRepo := &fakeAuthDeviceRepo{
			getRefreshTokenFn: func(_ context.Context, _, _ string) (string, error) {
				return "stored-token", nil
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		ctx = context.WithValue(ctx, "device_id", "d1")

		resp, err := svc.RefreshToken(ctx, &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeInvalidToken)
	})

	t.Run("store_access_token_failed", func(t *testing.T) {
		deviceRepo := &fakeAuthDeviceRepo{
			getRefreshTokenFn: func(_ context.Context, _, _ string) (string, error) {
				return "rtk", nil
			},
			storeAccessTokenFn: func(_ context.Context, _, _, _ string, _ time.Duration) error {
				return errors.New("redis error")
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		ctx = context.WithValue(ctx, "device_id", "d1")

		resp, err := svc.RefreshToken(ctx, &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.Nil(t, resp)
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_with_touch_ttl_failed", func(t *testing.T) {
		var touchCalled bool
		deviceRepo := &fakeAuthDeviceRepo{
			getRefreshTokenFn: func(_ context.Context, userUUID, deviceID string) (string, error) {
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "d1", deviceID)
				return "rtk", nil
			},
			touchDeviceInfoFn: func(_ context.Context, userUUID string) error {
				touchCalled = true
				require.Equal(t, "u1", userUUID)
				return errors.New("redis warning")
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		ctx = context.WithValue(ctx, "device_id", "d1")

		resp, err := svc.RefreshToken(ctx, &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.AccessToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.True(t, touchCalled)
	})
}

func TestUserAuthServiceLogout(t *testing.T) {
	initUserAuthTestLogger()

	t.Run("nil_request", func(t *testing.T) {
		svc := NewAuthService(&fakeAuthRepo{}, &fakeAuthDeviceRepo{})
		err := svc.Logout(context.Background(), nil)
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("empty_device_id", func(t *testing.T) {
		svc := NewAuthService(&fakeAuthRepo{}, &fakeAuthDeviceRepo{})
		err := svc.Logout(context.Background(), &pb.LogoutRequest{})
		requireAuthStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("missing_user_uuid_context", func(t *testing.T) {
		svc := NewAuthService(&fakeAuthRepo{}, &fakeAuthDeviceRepo{})
		err := svc.Logout(context.Background(), &pb.LogoutRequest{DeviceId: "d1"})
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("delete_tokens_failed", func(t *testing.T) {
		deviceRepo := &fakeAuthDeviceRepo{
			deleteTokensFn: func(_ context.Context, _, _ string) error {
				return errors.New("redis error")
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")

		err := svc.Logout(ctx, &pb.LogoutRequest{DeviceId: "d1"})
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("update_online_status_not_found_is_idempotent_success", func(t *testing.T) {
		deviceRepo := &fakeAuthDeviceRepo{
			updateOnlineStatusFn: func(_ context.Context, _, _ string, _ int8) error {
				return repository.ErrRecordNotFound
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")

		err := svc.Logout(ctx, &pb.LogoutRequest{DeviceId: "d1"})
		require.NoError(t, err)
	})

	t.Run("set_active_timestamp_failed_not_blocking", func(t *testing.T) {
		deviceRepo := &fakeAuthDeviceRepo{
			setActiveTsFn: func(_ context.Context, _, _ string, _ int64) error {
				return errors.New("redis warning")
			},
		}
		svc := NewAuthService(&fakeAuthRepo{}, deviceRepo)
		ctx := context.WithValue(context.Background(), "user_uuid", "u1")

		err := svc.Logout(ctx, &pb.LogoutRequest{DeviceId: "d1"})
		require.NoError(t, err)
	})
}

func TestUserAuthServiceResetPassword(t *testing.T) {
	initUserAuthTestLogger()

	oldHashed := mustHashPassword(t, "oldpass123")

	t.Run("user_not_found", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return nil, repository.ErrRecordNotFound
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		err := svc.ResetPassword(context.Background(), &pb.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "newpass123",
		})
		requireAuthStatusCode(t, err, codes.NotFound, consts.CodeUserNotFound)
	})

	t.Run("verify_code_expired", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHashed}, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(3), codeType)
				return false, repository.ErrRedisNil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		err := svc.ResetPassword(context.Background(), &pb.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "newpass123",
		})
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeExpire)
	})

	t.Run("verify_code_invalid", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHashed}, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(3), codeType)
				return false, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		err := svc.ResetPassword(context.Background(), &pb.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "newpass123",
		})
		requireAuthStatusCode(t, err, codes.Unauthenticated, consts.CodeVerifyCodeError)
	})

	t.Run("new_password_same_as_old", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHashed}, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		err := svc.ResetPassword(context.Background(), &pb.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "oldpass123",
		})
		requireAuthStatusCode(t, err, codes.FailedPrecondition, consts.CodePasswordSameAsOld)
	})

	t.Run("update_password_failed", func(t *testing.T) {
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHashed}, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
			updatePasswordFn: func(_ context.Context, _, _ string) error {
				return errors.New("db error")
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		err := svc.ResetPassword(context.Background(), &pb.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "newpass123",
		})
		requireAuthStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_with_delete_code_error", func(t *testing.T) {
		var deleteCalled bool
		repo := &fakeAuthRepo{
			getByEmailFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHashed}, nil
			},
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return true, nil
			},
			updatePasswordFn: func(_ context.Context, userUUID, password string) error {
				require.Equal(t, "u1", userUUID)
				require.NotEmpty(t, password)
				return nil
			},
			deleteVerifyCodeFn: func(_ context.Context, _ string, _ int32) error {
				deleteCalled = true
				return errors.New("delete error")
			},
		}
		svc := NewAuthService(repo, &fakeAuthDeviceRepo{})

		err := svc.ResetPassword(context.Background(), &pb.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "newpass123",
		})
		require.NoError(t, err)
		assert.True(t, deleteCalled)
	})
}
