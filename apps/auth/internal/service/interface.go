package service

import (
	"context"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// IAuthService 定义认证服务的核心业务能力。
type IAuthService interface {
	// Register 用户注册。
	Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error)
	// Login 用户密码登录。
	Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error)
	// LoginByCode 用户验证码登录。
	LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error)
	// SendVerifyCode 发送验证码。
	SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error)
	// VerifyCode 校验验证码。
	VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error)
	// RefreshToken 刷新 AccessToken。
	RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error)
	// Logout 退出当前设备登录态。
	Logout(ctx context.Context, req *authpb.LogoutRequest) error
	// ResetPassword 通过验证码重置密码。
	ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) error
}

// IDeviceService 定义设备会话与在线状态相关业务能力。
type IDeviceService interface {
	// GetDeviceList 获取当前用户设备列表。
	GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error)
	// KickDevice 踢出指定设备。
	KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) error
	// GetOnlineStatus 获取单用户在线状态。
	GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error)
	// BatchGetOnlineStatus 批量获取在线状态。
	BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error)
	// UpdateDeviceStatus 更新设备在线状态。
	UpdateDeviceStatus(ctx context.Context, req *authpb.UpdateDeviceStatusRequest) error
}

// IAccountService 定义账号安全与生命周期相关业务能力。
type IAccountService interface {
	// ChangePassword 修改密码。
	ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) error
	// ChangeEmail 换绑邮箱。
	ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error)
	// ChangeTelephone 换绑手机号。
	ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error)
	// DeleteAccount 注销账号。
	DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error)
}

// IInternalAuthService 定义内部账号查询与同步能力。
type IInternalAuthService interface {
	// FindAccountByEmail 按邮箱查询账号。
	FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error)
	// FindAccountByTelephone 按手机号查询账号。
	FindAccountByTelephone(ctx context.Context, req *authpb.FindAccountByTelephoneRequest) (*authpb.FindAccountByTelephoneResponse, error)
	// UpdateLoginDisplay 回写登录展示字段。
	UpdateLoginDisplay(ctx context.Context, req *authpb.UpdateLoginDisplayRequest) (*authpb.UpdateLoginDisplayResponse, error)
	// BatchCheckAccountStatus 批量检查账号状态。
	BatchCheckAccountStatus(ctx context.Context, req *authpb.BatchCheckAccountStatusRequest) (*authpb.BatchCheckAccountStatusResponse, error)
}

// AuthService 是 IAuthService 的别名。
type AuthService = IAuthService

// DeviceService 是 IDeviceService 的别名。
type DeviceService = IDeviceService

// AccountService 是 IAccountService 的别名。
type AccountService = IAccountService

// InternalAuthService 是 IInternalAuthService 的别名。
type InternalAuthService = IInternalAuthService
