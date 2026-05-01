package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// AccountHandler 负责承接账号安全与生命周期相关请求。
type AccountHandler struct {
	authpb.UnimplementedAccountServiceServer

	accountService service.IAccountService
}

// NewAccountHandler 创建账号 Handler 实例。
func NewAccountHandler(accountService service.IAccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

// ChangePassword 修改密码。
func (h *AccountHandler) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {
	return &authpb.ChangePasswordResponse{}, h.accountService.ChangePassword(ctx, req)
}

// ChangeEmail 换绑邮箱。
func (h *AccountHandler) ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error) {
	return h.accountService.ChangeEmail(ctx, req)
}

// ChangeTelephone 换绑手机号。
func (h *AccountHandler) ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error) {
	return h.accountService.ChangeTelephone(ctx, req)
}

// DeleteAccount 注销账号。
func (h *AccountHandler) DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error) {
	return h.accountService.DeleteAccount(ctx, req)
}
