package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var userSvcLoggerOnce sync.Once

func initUserSvcTestLogger() {
	userSvcLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

type fakeUserSvcRepo struct {
	repository.IUserRepository

	getByUUIDFn                       func(context.Context, string) (*model.UserInfo, error)
	searchUserFn                      func(context.Context, string, int, int) ([]*model.UserInfo, int64, error)
	updateBasicInfoFn                 func(context.Context, string, string, string, string, int8) error
	updateBasicInfoWithDisplayEventFn func(context.Context, string, string, string, string, int8) (*model.UserInfo, error)
	updateAvatarFn                    func(context.Context, string, string) error
	updateAvatarWithDisplayEventFn    func(context.Context, string, string) (*model.UserInfo, error)
	updatePasswordFn                  func(context.Context, string, string) error
	existsByEmailFn                   func(context.Context, string) (bool, error)
	updateEmailFn                     func(context.Context, string, string) error
	getQRCodeByUserUUIDFn             func(context.Context, string) (string, time.Time, error)
	saveQRCodeFn                      func(context.Context, string, string) error
	getUUIDByQRCodeTokenFn            func(context.Context, string) (string, error)
	deleteFn                          func(context.Context, string) error
	batchGetByUUIDsFn                 func(context.Context, []string) ([]*model.UserInfo, error)
}

func (f *fakeUserSvcRepo) GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error) {
	if f.getByUUIDFn == nil {
		return nil, errors.New("unexpected GetByUUID call")
	}
	return f.getByUUIDFn(ctx, uuid)
}

func (f *fakeUserSvcRepo) SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserInfo, int64, error) {
	if f.searchUserFn == nil {
		return nil, 0, errors.New("unexpected SearchUser call")
	}
	return f.searchUserFn(ctx, keyword, page, pageSize)
}

func (f *fakeUserSvcRepo) UpdateBasicInfo(ctx context.Context, userUUID, nickname, signature, birthday string, gender int8) error {
	if f.updateBasicInfoFn == nil {
		return nil
	}
	return f.updateBasicInfoFn(ctx, userUUID, nickname, signature, birthday, gender)
}

func (f *fakeUserSvcRepo) UpdateBasicInfoWithDisplayEvent(ctx context.Context, userUUID, nickname, signature, birthday string, gender int8) (*model.UserInfo, error) {
	if f.updateBasicInfoWithDisplayEventFn != nil {
		return f.updateBasicInfoWithDisplayEventFn(ctx, userUUID, nickname, signature, birthday, gender)
	}
	if f.updateBasicInfoFn != nil {
		if err := f.updateBasicInfoFn(ctx, userUUID, nickname, signature, birthday, gender); err != nil {
			return nil, err
		}
		if f.getByUUIDFn != nil {
			return f.getByUUIDFn(ctx, userUUID)
		}
		return &model.UserInfo{Uuid: userUUID, Nickname: nickname}, nil
	}
	return nil, errors.New("unexpected UpdateBasicInfoWithDisplayEvent call")
}

func (f *fakeUserSvcRepo) UpdateAvatar(ctx context.Context, userUUID, avatar string) error {
	if f.updateAvatarFn == nil {
		return nil
	}
	return f.updateAvatarFn(ctx, userUUID, avatar)
}

func (f *fakeUserSvcRepo) UpdateAvatarWithDisplayEvent(ctx context.Context, userUUID, avatar string) (*model.UserInfo, error) {
	if f.updateAvatarWithDisplayEventFn != nil {
		return f.updateAvatarWithDisplayEventFn(ctx, userUUID, avatar)
	}
	if f.updateAvatarFn != nil {
		if err := f.updateAvatarFn(ctx, userUUID, avatar); err != nil {
			return nil, err
		}
		if f.getByUUIDFn != nil {
			return f.getByUUIDFn(ctx, userUUID)
		}
		return &model.UserInfo{Uuid: userUUID, Avatar: avatar}, nil
	}
	return nil, errors.New("unexpected UpdateAvatarWithDisplayEvent call")
}

func (f *fakeUserSvcRepo) UpdatePassword(ctx context.Context, userUUID, password string) error {
	if f.updatePasswordFn == nil {
		return nil
	}
	return f.updatePasswordFn(ctx, userUUID, password)
}

func (f *fakeUserSvcRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	if f.existsByEmailFn == nil {
		return false, nil
	}
	return f.existsByEmailFn(ctx, email)
}

func (f *fakeUserSvcRepo) UpdateEmail(ctx context.Context, userUUID, email string) error {
	if f.updateEmailFn == nil {
		return nil
	}
	return f.updateEmailFn(ctx, userUUID, email)
}

