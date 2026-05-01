package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// AuthHandler 负责承接对外认证 gRPC 请求。
type AuthHandler struct {
	authpb.UnimplementedAuthServiceServer

	authService service.IAuthService
}

// NewAuthHandler 创建认证 Handler 实例。
func NewAuthHandler(authService service.IAuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register 用户注册。
func (h *AuthHandler) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	return h.authService.Register(ctx, req)
}

// Login 用户密码登录。
func (h *AuthHandler) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	return h.authService.Login(ctx, req)
}

// LoginByCode 用户验证码登录。
func (h *AuthHandler) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	return h.authService.LoginByCode(ctx, req)
}

// SendVerifyCode 发送验证码。
func (h *AuthHandler) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	return h.authService.SendVerifyCode(ctx, req)
}

// VerifyCode 校验验证码。
func (h *AuthHandler) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	return h.authService.VerifyCode(ctx, req)
}

// RefreshToken 刷新 AccessToken。
func (h *AuthHandler) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	return h.authService.RefreshToken(ctx, req)
}

// Logout 退出当前设备登录态。
func (h *AuthHandler) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	return &authpb.LogoutResponse{}, h.authService.Logout(ctx, req)
}

// ResetPassword 通过验证码重置密码。
func (h *AuthHandler) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
	return &authpb.ResetPasswordResponse{}, h.authService.ResetPassword(ctx, req)
}
