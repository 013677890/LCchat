package service

import (
	"context"
	"errors"
		"sync"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/utils"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/apperr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var gatewayUserServiceTestOnce sync.Once

func initGatewayUserServiceTestEnv() {
	gatewayUserServiceTestOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
		_ = async.Init(config.DefaultAsyncConfig())
	})
}

type fakeGatewayUserServiceClient struct {
	gatewaypb.UserServiceClient

	getProfileFn       func(context.Context, *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error)
	getOtherProfileFn  func(context.Context, *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error)
	checkIsFriendFn    func(context.Context, *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error)
	searchUserFn       func(context.Context, *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error)
	batchIsFriendFn    func(context.Context, *userpb.BatchCheckIsFriendRequest) (*userpb.BatchCheckIsFriendResponse, error)
	updateProfileFn    func(context.Context, *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error)
	changePasswordFn   func(context.Context, *userpb.ChangePasswordRequest) (*userpb.ChangePasswordResponse, error)
	changeEmailFn      func(context.Context, *userpb.ChangeEmailRequest) (*userpb.ChangeEmailResponse, error)
	uploadAvatarFn     func(context.Context, *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error)
	batchGetProfileFn  func(context.Context, *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error)
	getQRCodeFn        func(context.Context, *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error)
	parseQRCodeFn      func(context.Context, *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error)
	deleteAccountFn    func(context.Context, *userpb.DeleteAccountRequest) (*userpb.DeleteAccountResponse, error)
}

func (f *fakeGatewayUserServiceClient) GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
	if f.getProfileFn == nil {
		return nil, errors.New("unexpected GetProfile call")
	}
	return f.getProfileFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
	if f.getOtherProfileFn == nil {
		return nil, errors.New("unexpected GetOtherProfile call")
	}
	return f.getOtherProfileFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) CheckIsFriend(ctx context.Context, req *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error) {
	if f.checkIsFriendFn == nil {
		return nil, errors.New("unexpected CheckIsFriend call")
	}
	return f.checkIsFriendFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
	if f.searchUserFn == nil {
		return nil, errors.New("unexpected SearchUser call")
	}
	return f.searchUserFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) BatchCheckIsFriend(ctx context.Context, req *userpb.BatchCheckIsFriendRequest) (*userpb.BatchCheckIsFriendResponse, error) {
	if f.batchIsFriendFn == nil {
		return nil, errors.New("unexpected BatchCheckIsFriend call")
	}
	return f.batchIsFriendFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error) {
	if f.updateProfileFn == nil {
		return nil, errors.New("unexpected UpdateProfile call")
	}
	return f.updateProfileFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) ChangePassword(ctx context.Context, req *userpb.ChangePasswordRequest) (*userpb.ChangePasswordResponse, error) {
	if f.changePasswordFn == nil {
		return nil, errors.New("unexpected ChangePassword call")
	}
	return f.changePasswordFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) ChangeEmail(ctx context.Context, req *userpb.ChangeEmailRequest) (*userpb.ChangeEmailResponse, error) {
	if f.changeEmailFn == nil {
		return nil, errors.New("unexpected ChangeEmail call")
	}
	return f.changeEmailFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error) {
	if f.uploadAvatarFn == nil {
		return nil, errors.New("unexpected UploadAvatar call")
	}
	return f.uploadAvatarFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	if f.batchGetProfileFn == nil {
		return nil, errors.New("unexpected BatchGetProfile call")
	}
	return f.batchGetProfileFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
	if f.getQRCodeFn == nil {
		return nil, errors.New("unexpected GetQRCode call")
	}
	return f.getQRCodeFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error) {
	if f.parseQRCodeFn == nil {
		return nil, errors.New("unexpected ParseQRCode call")
	}
	return f.parseQRCodeFn(ctx, req)
}

func (f *fakeGatewayUserServiceClient) DeleteAccount(ctx context.Context, req *userpb.DeleteAccountRequest) (*userpb.DeleteAccountResponse, error) {
	if f.deleteAccountFn == nil {
		return nil, errors.New("unexpected DeleteAccount call")
	}
	return f.deleteAccountFn(ctx, req)
}

func TestGatewayUserServiceGetProfile(t *testing.T) {
	initGatewayUserServiceTestEnv()

	t.Run("success", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getProfileFn: func(_ context.Context, _ *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
				return &userpb.GetProfileResponse{
					UserInfo: &userpb.UserInfo{
						Uuid:     "u1",
						Nickname: "nick",
					},
				}, nil
			},
		})

		resp, err := svc.GetProfile(context.Background())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.UserInfo)
		assert.Equal(t, "u1", resp.UserInfo.UUID)
		assert.Equal(t, "nick", resp.UserInfo.Nickname)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getProfileFn: func(_ context.Context, _ *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := svc.GetProfile(context.Background())
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("nil_user_info_returns_internal", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getProfileFn: func(_ context.Context, _ *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
				return &userpb.GetProfileResponse{}, nil
			},
		})

		resp, err := svc.GetProfile(context.Background())
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})
}

