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
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

	getByUUIDFn                       func(context.Context, string) (*model.UserProfile, error)
	searchUserFn                      func(context.Context, string, int, int) ([]*model.UserProfile, int64, error)
	updateBasicInfoWithDisplayEventFn func(context.Context, string, string, string, string, int8) (*model.UserProfile, error)
	updateAvatarWithDisplayEventFn    func(context.Context, string, string) (*model.UserProfile, error)
	getQRCodeByUserUUIDFn             func(context.Context, string) (string, time.Time, error)
	saveQRCodeFn                      func(context.Context, string, string) error
	getUUIDByQRCodeTokenFn            func(context.Context, string) (string, error)
	batchGetByUUIDsFn                 func(context.Context, []string) ([]*model.UserProfile, error)
}

func (f *fakeUserSvcRepo) GetByUUID(ctx context.Context, uuid string) (*model.UserProfile, error) {
	if f.getByUUIDFn == nil {
		return nil, errors.New("unexpected GetByUUID call")
	}
	return f.getByUUIDFn(ctx, uuid)
}

func (f *fakeUserSvcRepo) SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserProfile, int64, error) {
	if f.searchUserFn == nil {
		return nil, 0, errors.New("unexpected SearchUser call")
	}
	return f.searchUserFn(ctx, keyword, page, pageSize)
}

func (f *fakeUserSvcRepo) UpdateBasicInfoWithDisplayEvent(ctx context.Context, userUUID, nickname, signature, birthday string, gender int8) (*model.UserProfile, error) {
	if f.updateBasicInfoWithDisplayEventFn == nil {
		return nil, errors.New("unexpected UpdateBasicInfoWithDisplayEvent call")
	}
	return f.updateBasicInfoWithDisplayEventFn(ctx, userUUID, nickname, signature, birthday, gender)
}

func (f *fakeUserSvcRepo) UpdateAvatarWithDisplayEvent(ctx context.Context, userUUID, avatar string) (*model.UserProfile, error) {
	if f.updateAvatarWithDisplayEventFn == nil {
		return nil, errors.New("unexpected UpdateAvatarWithDisplayEvent call")
	}
	return f.updateAvatarWithDisplayEventFn(ctx, userUUID, avatar)
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

func (f *fakeUserSvcRepo) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserProfile, error) {
	if f.batchGetByUUIDsFn == nil {
		return nil, errors.New("unexpected BatchGetByUUIDs call")
	}
	return f.batchGetByUUIDsFn(ctx, uuids)
}

func userSvcCtx(uuid string) context.Context {
	return context.WithValue(context.Background(), "user_uuid", uuid)
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
		svc := NewProfileUserService(&fakeUserSvcRepo{})
		resp, err := svc.GetProfile(context.Background(), &pb.GetProfileRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("get_profile_success", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			getByUUIDFn: func(_ context.Context, uuid string) (*model.UserProfile, error) {
				require.Equal(t, "u1", uuid)
				return &model.UserProfile{UserUuid: "u1", Nickname: "n1"}, nil
			},
		})
		resp, err := svc.GetProfile(userSvcCtx("u1"), &pb.GetProfileRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.UserInfo)
		assert.Equal(t, "u1", resp.UserInfo.Uuid)
	})

	t.Run("search_user_missing_user_uuid", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{})
		resp, err := svc.SearchUser(context.Background(), &pb.SearchUserRequest{Keyword: "a", Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("search_user_repo_error", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			searchUserFn: func(_ context.Context, _ string, _, _ int) ([]*model.UserProfile, int64, error) {
				return nil, 0, errors.New("db error")
			},
		})
		resp, err := svc.SearchUser(userSvcCtx("u1"), &pb.SearchUserRequest{Keyword: "a", Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("search_user_success", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			searchUserFn: func(_ context.Context, keyword string, page, pageSize int) ([]*model.UserProfile, int64, error) {
				require.Equal(t, "alice", keyword)
				require.Equal(t, 1, page)
				require.Equal(t, 20, pageSize)
				return []*model.UserProfile{{UserUuid: "u2", Nickname: "n2"}}, 1, nil
			},
		})
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
		svc := NewProfileUserService(&fakeUserSvcRepo{})
		resp, err := svc.UpdateProfile(userSvcCtx("u1"), &pb.UpdateProfileRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("update_profile_birthday_format_error", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{})
		resp, err := svc.UpdateProfile(userSvcCtx("u1"), &pb.UpdateProfileRequest{Birthday: "2026/02/06"})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.InvalidArgument, consts.CodeBirthdayFormatError)
	})

	t.Run("update_profile_success", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			updateBasicInfoWithDisplayEventFn: func(_ context.Context, userUUID, nickname, _, _ string, _ int8) (*model.UserProfile, error) {
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "new-nick", nickname)
				return &model.UserProfile{UserUuid: "u1", Nickname: "new-nick"}, nil
			},
		})
		resp, err := svc.UpdateProfile(userSvcCtx("u1"), &pb.UpdateProfileRequest{Nickname: "new-nick"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "new-nick", resp.UserInfo.Nickname)
	})

	t.Run("upload_avatar_empty_url", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{})
		resp, err := svc.UploadAvatar(userSvcCtx("u1"), &pb.UploadAvatarRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("upload_avatar_success", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			updateAvatarWithDisplayEventFn: func(_ context.Context, userUUID, avatar string) (*model.UserProfile, error) {
				require.Equal(t, "u1", userUUID)
				require.Equal(t, "https://cdn/a.png", avatar)
				return &model.UserProfile{UserUuid: userUUID, Avatar: avatar}, nil
			},
		})
		resp, err := svc.UploadAvatar(userSvcCtx("u1"), &pb.UploadAvatarRequest{AvatarUrl: "https://cdn/a.png"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "https://cdn/a.png", resp.AvatarUrl)
	})
}

