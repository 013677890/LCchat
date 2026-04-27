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

type fakeAuthHandlerService struct {
	registerFn       func(context.Context, *pb.RegisterRequest) (*pb.RegisterResponse, error)
	loginFn          func(context.Context, *pb.LoginRequest) (*pb.LoginResponse, error)
	loginByCodeFn    func(context.Context, *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error)
	sendVerifyCodeFn func(context.Context, *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error)
	verifyCodeFn     func(context.Context, *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error)
	refreshTokenFn   func(context.Context, *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error)
	logoutFn         func(context.Context, *pb.LogoutRequest) error
	resetPasswordFn  func(context.Context, *pb.ResetPasswordRequest) error
}

var _ service.IAuthService = (*fakeAuthHandlerService)(nil)

func (f *fakeAuthHandlerService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if f.registerFn == nil {
		return &pb.RegisterResponse{}, nil
	}
	return f.registerFn(ctx, req)
}

func (f *fakeAuthHandlerService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	if f.loginFn == nil {
		return &pb.LoginResponse{}, nil
	}
	return f.loginFn(ctx, req)
}

func (f *fakeAuthHandlerService) LoginByCode(ctx context.Context, req *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
	if f.loginByCodeFn == nil {
		return &pb.LoginByCodeResponse{}, nil
	}
	return f.loginByCodeFn(ctx, req)
}

func (f *fakeAuthHandlerService) SendVerifyCode(ctx context.Context, req *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
	if f.sendVerifyCodeFn == nil {
		return &pb.SendVerifyCodeResponse{}, nil
	}
	return f.sendVerifyCodeFn(ctx, req)
}

func (f *fakeAuthHandlerService) VerifyCode(ctx context.Context, req *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
	if f.verifyCodeFn == nil {
		return &pb.VerifyCodeResponse{}, nil
	}
	return f.verifyCodeFn(ctx, req)
}

func (f *fakeAuthHandlerService) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	if f.refreshTokenFn == nil {
		return &pb.RefreshTokenResponse{}, nil
	}
	return f.refreshTokenFn(ctx, req)
}

func (f *fakeAuthHandlerService) Logout(ctx context.Context, req *pb.LogoutRequest) error {
	if f.logoutFn == nil {
		return nil
	}
	return f.logoutFn(ctx, req)
}

func (f *fakeAuthHandlerService) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) error {
	if f.resetPasswordFn == nil {
		return nil
	}
	return f.resetPasswordFn(ctx, req)
}

func TestUserAuthHandlerRegister(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.RegisterResponse{UserUuid: "u1"}
		h := NewAuthHandler(&fakeAuthHandlerService{
			registerFn: func(_ context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				return want, nil
			},
		})

		resp, err := h.Register(context.Background(), &pb.RegisterRequest{Email: "a@test.com"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("register failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			registerFn: func(_ context.Context, _ *pb.RegisterRequest) (*pb.RegisterResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := h.Register(context.Background(), &pb.RegisterRequest{Email: "a@test.com"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserAuthHandlerLogin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.LoginResponse{AccessToken: "atk"}
		h := NewAuthHandler(&fakeAuthHandlerService{
			loginFn: func(_ context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
				require.Equal(t, "acc", req.Account)
				return want, nil
			},
		})

		resp, err := h.Login(context.Background(), &pb.LoginRequest{Account: "acc"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("login failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			loginFn: func(_ context.Context, _ *pb.LoginRequest) (*pb.LoginResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := h.Login(context.Background(), &pb.LoginRequest{Account: "acc"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserAuthHandlerLoginByCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.LoginByCodeResponse{AccessToken: "atk"}
		h := NewAuthHandler(&fakeAuthHandlerService{
			loginByCodeFn: func(_ context.Context, req *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				return want, nil
			},
		})

		resp, err := h.LoginByCode(context.Background(), &pb.LoginByCodeRequest{Email: "a@test.com"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("login by code failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			loginByCodeFn: func(_ context.Context, _ *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := h.LoginByCode(context.Background(), &pb.LoginByCodeRequest{Email: "a@test.com"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserAuthHandlerSendVerifyCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.SendVerifyCodeResponse{ExpireSeconds: 120}
		h := NewAuthHandler(&fakeAuthHandlerService{
			sendVerifyCodeFn: func(_ context.Context, req *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				return want, nil
			},
		})

		resp, err := h.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{Email: "a@test.com"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("send verify code failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			sendVerifyCodeFn: func(_ context.Context, _ *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := h.SendVerifyCode(context.Background(), &pb.SendVerifyCodeRequest{Email: "a@test.com"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserAuthHandlerVerifyCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.VerifyCodeResponse{Valid: true}
		h := NewAuthHandler(&fakeAuthHandlerService{
			verifyCodeFn: func(_ context.Context, req *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				return want, nil
			},
		})

		resp, err := h.VerifyCode(context.Background(), &pb.VerifyCodeRequest{Email: "a@test.com"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("verify code failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			verifyCodeFn: func(_ context.Context, _ *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := h.VerifyCode(context.Background(), &pb.VerifyCodeRequest{Email: "a@test.com"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserAuthHandlerRefreshToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.RefreshTokenResponse{AccessToken: "new-token"}
		h := NewAuthHandler(&fakeAuthHandlerService{
			refreshTokenFn: func(_ context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
				require.Equal(t, "rtk", req.RefreshToken)
				return want, nil
			},
		})

		resp, err := h.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("refresh token failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			refreshTokenFn: func(_ context.Context, _ *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := h.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: "rtk"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserAuthHandlerLogout(t *testing.T) {
	t.Run("success_empty_response_contract", func(t *testing.T) {
		h := NewAuthHandler(&fakeAuthHandlerService{
			logoutFn: func(_ context.Context, req *pb.LogoutRequest) error {
				require.Equal(t, "d1", req.DeviceId)
				return nil
			},
		})

		resp, err := h.Logout(context.Background(), &pb.LogoutRequest{DeviceId: "d1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.LogoutResponse{}, resp)
	})

	t.Run("error_empty_response_contract", func(t *testing.T) {
		wantErr := errors.New("logout failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			logoutFn: func(_ context.Context, _ *pb.LogoutRequest) error {
				return wantErr
			},
		})

		resp, err := h.Logout(context.Background(), &pb.LogoutRequest{DeviceId: "d1"})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.LogoutResponse{}, resp)
	})
}

func TestUserAuthHandlerResetPassword(t *testing.T) {
	t.Run("success_empty_response_contract", func(t *testing.T) {
		h := NewAuthHandler(&fakeAuthHandlerService{
			resetPasswordFn: func(_ context.Context, req *pb.ResetPasswordRequest) error {
				require.Equal(t, "a@test.com", req.Email)
				return nil
			},
		})

		resp, err := h.ResetPassword(context.Background(), &pb.ResetPasswordRequest{Email: "a@test.com"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.ResetPasswordResponse{}, resp)
	})

	t.Run("error_empty_response_contract", func(t *testing.T) {
		wantErr := errors.New("reset failed")
		h := NewAuthHandler(&fakeAuthHandlerService{
			resetPasswordFn: func(_ context.Context, _ *pb.ResetPasswordRequest) error {
				return wantErr
			},
		})

		resp, err := h.ResetPassword(context.Background(), &pb.ResetPasswordRequest{Email: "a@test.com"})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.ResetPasswordResponse{}, resp)
	})
}
