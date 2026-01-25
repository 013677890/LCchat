package service

import (
	"ChatServer/apps/gateway/internal/dto"
	"context"
)

// AuthService 认证服务接口
// 职责：
//   - 调用下游用户服务进行认证
//   - 生成访问令牌（Access Token 和 Refresh Token）
type AuthService interface {
	// Login 用户登录
	// ctx: 请求上下文
	// req: 登录请求
	// deviceId: 设备唯一标识
	// 返回: 完整的登录响应（包含Token和用户信息）
	Login(ctx context.Context, req *dto.LoginRequest, deviceId string) (*dto.LoginResponse, error)
	// Register 用户注册
	// ctx: 请求上下文
	// req: 注册请求
	// 返回: 完整的注册响应（包含Token和用户信息）
	Register(ctx context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error)
	// SendVerifyCode 发送验证码
	// ctx: 请求上下文
	// req: 发送验证码请求
	// 返回: 发送验证码响应
	SendVerifyCode(ctx context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error)

	// LoginByCode 验证码登录
	// ctx: 请求上下文
	// req: 验证码登录请求
	// deviceId: 设备唯一标识
	// 返回: 完整的登录响应（包含Token和用户信息）
	LoginByCode(ctx context.Context, req *dto.LoginByCodeRequest, deviceId string) (*dto.LoginByCodeResponse, error)

	// Logout 用户登出
	// ctx: 请求上下文
	// req: 登出请求
	// 返回: 登出响应
	Logout(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error)

	// ResetPassword 重置密码
	// ctx: 请求上下文
	// req: 重置密码请求
	// 返回: 重置密码响应
	ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error)
}