func TestUserServiceQRCodeDeleteAndBatch(t *testing.T) {
	initUserSvcTestLogger()
	require.NoError(t, util.InitSnowflake(201))

	t.Run("get_qrcode_existing_token", func(t *testing.T) {
		expireAt := time.Now().Add(12 * time.Hour)
		svc := NewProfileUserService(&fakeUserSvcRepo{
			getQRCodeByUserUUIDFn: func(_ context.Context, _ string) (string, time.Time, error) {
				return "tk1", expireAt, nil
			},
		})
		resp, err := svc.GetQRCode(userSvcCtx("u1"), &pb.GetQRCodeRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "https://www.LCchat.top/q/tk1", resp.Qrcode)
	})

	t.Run("get_qrcode_save_error", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			getQRCodeByUserUUIDFn: func(_ context.Context, _ string) (string, time.Time, error) {
				return "", time.Time{}, repository.ErrRedisNil
			},
			saveQRCodeFn: func(_ context.Context, _, _ string) error {
				return errors.New("save failed")
			},
		})
		resp, err := svc.GetQRCode(userSvcCtx("u1"), &pb.GetQRCodeRequest{})
		require.Nil(t, resp)
		requireUserSvcStatus(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("parse_qrcode_empty_or_expired_or_success", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{})
		resp1, err1 := svc.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{})
		require.Nil(t, resp1)
		requireUserSvcStatus(t, err1, codes.InvalidArgument, consts.CodeQRCodeFormatError)

		svcExpired := NewProfileUserService(&fakeUserSvcRepo{
			getUUIDByQRCodeTokenFn: func(_ context.Context, _ string) (string, error) {
				return "", repository.ErrRedisNil
			},
		})
		resp2, err2 := svcExpired.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{Token: "tk1"})
		require.Nil(t, resp2)
		requireUserSvcStatus(t, err2, codes.NotFound, consts.CodeQRCodeExpired)

		svcOK := NewProfileUserService(&fakeUserSvcRepo{
			getUUIDByQRCodeTokenFn: func(_ context.Context, token string) (string, error) {
				require.Equal(t, "tk1", token)
				return "u1", nil
			},
		})
		resp3, err3 := svcOK.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{Token: "tk1"})
		require.NoError(t, err3)
		require.NotNil(t, resp3)
		assert.Equal(t, "u1", resp3.UserUuid)
	})

	t.Run("batch_get_profile_empty_too_many_success", func(t *testing.T) {
		svc := NewProfileUserService(&fakeUserSvcRepo{
			batchGetByUUIDsFn: func(_ context.Context, _ []string) ([]*model.UserProfile, error) {
				return []*model.UserProfile{{UserUuid: "u1", Nickname: "n1"}}, nil
			},
		})

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