func TestGatewayUserServiceGetOtherProfile(t *testing.T) {
	initGatewayUserServiceTestEnv()

	t.Run("missing_user_uuid_context", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{})

		resp, err := svc.GetOtherProfile(context.Background(), &dto.GetOtherProfileRequest{UserUUID: "u2"})
		require.Nil(t, resp)
		require.Equal(t, consts.CodeUnauthorized, apperr.Code(err))
	})

	t.Run("user_service_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("downstream error")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getOtherProfileFn: func(_ context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
				require.Equal(t, "u2", req.UserUuid)
				return nil, wantErr
			},
			checkIsFriendFn: func(_ context.Context, req *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error) {
				require.Equal(t, "u1", req.UserUuid)
				require.Equal(t, "u2", req.PeerUuid)
				return &userpb.CheckIsFriendResponse{IsFriend: false}, nil
			},
		})

		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		resp, err := svc.GetOtherProfile(ctx, &dto.GetOtherProfileRequest{UserUUID: "u2"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("nil_user_info_returns_internal", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getOtherProfileFn: func(_ context.Context, _ *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
				return &userpb.GetOtherProfileResponse{}, nil
			},
			checkIsFriendFn: func(_ context.Context, _ *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error) {
				return &userpb.CheckIsFriendResponse{IsFriend: true}, nil
			},
		})

		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		resp, err := svc.GetOtherProfile(ctx, &dto.GetOtherProfileRequest{UserUUID: "u2"})
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})

	t.Run("friend_service_error_downgrade_and_mask", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getOtherProfileFn: func(_ context.Context, _ *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
				return &userpb.GetOtherProfileResponse{
					UserInfo: &userpb.UserInfo{
						Uuid:      "u2",
						Email:     "alice@example.com",
						Telephone: "13800138000",
					},
				}, nil
			},
			checkIsFriendFn: func(_ context.Context, _ *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error) {
				return nil, errors.New("friend service failed")
			},
		})

		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		resp, err := svc.GetOtherProfile(ctx, &dto.GetOtherProfileRequest{UserUUID: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.UserInfo)
		assert.False(t, resp.IsFriend)
		assert.Equal(t, utils.MaskEmail("alice@example.com"), resp.UserInfo.Email)
		assert.Equal(t, utils.MaskTelephone("13800138000"), resp.UserInfo.Telephone)
	})

	t.Run("friend_true_no_mask", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getOtherProfileFn: func(_ context.Context, _ *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
				return &userpb.GetOtherProfileResponse{
					UserInfo: &userpb.UserInfo{
						Uuid:      "u2",
						Email:     "alice@example.com",
						Telephone: "13800138000",
					},
				}, nil
			},
			checkIsFriendFn: func(_ context.Context, _ *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error) {
				return &userpb.CheckIsFriendResponse{IsFriend: true}, nil
			},
		})

		ctx := context.WithValue(context.Background(), "user_uuid", "u1")
		resp, err := svc.GetOtherProfile(ctx, &dto.GetOtherProfileRequest{UserUUID: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.UserInfo)
		assert.True(t, resp.IsFriend)
		assert.Equal(t, "alice@example.com", resp.UserInfo.Email)
		assert.Equal(t, "13800138000", resp.UserInfo.Telephone)
	})
}