func (f *fakeUserSvcRepo) GetQRCodeTokenByUserUUID(ctx context.Context, userUUID string) (string, time.Time, error) {
	if f.getQRCodeByUserUUIDFn == nil {
		return "", time.Time{}, repository.ErrRedisNil
	}
	return f.getQRCodeByUserUUIDFn(ctx, userUUID)
}

func (f *fakeUserSvcRepo) SaveQRCode(ctx context.Context, userUUID, token string) error {
	if f.saveQRCodeFn == nil {
		return nil
	}
	return f.saveQRCodeFn(ctx, userUUID, token)
}

func (f *fakeUserSvcRepo) GetUUIDByQRCodeToken(ctx context.Context, token string) (string, error) {
	if f.getUUIDByQRCodeTokenFn == nil {
		return "", repository.ErrRedisNil
	}
	return f.getUUIDByQRCodeTokenFn(ctx, token)
}

func (f *fakeUserSvcRepo) Delete(ctx context.Context, userUUID string) error {
	if f.deleteFn == nil {
		return nil
	}
	return f.deleteFn(ctx, userUUID)
}

func (f *fakeUserSvcRepo) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error) {
	if f.batchGetByUUIDsFn == nil {
		return nil, errors.New("unexpected BatchGetByUUIDs call")
	}
	return f.batchGetByUUIDsFn(ctx, uuids)
}

type fakeUserSvcAuthRepo struct {
	repository.IAuthRepository

	verifyVerifyCodeFn func(context.Context, string, string, int32) (bool, error)
	deleteVerifyCodeFn func(context.Context, string, int32) error
}

func (f *fakeUserSvcAuthRepo) VerifyVerifyCode(ctx context.Context, email, verifyCode string, codeType int32) (bool, error) {
	if f.verifyVerifyCodeFn == nil {
		return false, errors.New("unexpected VerifyVerifyCode call")
	}
	return f.verifyVerifyCodeFn(ctx, email, verifyCode, codeType)
}

func (f *fakeUserSvcAuthRepo) DeleteVerifyCode(ctx context.Context, email string, codeType int32) error {
	if f.deleteVerifyCodeFn == nil {
		return nil
	}
	return f.deleteVerifyCodeFn(ctx, email, codeType)
}

type fakeUserSvcDeviceRepo struct {
	repository.IDeviceRepository
	deleteByUserUUIDFn func(context.Context, string) error
}

func (f *fakeUserSvcDeviceRepo) DeleteByUserUUID(ctx context.Context, userUUID string) error {
	if f.deleteByUserUUIDFn == nil {
		return nil
	}
	return f.deleteByUserUUIDFn(ctx, userUUID)
}

func userSvcCtx(uuid string) context.Context {
	return context.WithValue(context.Background(), "user_uuid", uuid)
}

func hashUserSvcPassword(t *testing.T, raw string) string {
	t.Helper()
	v, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	require.NoError(t, err)
	return string(v)
}

func requireUserSvcStatus(t *testing.T, err error, _ codes.Code, wantBiz int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantBiz, apperr.Code(err))
	stErr := apperr.ToStatus(err)
	st, ok := status.FromError(stErr)
	require.True(t, ok)
	require.Equal(t, wantBiz, apperr.Code(apperr.FromStatus(stErr)))
	require.NotEmpty(t, st.Message())
}

