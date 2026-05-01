package service

import (
	"context"
	"errors"
	"testing"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGatewayAuthUserClient struct {
	gatewaypb.UserServiceClient

	loginFn          func(context.Context, *authpb.LoginRequest) (*authpb.LoginResponse, error)
	registerFn       func(context.Context, *authpb.RegisterRequest) (*authpb.RegisterResponse, error)
	sendVerifyCodeFn func(context.Context, *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error)
	loginByCodeFn    func(context.Context, *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error)
	logoutFn         func(context.Context, *authpb.LogoutRequest) (*authpb.LogoutResponse, error)
	resetPasswordFn  func(context.Context, *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error)
	refreshTokenFn   func(context.Context, *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error)
	verifyCodeFn     func(context.Context, *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error)
}

func (f *fakeGatewayAuthUserClient) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	if f.loginFn == nil {
		return nil, errors.New("unexpected Login call")
	}
	return f.loginFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	if f.registerFn == nil {
		return nil, errors.New("unexpected Register call")
	}
	return f.registerFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	if f.sendVerifyCodeFn == nil {
		return nil, errors.New("unexpected SendVerifyCode call")
	}
	return f.sendVerifyCodeFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	if f.loginByCodeFn == nil {
		return nil, errors.New("unexpected LoginByCode call")
	}
	return f.loginByCodeFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	if f.logoutFn == nil {
		return nil, errors.New("unexpected Logout call")
	}
	return f.logoutFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
	if f.resetPasswordFn == nil {
		return nil, errors.New("unexpected ResetPassword call")
	}
	return f.resetPasswordFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	if f.refreshTokenFn == nil {
		return nil, errors.New("unexpected RefreshToken call")
	}
	return f.refreshTokenFn(ctx, req)
}

func (f *fakeGatewayAuthUserClient) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	if f.verifyCodeFn == nil {
		return nil, errors.New("unexpected VerifyCode call")
	}
	return f.verifyCodeFn(ctx, req)
}

