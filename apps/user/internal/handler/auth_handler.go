package handler

import (
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"context"
)

// AuthHandler 认证服务Handler
type AuthHandler struct {
	pb.UnimplementedAuthServiceServer

	authService service.IAuthService
}

// NewAuthHandler 创建认证Handler实例
func NewAuthHandler(authService service.IAuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Register 用户注册
func (h *AuthHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return h.authService.Register(ctx, req)
}

// Login 用户登录（密码）
// 遵循gRPC标准错误处理：
//   - 成功时返回(response, nil)
//   - 失败时返回(nil, status.Error(...))
func (h *AuthHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 直接透传 Proto 到 Service 层
	return h.authService.Login(ctx, req)
}

// LoginByCode 验证码登录
func (h *AuthHandler) LoginByCode(ctx context.Context, req *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
	return h.authService.LoginByCode(ctx, req)
}

// SendVerifyCode 发送验证码
func (h *AuthHandler) SendVerifyCode(ctx context.Context, req *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
	return h.authService.SendVerifyCode(ctx, req)
}

// VerifyCode 校验验证码
func (h *AuthHandler) VerifyCode(ctx context.Context, req *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
	return h.authService.VerifyCode(ctx, req)
}

// RefreshToken 刷新Token
func (h *AuthHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	return h.authService.RefreshToken(ctx, req)
}

// Logout 用户登出
func (h *AuthHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	return &pb.LogoutResponse{}, h.authService.Logout(ctx, req)
}

// ResetPassword 重置密码
func (h *AuthHandler) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	return &pb.ResetPasswordResponse{}, h.authService.ResetPassword(ctx, req)
}
