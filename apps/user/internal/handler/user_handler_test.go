package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUserHandlerService struct {
	service.IUserService

	getProfileFn      func(context.Context, *pb.GetProfileRequest) (*pb.GetProfileResponse, error)
	getOtherProfileFn func(context.Context, *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error)
	searchUserFn      func(context.Context, *pb.SearchUserRequest) (*pb.SearchUserResponse, error)
	updateProfileFn   func(context.Context, *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error)
	uploadAvatarFn    func(context.Context, *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error)
	getQRCodeFn       func(context.Context, *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error)
	parseQRCodeFn     func(context.Context, *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error)
	batchGetProfileFn func(context.Context, *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error)
}

var _ service.IUserService = (*fakeUserHandlerService)(nil)

func (f *fakeUserHandlerService) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	if f.getProfileFn == nil {
		return &pb.GetProfileResponse{}, nil
	}
	return f.getProfileFn(ctx, req)
}

func (f *fakeUserHandlerService) GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
	if f.getOtherProfileFn == nil {
		return &pb.GetOtherProfileResponse{}, nil
	}
	return f.getOtherProfileFn(ctx, req)
}

func (f *fakeUserHandlerService) SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
	if f.searchUserFn == nil {
		return &pb.SearchUserResponse{}, nil
	}
	return f.searchUserFn(ctx, req)
}

func (f *fakeUserHandlerService) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	if f.updateProfileFn == nil {
		return &pb.UpdateProfileResponse{}, nil
	}
	return f.updateProfileFn(ctx, req)
}

func (f *fakeUserHandlerService) UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
	if f.uploadAvatarFn == nil {
		return &pb.UploadAvatarResponse{}, nil
	}
	return f.uploadAvatarFn(ctx, req)
}

func (f *fakeUserHandlerService) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	if f.getQRCodeFn == nil {
		return &pb.GetQRCodeResponse{}, nil
	}
	return f.getQRCodeFn(ctx, req)
}

func (f *fakeUserHandlerService) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	if f.parseQRCodeFn == nil {
		return &pb.ParseQRCodeResponse{}, nil
	}
	return f.parseQRCodeFn(ctx, req)
}

func (f *fakeUserHandlerService) BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
	if f.batchGetProfileFn == nil {
		return &pb.BatchGetProfileResponse{}, nil
	}
	return f.batchGetProfileFn(ctx, req)
}