func TestGatewayUserServiceSearchUser(t *testing.T) {
	initGatewayUserServiceTestEnv()

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("search failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			searchUserFn: func(_ context.Context, _ *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := svc.SearchUser(context.Background(), &dto.SearchUserRequest{
			Keyword:  "a",
			Page:     1,
			PageSize: 20,
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("empty_items_should_not_call_batch_friend", func(t *testing.T) {
		called := false
		svc := NewUserService(&fakeGatewayUserServiceClient{
			searchUserFn: func(_ context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
				require.Equal(t, "alice", req.Keyword)
				return &userpb.SearchUserResponse{
					Items: []*userpb.SimpleUserItem{},
				}, nil
			},
			batchIsFriendFn: func(_ context.Context, _ *userpb.BatchCheckIsFriendRequest) (*userpb.BatchCheckIsFriendResponse, error) {
				called = true
				return nil, nil
			},
		})

		resp, err := svc.SearchUser(context.WithValue(context.Background(), "user_uuid", "u1"), &dto.SearchUserRequest{
			Keyword:  "alice",
			Page:     1,
			PageSize: 20,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, called)
	})

	t.Run("batch_friend_check_with_dedup", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			searchUserFn: func(_ context.Context, _ *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
				return &userpb.SearchUserResponse{
					Items: []*userpb.SimpleUserItem{
						{Uuid: "u2", Nickname: "n2"},
						{Uuid: "u3", Nickname: "n3"},
						{Uuid: "u2", Nickname: "n2-dup"},
						nil,
						{Uuid: "", Nickname: "empty"},
					},
				}, nil
			},
			batchIsFriendFn: func(_ context.Context, req *userpb.BatchCheckIsFriendRequest) (*userpb.BatchCheckIsFriendResponse, error) {
				require.Equal(t, "u1", req.UserUuid)
				require.Len(t, req.PeerUuids, 2)
				assert.ElementsMatch(t, []string{"u2", "u3"}, req.PeerUuids)
				return &userpb.BatchCheckIsFriendResponse{
					Items: []*userpb.FriendCheckItem{
						{PeerUuid: "u2", IsFriend: true},
						{PeerUuid: "u3", IsFriend: false},
					},
				}, nil
			},
		})

		resp, err := svc.SearchUser(context.WithValue(context.Background(), "user_uuid", "u1"), &dto.SearchUserRequest{
			Keyword:  "any",
			Page:     1,
			PageSize: 20,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 5)
		assert.True(t, resp.Items[0].IsFriend)
		assert.False(t, resp.Items[1].IsFriend)
		assert.True(t, resp.Items[2].IsFriend)
	})

	t.Run("batch_friend_error_degrade", func(t *testing.T) {
		svc := NewUserService(&fakeGatewayUserServiceClient{
			searchUserFn: func(_ context.Context, _ *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
				return &userpb.SearchUserResponse{
					Items: []*userpb.SimpleUserItem{{Uuid: "u2", Nickname: "n2"}},
				}, nil
			},
			batchIsFriendFn: func(_ context.Context, _ *userpb.BatchCheckIsFriendRequest) (*userpb.BatchCheckIsFriendResponse, error) {
				return nil, errors.New("friend rpc failed")
			},
		})

		resp, err := svc.SearchUser(context.WithValue(context.Background(), "user_uuid", "u1"), &dto.SearchUserRequest{
			Keyword:  "n2",
			Page:     1,
			PageSize: 20,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
		assert.False(t, resp.Items[0].IsFriend)
	})
}

func TestGatewayUserServiceOtherMethods(t *testing.T) {
	initGatewayUserServiceTestEnv()

	t.Run("update_profile_success_and_error", func(t *testing.T) {
		wantErr := errors.New("update failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			updateProfileFn: func(_ context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error) {
				if req.Nickname == "err" {
					return nil, wantErr
				}
				return &userpb.UpdateProfileResponse{
					UserInfo: &userpb.UserInfo{Uuid: "u1", Nickname: req.Nickname},
				}, nil
			},
		})

		okResp, okErr := svc.UpdateProfile(context.Background(), &dto.UpdateProfileRequest{Nickname: "nick"})
		require.NoError(t, okErr)
		require.NotNil(t, okResp)
		assert.Equal(t, "nick", okResp.UserInfo.Nickname)

		errResp, err := svc.UpdateProfile(context.Background(), &dto.UpdateProfileRequest{Nickname: "err"})
		require.Nil(t, errResp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("change_password_success_and_error", func(t *testing.T) {
		wantErr := errors.New("change password failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			changePasswordFn: func(_ context.Context, req *userpb.ChangePasswordRequest) (*userpb.ChangePasswordResponse, error) {
				if req.OldPassword == "bad" {
					return nil, wantErr
				}
				return &userpb.ChangePasswordResponse{}, nil
			},
		})

		require.NoError(t, svc.ChangePassword(context.Background(), &dto.ChangePasswordRequest{
			OldPassword: "old",
			NewPassword: "new",
		}))
		require.ErrorIs(t, svc.ChangePassword(context.Background(), &dto.ChangePasswordRequest{
			OldPassword: "bad",
			NewPassword: "new",
		}), wantErr)
	})

	t.Run("change_email_success_and_error", func(t *testing.T) {
		wantErr := errors.New("change email failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			changeEmailFn: func(_ context.Context, req *userpb.ChangeEmailRequest) (*userpb.ChangeEmailResponse, error) {
				if req.NewEmail == "err@test.com" {
					return nil, wantErr
				}
				return &userpb.ChangeEmailResponse{Email: req.NewEmail}, nil
			},
		})

		okResp, okErr := svc.ChangeEmail(context.Background(), &dto.ChangeEmailRequest{NewEmail: "ok@test.com", VerifyCode: "123456"})
		require.NoError(t, okErr)
		require.NotNil(t, okResp)
		assert.Equal(t, "ok@test.com", okResp.Email)

		errResp, err := svc.ChangeEmail(context.Background(), &dto.ChangeEmailRequest{NewEmail: "err@test.com", VerifyCode: "123456"})
		require.Nil(t, errResp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("upload_avatar_success_and_error", func(t *testing.T) {
		wantErr := errors.New("upload avatar failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			uploadAvatarFn: func(_ context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error) {
				if req.AvatarUrl == "bad-url" {
					return nil, wantErr
				}
				return &userpb.UploadAvatarResponse{AvatarUrl: req.AvatarUrl}, nil
			},
		})

		okURL, okErr := svc.UploadAvatar(context.Background(), "https://cdn/avatar.png")
		require.NoError(t, okErr)
		assert.Equal(t, "https://cdn/avatar.png", okURL)

		errURL, err := svc.UploadAvatar(context.Background(), "bad-url")
		assert.Empty(t, errURL)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("batch_get_profile_success_and_error", func(t *testing.T) {
		wantErr := errors.New("batch failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				if len(req.UserUuids) == 1 && req.UserUuids[0] == "bad" {
					return nil, wantErr
				}
				return &userpb.BatchGetProfileResponse{
					Users: []*userpb.SimpleUserInfo{{Uuid: "u1", Nickname: "n1"}},
				}, nil
			},
		})

		okResp, okErr := svc.BatchGetProfile(context.Background(), &dto.BatchGetProfileRequest{UserUUIDs: []string{"u1"}})
		require.NoError(t, okErr)
		require.NotNil(t, okResp)
		require.Len(t, okResp.Users, 1)
		assert.Equal(t, "u1", okResp.Users[0].UUID)

		errResp, err := svc.BatchGetProfile(context.Background(), &dto.BatchGetProfileRequest{UserUUIDs: []string{"bad"}})
		require.Nil(t, errResp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("qrcode_parse_delete_success_and_error", func(t *testing.T) {
		getErr := errors.New("get qrcode failed")
		parseErr := errors.New("parse qrcode failed")
		deleteErr := errors.New("delete account failed")
		svc := NewUserService(&fakeGatewayUserServiceClient{
			getQRCodeFn: func(_ context.Context, _ *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
				return &userpb.GetQRCodeResponse{Qrcode: "q", ExpireAt: "2026-02-06T00:00:00Z"}, nil
			},
			parseQRCodeFn: func(_ context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error) {
				if req.Token == "bad" {
					return nil, parseErr
				}
				return &userpb.ParseQRCodeResponse{UserUuid: "u1"}, nil
			},
			deleteAccountFn: func(_ context.Context, req *userpb.DeleteAccountRequest) (*userpb.DeleteAccountResponse, error) {
				if req.Password == "bad" {
					return nil, deleteErr
				}
				return &userpb.DeleteAccountResponse{DeleteAt: "now", RecoverDeadline: "later"}, nil
			},
		})

		qrResp, qrErr := svc.GetQRCode(context.Background())
		require.NoError(t, qrErr)
		require.NotNil(t, qrResp)
		assert.Equal(t, "q", qrResp.QRCode)

		parseResp, parseErrGot := svc.ParseQRCode(context.Background(), &dto.ParseQRCodeRequest{Token: "ok"})
		require.NoError(t, parseErrGot)
		require.NotNil(t, parseResp)
		assert.Equal(t, "u1", parseResp.UserUUID)

		delResp, delErr := svc.DeleteAccount(context.Background(), &dto.DeleteAccountRequest{Password: "ok"})
		require.NoError(t, delErr)
		require.NotNil(t, delResp)
		assert.Equal(t, "now", delResp.DeleteAt)

		_, parseErrBad := svc.ParseQRCode(context.Background(), &dto.ParseQRCodeRequest{Token: "bad"})
		require.ErrorIs(t, parseErrBad, parseErr)

		_, deleteErrBad := svc.DeleteAccount(context.Background(), &dto.DeleteAccountRequest{Password: "bad"})
		require.ErrorIs(t, deleteErrBad, deleteErr)

		svcErr := NewUserService(&fakeGatewayUserServiceClient{
			getQRCodeFn: func(_ context.Context, _ *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
				return nil, getErr
			},
		})
		_, gotGetErr := svcErr.GetQRCode(context.Background())
		require.ErrorIs(t, gotGetErr, getErr)
	})
}