func TestUserServiceProfileAndSearch(t *testing.T) {
	initUserSvcTestLogger()

	t.Run("get_profile_missing_user_uuid", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.GetProfile(context.Background(), &pb.GetProfileRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("get_profile_success", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, uuid string) (*model.UserInfo, error) {
				require.Equal(t, "u1", uuid)
				return &model.UserInfo{Uuid: "u1", Nickname: "n1"}, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.GetProfile(userSvcCtx("u1"), &pb.GetProfileRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.UserInfo)
		assert.Equal(t, "u1", resp.UserInfo.Uuid)
	})

	t.Run("search_user_missing_user_uuid", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.SearchUser(context.Background(), &pb.SearchUserRequest{Keyword: "a", Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("search_user_repo_error", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			searchUserFn: func(_ context.Context, _ string, _, _ int) ([]*model.UserInfo, int64, error) {
				return nil, 0, errors.New("db error")
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.SearchUser(userSvcCtx("u1"), &pb.SearchUserRequest{Keyword: "a", Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("search_user_success", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			searchUserFn: func(_ context.Context, keyword string, page, pageSize int) ([]*model.UserInfo, int64, error) {
				require.Equal(t, "alice", keyword)
				require.Equal(t, 1, page)
				require.Equal(t, 20, pageSize)
				return []*model.UserInfo{{Uuid: "u2", Nickname: "n2"}}, 1, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.SearchUser(userSvcCtx("u1"), &pb.SearchUserRequest{Keyword: "alice", Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
		assert.Equal(t, "u2", resp.Items[0].Uuid)
	})
}

func TestUserServiceUpdateAndAvatar(t *testing.T) {
	initUserSvcTestLogger()

	t.Run("update_profile_empty_request", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.UpdateProfile(userSvcCtx("u1"), &pb.UpdateProfileRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("update_profile_birthday_format_error", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.UpdateProfile(userSvcCtx("u1"), &pb.UpdateProfileRequest{Birthday: "2026/02/06"})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.InvalidArgument, consts.CodeBirthdayFormatError)
	})

	t.Run("update_profile_success", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			updateBasicInfoFn: func(_ context.Context, userUUID, nickname, _, _ string, _ int8) error {
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "new-nick", nickname)
				return nil
			},
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Nickname: "new-nick"}, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.UpdateProfile(userSvcCtx("u1"), &pb.UpdateProfileRequest{Nickname: "new-nick"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "new-nick", resp.UserInfo.Nickname)
	})

	t.Run("upload_avatar_empty_url", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.UploadAvatar(userSvcCtx("u1"), &pb.UploadAvatarRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("upload_avatar_success", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			updateAvatarFn: func(_ context.Context, userUUID, avatar string) error {
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "https://cdn/a.png", avatar)
				return nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.UploadAvatar(userSvcCtx("u1"), &pb.UploadAvatarRequest{AvatarUrl: "https://cdn/a.png"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "https://cdn/a.png", resp.AvatarUrl)
	})
}

func TestUserServiceChangePasswordAndEmail(t *testing.T) {
	initUserSvcTestLogger()
	oldHash := hashUserSvcPassword(t, "oldpass123")

	t.Run("change_password_old_password_wrong", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHash}, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		err := svc.ChangePassword(userSvcCtx("u1"), &pb.ChangePasswordRequest{OldPassword: "wrong", NewPassword: "newpass123"})
		requireUserSvcStatus(t, err, codes.Unauthenticated, consts.CodePasswordError)
	})

	t.Run("change_password_same_as_old", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHash}, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		err := svc.ChangePassword(userSvcCtx("u1"), &pb.ChangePasswordRequest{OldPassword: "oldpass123", NewPassword: "oldpass123"})
		requireUserSvcStatus(t, err, codes.FailedPrecondition, consts.CodePasswordSameAsOld)
	})

	t.Run("change_password_success", func(t *testing.T) {
		updated := false
		svc := NewUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: oldHash}, nil
			},
			updatePasswordFn: func(_ context.Context, userUUID, password string) error {
				updated = true
				require.Equal(t, "u1", userUUID)
				require.NotEmpty(t, password)
				return nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		err := svc.ChangePassword(userSvcCtx("u1"), &pb.ChangePasswordRequest{OldPassword: "oldpass123", NewPassword: "newpass123"})
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("change_email_already_exists", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			existsByEmailFn: func(_ context.Context, _ string) (bool, error) {
				return true, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.ChangeEmail(userSvcCtx("u1"), &pb.ChangeEmailRequest{NewEmail: "a@test.com", VerifyCode: "123456"})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.AlreadyExists, consts.CodeEmailAlreadyExist)
	})

	t.Run("change_email_verify_code_expired", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			existsByEmailFn: func(_ context.Context, _ string) (bool, error) {
				return false, nil
			},
		}, &fakeUserSvcAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, _ int32) (bool, error) {
				return false, repository.ErrRedisNil
			},
		}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.ChangeEmail(userSvcCtx("u1"), &pb.ChangeEmailRequest{NewEmail: "a@test.com", VerifyCode: "123456"})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Unauthenticated, consts.CodeVerifyCodeExpire)
	})

	t.Run("change_email_success_with_delete_code_error", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			existsByEmailFn: func(_ context.Context, _ string) (bool, error) {
				return false, nil
			},
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Email: "old@test.com"}, nil
			},
			updateEmailFn: func(_ context.Context, userUUID, email string) error {
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "a@test.com", email)
				return nil
			},
		}, &fakeUserSvcAuthRepo{
			verifyVerifyCodeFn: func(_ context.Context, _, _ string, codeType int32) (bool, error) {
				require.Equal(t, int32(4), codeType)
				return true, nil
			},
			deleteVerifyCodeFn: func(_ context.Context, _ string, _ int32) error {
				return errors.New("delete code failed")
			},
		}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.ChangeEmail(userSvcCtx("u1"), &pb.ChangeEmailRequest{NewEmail: "a@test.com", VerifyCode: "123456"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "a@test.com", resp.Email)
	})
}