func TestUserHandlerForwardingContracts(t *testing.T) {
	t.Run("get_profile_success_and_error", func(t *testing.T) {
		want := &pb.GetProfileResponse{UserInfo: &pb.UserInfo{Uuid: "u1"}}
		wantErr := errors.New("get profile failed")
		h := NewUserHandler(&fakeUserHandlerService{
			getProfileFn: func(_ context.Context, _ *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
				return want, nil
			},
		})

		resp, err := h.GetProfile(context.Background(), &pb.GetProfileRequest{})
		require.NoError(t, err)
		assert.Equal(t, want, resp)

		hErr := NewUserHandler(&fakeUserHandlerService{
			getProfileFn: func(_ context.Context, _ *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
				return nil, wantErr
			},
		})
		respErr, errGot := hErr.GetProfile(context.Background(), &pb.GetProfileRequest{})
		assert.Nil(t, respErr)
		require.ErrorIs(t, errGot, wantErr)
	})

	t.Run("get_other_profile_success_and_error", func(t *testing.T) {
		want := &pb.GetOtherProfileResponse{UserInfo: &pb.UserInfo{Uuid: "u2"}}
		wantErr := errors.New("get other failed")
		h := NewUserHandler(&fakeUserHandlerService{
			getOtherProfileFn: func(_ context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
				require.Equal(t, "u2", req.UserUuid)
				return want, nil
			},
		})

		resp, err := h.GetOtherProfile(context.Background(), &pb.GetOtherProfileRequest{UserUuid: "u2"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)

		hErr := NewUserHandler(&fakeUserHandlerService{
			getOtherProfileFn: func(_ context.Context, _ *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
				return nil, wantErr
			},
		})
		respErr, errGot := hErr.GetOtherProfile(context.Background(), &pb.GetOtherProfileRequest{UserUuid: "u2"})
		assert.Nil(t, respErr)
		require.ErrorIs(t, errGot, wantErr)
	})

	t.Run("search_update_upload_success_and_error", func(t *testing.T) {
		h := NewUserHandler(&fakeUserHandlerService{
			searchUserFn: func(_ context.Context, _ *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
				return &pb.SearchUserResponse{}, nil
			},
			updateProfileFn: func(_ context.Context, _ *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
				return &pb.UpdateProfileResponse{}, nil
			},
			uploadAvatarFn: func(_ context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
				return &pb.UploadAvatarResponse{AvatarUrl: req.AvatarUrl}, nil
			},
		})

		searchResp, searchErr := h.SearchUser(context.Background(), &pb.SearchUserRequest{Keyword: "a", Page: 1, PageSize: 20})
		require.NoError(t, searchErr)
		require.NotNil(t, searchResp)

		updateResp, updateErr := h.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{Nickname: "new"})
		require.NoError(t, updateErr)
		require.NotNil(t, updateResp)

		avatarResp, avatarErr := h.UploadAvatar(context.Background(), &pb.UploadAvatarRequest{AvatarUrl: "url"})
		require.NoError(t, avatarErr)
		require.NotNil(t, avatarResp)
		assert.Equal(t, "url", avatarResp.AvatarUrl)

		wantErr := errors.New("service failed")
		hErr := NewUserHandler(&fakeUserHandlerService{
			searchUserFn: func(_ context.Context, _ *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
				return nil, wantErr
			},
			updateProfileFn: func(_ context.Context, _ *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
				return nil, wantErr
			},
			uploadAvatarFn: func(_ context.Context, _ *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
				return nil, wantErr
			},
		})
		_, err1 := hErr.SearchUser(context.Background(), &pb.SearchUserRequest{})
		require.ErrorIs(t, err1, wantErr)
		_, err2 := hErr.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{})
		require.ErrorIs(t, err2, wantErr)
		_, err3 := hErr.UploadAvatar(context.Background(), &pb.UploadAvatarRequest{})
		require.ErrorIs(t, err3, wantErr)
	})

	t.Run("qrcode_batch_delete_success_and_error", func(t *testing.T) {
		wantErr := errors.New("service failed")
		h := NewUserHandler(&fakeUserHandlerService{
			getQRCodeFn: func(_ context.Context, _ *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
				return &pb.GetQRCodeResponse{Qrcode: "q"}, nil
			},
			parseQRCodeFn: func(_ context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
				return &pb.ParseQRCodeResponse{UserUuid: req.Token}, nil
			},
			batchGetProfileFn: func(_ context.Context, _ *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
				return &pb.BatchGetProfileResponse{}, nil
			},
		})

		qrResp, qrErr := h.GetQRCode(context.Background(), &pb.GetQRCodeRequest{})
		require.NoError(t, qrErr)
		require.NotNil(t, qrResp)
		assert.Equal(t, "q", qrResp.Qrcode)

		parseResp, parseErr := h.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{Token: "u1"})
		require.NoError(t, parseErr)
		require.NotNil(t, parseResp)
		assert.Equal(t, "u1", parseResp.UserUuid)

		batchResp, batchErr := h.BatchGetProfile(context.Background(), &pb.BatchGetProfileRequest{UserUuids: []string{"u1"}})
		require.NoError(t, batchErr)
		require.NotNil(t, batchResp)

		hErr := NewUserHandler(&fakeUserHandlerService{
			getQRCodeFn: func(_ context.Context, _ *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
				return nil, wantErr
			},
			parseQRCodeFn: func(_ context.Context, _ *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
				return nil, wantErr
			},
			batchGetProfileFn: func(_ context.Context, _ *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
				return nil, wantErr
			},
		})
		_, err1 := hErr.GetQRCode(context.Background(), &pb.GetQRCodeRequest{})
		require.ErrorIs(t, err1, wantErr)
		_, err2 := hErr.ParseQRCode(context.Background(), &pb.ParseQRCodeRequest{})
		require.ErrorIs(t, err2, wantErr)
		_, err3 := hErr.BatchGetProfile(context.Background(), &pb.BatchGetProfileRequest{})
		require.ErrorIs(t, err3, wantErr)
	})
}
