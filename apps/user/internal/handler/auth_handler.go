package handler

import (
	"ChatServer/apps/user/internal/dto"
	"ChatServer/apps/user/internal/service"
	pb "ChatServer/apps/user/pb"
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
	return nil, nil
}

// Login 用户登录（密码）
// 遵循gRPC标准错误处理：
//   - 成功时返回(response, nil)
//   - 失败时返回(nil, status.Error(...))
func (h *AuthHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 1. 转换gRPC请求为DTO
	loginReq := &dto.LoginRequest{
		Telephone:  req.Account, // account 可以是手机号或邮箱
		Password:   req.Password,
		DeviceInfo: req.DeviceInfo,
	}

	// 2. 调用Service层执行登录业务流程
	loginResp, err := h.authService.Login(ctx, loginReq)
	if err != nil {
		return nil, err
	}

	// 3. 转换DTO为gRPC响应
	// 注意：Token 相关字段由 Gateway 层生成，这里暂时返回空值
	return &pb.LoginResponse{
		UserInfo: dto.ConvertToProtoUserInfo(loginResp.UserInfo),
	}, nil
}

// LoginByCode 验证码登录
func (h *AuthHandler) LoginByCode(ctx context.Context, req *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
	return nil, nil
}

// SendVerifyCode 发送验证码
func (h *AuthHandler) SendVerifyCode(ctx context.Context, req *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
	return nil, nil
}

// VerifyCode 校验验证码
func (h *AuthHandler) VerifyCode(ctx context.Context, req *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
	return nil, nil
}

// RefreshToken 刷新Token
func (h *AuthHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	return nil, nil
}

// Logout 用户登出
func (h *AuthHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	return nil, nil
}

// ResetPassword 重置密码
func (h *AuthHandler) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	return nil, nil
}