func TestUserServiceQRCodeDeleteAndBatch(t *testing.T) {
	initUserSvcTestLogger()

	t.Run("get_qrcode_existing_token", func(t *testing.T) {
		expireAt := time.Now().Add(12 * time.Hour)
		svc := NewUserService(&fakeUserSvcRepo{
			getQRCodeByUserUUIDFn: func(_ context.Context, _ string) (string, time.Time, error) {
				return "tk1", expireAt, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.GetQRCode(userSvcCtx("u1"), &pb.GetQRCodeRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "https://www.LCchat.top/q/tk1", resp.Qrcode)
	})

	t.Run("get_qrcode_save_error", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			getQRCodeByUserUUIDFn: func(_ context.Context, _ string) (string, time.Time, error) {
				return "", time.Time{}, repository.ErrRedisNil
			},
			saveQRCodeFn: func(_ context.Context, _, _ string) error {
				return errors.New("save failed")
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp, err := svc.GetQRCode(userSvcCtx("u1"), &pb.GetQRCodeRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("parse_qrcode_empty_or_expired_or_success", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp1, err1 := svc.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{})
		require.Nil(t, resp1)
		requireUserSvcStatus(t, err1, codes.InvalidArgument, consts.CodeQRCodeFormatError)

		svcExpired := NewUserService(&fakeUserSvcRepo{
			getUUIDByQRCodeTokenFn: func(_ context.Context, _ string) (string, error) {
				return "", repository.ErrRedisNil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp2, err2 := svcExpired.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{Token: "tk1"})
		require.Nil(t, resp2)
		requireUserSvcStatus(t, err2, codes.NotFound, consts.CodeQRCodeExpired)

		svcOK := NewUserService(&fakeUserSvcRepo{
			getUUIDByQRCodeTokenFn: func(_ context.Context, token string) (string, error) {
				require.Equal(t, "tk1", token)
				return "u1", nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		resp3, err3 := svcOK.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{Token: "tk1"})
		require.NoError(t, err3)
		require.NotNil(t, resp3)
		assert.Equal(t, "u1", resp3.UserUuid)
	})

	t.Run("delete_account_password_wrong_and_success", func(t *testing.T) {
		hash := hashUserSvcPassword(t, "pass123456")
		svcWrong := NewUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: hash}, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		respWrong, errWrong := svcWrong.DeleteAccount(userSvcCtx("u1"), &pb.DeleteAccountRequest{Password: "wrong"})
		require.Nil(t, respWrong)
		requireUserSvcStatus(t, errWrong, codes.Unauthenticated, consts.CodePasswordError)

		svcOK := NewUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, _ string) (*model.UserInfo, error) {
				return &model.UserInfo{Uuid: "u1", Password: hash}, nil
			},
			deleteFn: func(_ context.Context, userUUID string) error {
				require.Equal(t, "u1", userUUID)
				return nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})
		respOK, errOK := svcOK.DeleteAccount(userSvcCtx("u1"), &pb.DeleteAccountRequest{Password: "pass123456"})
		require.NoError(t, errOK)
		require.NotNil(t, respOK)
		assert.NotEmpty(t, respOK.DeleteAt)
	})

	t.Run("batch_get_profile_empty_too_many_success", func(t *testing.T) {
		svc := NewUserService(&fakeUserSvcRepo{
			batchGetByUUIDsFn: func(_ context.Context, _ []string) ([]*model.UserInfo, error) {
				return []*model.UserInfo{{Uuid: "u1", Nickname: "n1"}}, nil
			},
		}, &fakeUserSvcAuthRepo{}, &fakeUserSvcDeviceRepo{})

		respEmpty, errEmpty := svc.BatchGetProfile(context.Background(), &pb.BatchGetProfileRequest{UserUuids: []string{}})
		require.NoError(t, errEmpty)
		require.NotNil(t, respEmpty)
		assert.Empty(t, respEmpty.Users)

		uuids := make([]string, 101)
		for i := range uuids {
			uuids[i] = "u" + strconv.Itoa(i)
		}
		respTooMany, errTooMany := svc.BatchGetProfile(context.Background(), &pb.BatchGetProfileRequest{UserUuids: uuids})
		require.Nil(t, respTooMany)
		requireUserSvcStatus(t, errTooMany, codes.InvalidArgument, consts.CodeParamError)

		respOK, errOK := svc.BatchGetProfile(context.Background(), &pb.BatchGetProfileRequest{UserUuids: []string{"u1"}})
		require.NoError(t, errOK)
		require.NotNil(t, respOK)
		require.Len(t, respOK.Users, 1)
		assert.Equal(t, "u1", respOK.Users[0].Uuid)
	})
}
