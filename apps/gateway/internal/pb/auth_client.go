package pb

import (
	"context"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// 这一组方法统一落在 auth-service 相关 client 上，
// 让 gateway facade 的拆分边界与下游领域职责保持一致。

// Login 用户登录。
func (c *userServiceClientImpl) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	return c.authClient.Login(ctx, req)
}

// LoginByCode 验证码登录。
func (c *userServiceClientImpl) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	return c.authClient.LoginByCode(ctx, req)
}

// SendVerifyCode 发送验证码。
func (c *userServiceClientImpl) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	return c.authClient.SendVerifyCode(ctx, req)
}

// VerifyCode 校验验证码。
func (c *userServiceClientImpl) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	return c.authClient.VerifyCode(ctx, req)
}

// Register 用户注册。
func (c *userServiceClientImpl) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	return c.authClient.Register(ctx, req)
}

// RefreshToken 刷新 Token。
func (c *userServiceClientImpl) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	return c.authClient.RefreshToken(ctx, req)
}

// Logout 用户登出。
func (c *userServiceClientImpl) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	return c.authClient.Logout(ctx, req)
}

// ResetPassword 重置密码。
func (c *userServiceClientImpl) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
	return c.authClient.ResetPassword(ctx, req)
}

// ChangePassword 修改密码。
func (c *userServiceClientImpl) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {
	return c.accountClient.ChangePassword(ctx, req)
}

// ChangeEmail 绑定或换绑邮箱。
func (c *userServiceClientImpl) ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error) {
	return c.accountClient.ChangeEmail(ctx, req)
}

// ChangeTelephone 绑定或换绑手机。
func (c *userServiceClientImpl) ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error) {
	return c.accountClient.ChangeTelephone(ctx, req)
}

// DeleteAccount 注销账号。
func (c *userServiceClientImpl) DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error) {
	return c.accountClient.DeleteAccount(ctx, req)
}

// FindAccountByEmail 按邮箱查找账号。
func (c *userServiceClientImpl) FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error) {
	return c.internalAuth.FindAccountByEmail(ctx, req)
}