func TestGatewayAuthServiceLogin(t *testing.T) {
	t.Run("success_with_mapping", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			loginFn: func(_ context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
				require.Equal(t, "acc", req.Account)
				require.Equal(t, "pass123", req.Password)
				require.NotNil(t, req.DeviceInfo)
				require.Equal(t, "ios", req.DeviceInfo.Platform)
				return &authpb.LoginResponse{
					AccessToken:  "atk",
					RefreshToken: "rtk",
					TokenType:    "Bearer",
					ExpiresIn:    7200,
					UserInfo:     &authpb.LoginUserInfo{Uuid: "u1", Nickname: "n1"},
				}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Login(context.Background(), &dto.LoginRequest{
			Account:  "acc",
			Password: "pass123",
			DeviceInfo: &dto.DeviceInfo{
				Platform: "ios",
			},
		}, "d1")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "atk", resp.AccessToken)
		assert.Equal(t, "rtk", resp.RefreshToken)
		assert.Equal(t, "u1", resp.UserInfo.UUID)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			loginFn: func(_ context.Context, _ *authpb.LoginRequest) (*authpb.LoginResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Login(context.Background(), &dto.LoginRequest{
			Account:  "acc",
			Password: "pass123",
		}, "d1")
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("nil_user_info_returns_internal_code", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			loginFn: func(_ context.Context, _ *authpb.LoginRequest) (*authpb.LoginResponse, error) {
				return &authpb.LoginResponse{
					AccessToken: "atk",
				}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Login(context.Background(), &dto.LoginRequest{
			Account:  "acc",
			Password: "pass123",
		}, "d1")
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})
}

func TestGatewayAuthServiceRegister(t *testing.T) {
	t.Run("success_with_mapping", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			registerFn: func(_ context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				require.Equal(t, "pass123", req.Password)
				require.Equal(t, "123456", req.VerifyCode)
				require.Equal(t, "n1", req.Nickname)
				require.Equal(t, "13800138000", req.Telephone)
				return &authpb.RegisterResponse{
					UserUuid:  "u1",
					Email:     req.Email,
					Telephone: req.Telephone,
					Nickname:  req.Nickname,
				}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Register(context.Background(), &dto.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
			Nickname:   "n1",
			Telephone:  "13800138000",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "u1", resp.UserUUID)
		assert.Equal(t, "n1", resp.Nickname)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			registerFn: func(_ context.Context, _ *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Register(context.Background(), &dto.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("empty_user_uuid_returns_internal_code", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			registerFn: func(_ context.Context, _ *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
				return &authpb.RegisterResponse{}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Register(context.Background(), &dto.RegisterRequest{
			Email:      "a@test.com",
			Password:   "pass123",
			VerifyCode: "123456",
		})
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})
}

func TestGatewayAuthServiceSendVerifyCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			sendVerifyCodeFn: func(_ context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				require.Equal(t, int32(2), req.Type)
				return &authpb.SendVerifyCodeResponse{ExpireSeconds: 120}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.SendVerifyCode(context.Background(), &dto.SendVerifyCodeRequest{
			Email: "a@test.com",
			Type:  2,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(120), resp.ExpireSeconds)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			sendVerifyCodeFn: func(_ context.Context, _ *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.SendVerifyCode(context.Background(), &dto.SendVerifyCodeRequest{
			Email: "a@test.com",
			Type:  2,
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayAuthServiceLoginByCode(t *testing.T) {
	t.Run("success_with_mapping", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			loginByCodeFn: func(_ context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				require.Equal(t, "123456", req.VerifyCode)
				require.NotNil(t, req.DeviceInfo)
				require.Equal(t, "android", req.DeviceInfo.Platform)
				return &authpb.LoginByCodeResponse{
					AccessToken:  "atk",
					RefreshToken: "rtk",
					TokenType:    "Bearer",
					ExpiresIn:    7200,
					UserInfo: &authpb.LoginUserInfo{
						Uuid:     "u2",
						Nickname: "n2",
					},
				}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.LoginByCode(context.Background(), &dto.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			DeviceInfo: &dto.DeviceInfo{
				Platform: "android",
			},
		}, "d2")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "u2", resp.UserInfo.UUID)
		assert.Equal(t, "atk", resp.AccessToken)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			loginByCodeFn: func(_ context.Context, _ *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.LoginByCode(context.Background(), &dto.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		}, "d2")
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("nil_user_info_returns_internal_code", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			loginByCodeFn: func(_ context.Context, _ *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
				return &authpb.LoginByCodeResponse{
					AccessToken: "atk",
				}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.LoginByCode(context.Background(), &dto.LoginByCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
		}, "d2")
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})
}

func TestGatewayAuthServiceLogout(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			logoutFn: func(_ context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
				require.Equal(t, "d1", req.DeviceId)
				return &authpb.LogoutResponse{}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Logout(context.Background(), &dto.LogoutRequest{DeviceID: "d1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			logoutFn: func(_ context.Context, _ *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.Logout(context.Background(), &dto.LogoutRequest{DeviceID: "d1"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayAuthServiceResetPassword(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			resetPasswordFn: func(_ context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				require.Equal(t, "123456", req.VerifyCode)
				require.Equal(t, "pass999", req.NewPassword)
				return &authpb.ResetPasswordResponse{}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.ResetPassword(context.Background(), &dto.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "pass999",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			resetPasswordFn: func(_ context.Context, _ *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.ResetPassword(context.Background(), &dto.ResetPasswordRequest{
			Email:       "a@test.com",
			VerifyCode:  "123456",
			NewPassword: "pass999",
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayAuthServiceRefreshToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			refreshTokenFn: func(_ context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
				require.Equal(t, "rtk", req.RefreshToken)
				return &authpb.RefreshTokenResponse{
					AccessToken: "atk2",
					TokenType:   "Bearer",
					ExpiresIn:   7200,
				}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
			UserUUID:     "u1",
			DeviceID:     "d1",
			RefreshToken: "rtk",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "atk2", resp.AccessToken)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			refreshTokenFn: func(_ context.Context, _ *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
			UserUUID:     "u1",
			DeviceID:     "d1",
			RefreshToken: "rtk",
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayAuthServiceVerifyCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayAuthUserClient{
			verifyCodeFn: func(_ context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
				require.Equal(t, "a@test.com", req.Email)
				require.Equal(t, "123456", req.VerifyCode)
				require.Equal(t, int32(2), req.Type)
				return &authpb.VerifyCodeResponse{Valid: true}, nil
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.VerifyCode(context.Background(), &dto.VerifyCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			Type:       2,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Valid)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayAuthUserClient{
			verifyCodeFn: func(_ context.Context, _ *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewAuthService(client)

		resp, err := svc.VerifyCode(context.Background(), &dto.VerifyCodeRequest{
			Email:      "a@test.com",
			VerifyCode: "123456",
			Type:       2,
		})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}
