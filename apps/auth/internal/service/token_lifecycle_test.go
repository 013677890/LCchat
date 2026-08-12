package service

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type tokenLifecycleAuthRepo struct {
	repository.IAuthRepository
	user               *model.UserAccount
	getByUserUUIDErr   error
	verifyCodeValid    bool
	updatePasswordErr  error
	getByUserUUIDCalls int
}

func (r *tokenLifecycleAuthRepo) GetByUserUUID(_ context.Context, _ string) (*model.UserAccount, error) {
	r.getByUserUUIDCalls++
	if r.getByUserUUIDErr != nil {
		return nil, r.getByUserUUIDErr
	}
	return r.user, nil
}

func (r *tokenLifecycleAuthRepo) GetByEmail(_ context.Context, _ string) (*model.UserAccount, error) {
	if r.getByUserUUIDErr != nil {
		return nil, r.getByUserUUIDErr
	}
	return r.user, nil
}

func (r *tokenLifecycleAuthRepo) VerifyVerifyCode(_ context.Context, _, _ string, _ int32) (bool, error) {
	return r.verifyCodeValid, nil
}

func (r *tokenLifecycleAuthRepo) UpdatePassword(_ context.Context, _ string, _ string) error {
	return r.updatePasswordErr
}

func (r *tokenLifecycleAuthRepo) DeleteVerifyCode(_ context.Context, _ string, _ int32) error {
	return nil
}

type tokenLifecycleDeviceRepo struct {
	repository.IDeviceRepository
	refreshToken       string
	getRefreshErr      error
	deleteByUserErr    error
	deleteByUserCalls  []string
	deleteRefreshCalls []string
}

func (r *tokenLifecycleDeviceRepo) GetRefreshToken(_ context.Context, _, _ string) (string, error) {
	return r.refreshToken, r.getRefreshErr
}

func (r *tokenLifecycleDeviceRepo) TouchDeviceInfoTTL(_ context.Context, _ string) error {
	return nil
}

func (r *tokenLifecycleDeviceRepo) DeleteRefreshToken(_ context.Context, userUUID, deviceID string) error {
	r.deleteRefreshCalls = append(r.deleteRefreshCalls, userUUID+"/"+deviceID)
	return nil
}

func (r *tokenLifecycleDeviceRepo) DeleteByUserUUID(_ context.Context, userUUID string) error {
	r.deleteByUserCalls = append(r.deleteByUserCalls, userUUID)
	return r.deleteByUserErr
}

func tokenContext() context.Context {
	ctx := ctxmeta.WithUserUUID(context.Background(), "user-1")
	return ctxmeta.WithDeviceID(ctx, "device-1")
}

func TestRefreshTokenChecksAccountAfterCredentialMatch(t *testing.T) {
	t.Run("valid account receives stateless access token", func(t *testing.T) {
		authRepo := &tokenLifecycleAuthRepo{user: &model.UserAccount{UserUuid: "user-1"}}
		deviceRepo := &tokenLifecycleDeviceRepo{refreshToken: "refresh-1"}
		service := NewAuthService(authRepo, deviceRepo)

		response, err := service.RefreshToken(tokenContext(), &authpb.RefreshTokenRequest{RefreshToken: "refresh-1"})
		require.NoError(t, err)
		require.NotEmpty(t, response.GetAccessToken())
		require.Equal(t, 1, authRepo.getByUserUUIDCalls)
	})

	t.Run("deleted account cannot extend session", func(t *testing.T) {
		authRepo := &tokenLifecycleAuthRepo{getByUserUUIDErr: repoerr.ErrRecordNotFound}
		deviceRepo := &tokenLifecycleDeviceRepo{refreshToken: "refresh-1"}
		service := NewAuthService(authRepo, deviceRepo)

		_, err := service.RefreshToken(tokenContext(), &authpb.RefreshTokenRequest{RefreshToken: "refresh-1"})
		require.Equal(t, consts.CodeInvalidToken, apperr.Code(err))
		require.Equal(t, []string{"user-1/device-1"}, deviceRepo.deleteRefreshCalls)
	})

	t.Run("disabled account cannot extend session", func(t *testing.T) {
		authRepo := &tokenLifecycleAuthRepo{user: &model.UserAccount{UserUuid: "user-1", Status: 1}}
		deviceRepo := &tokenLifecycleDeviceRepo{refreshToken: "refresh-1"}
		service := NewAuthService(authRepo, deviceRepo)

		_, err := service.RefreshToken(tokenContext(), &authpb.RefreshTokenRequest{RefreshToken: "refresh-1"})
		require.Equal(t, consts.CodeUserDisabled, apperr.Code(err))
		require.Equal(t, []string{"user-1/device-1"}, deviceRepo.deleteRefreshCalls)
	})

	t.Run("invalid credential does not reveal account state", func(t *testing.T) {
		authRepo := &tokenLifecycleAuthRepo{getByUserUUIDErr: errors.New("must not be called")}
		deviceRepo := &tokenLifecycleDeviceRepo{refreshToken: "refresh-1"}
		service := NewAuthService(authRepo, deviceRepo)

		_, err := service.RefreshToken(tokenContext(), &authpb.RefreshTokenRequest{RefreshToken: "wrong"})
		require.Equal(t, consts.CodeInvalidToken, apperr.Code(err))
		require.Zero(t, authRepo.getByUserUUIDCalls)
	})
}

func TestPasswordChangesRevokeAllRefreshTokens(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("OldPass9"), bcrypt.MinCost)
	require.NoError(t, err)

	t.Run("change password", func(t *testing.T) {
		authRepo := &tokenLifecycleAuthRepo{user: &model.UserAccount{UserUuid: "user-1", PasswordHash: string(passwordHash)}}
		deviceRepo := &tokenLifecycleDeviceRepo{}
		service := NewAccountService(authRepo, deviceRepo)

		err := service.ChangePassword(tokenContext(), &authpb.ChangePasswordRequest{OldPassword: "OldPass9", NewPassword: "NewPass9"})
		require.NoError(t, err)
		require.Equal(t, []string{"user-1"}, deviceRepo.deleteByUserCalls)
	})

	t.Run("reset password", func(t *testing.T) {
		authRepo := &tokenLifecycleAuthRepo{
			user:            &model.UserAccount{UserUuid: "user-1", PasswordHash: string(passwordHash)},
			verifyCodeValid: true,
		}
		deviceRepo := &tokenLifecycleDeviceRepo{}
		service := NewAuthService(authRepo, deviceRepo)

		err := service.ResetPassword(context.Background(), &authpb.ResetPasswordRequest{
			Email: "user@example.com", VerifyCode: "123456", NewPassword: "NewPass9",
		})
		require.NoError(t, err)
		require.Equal(t, []string{"user-1"}, deviceRepo.deleteByUserCalls)
	})
}
