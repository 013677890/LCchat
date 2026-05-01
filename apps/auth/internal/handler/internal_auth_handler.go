package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// InternalAuthHandler 负责承接内部账号查询与同步请求。
type InternalAuthHandler struct {
	authpb.UnimplementedInternalAuthServiceServer

	internalAuthService service.IInternalAuthService
}

// NewInternalAuthHandler 创建内部认证 Handler 实例。
func NewInternalAuthHandler(internalAuthService service.IInternalAuthService) *InternalAuthHandler {
	return &InternalAuthHandler{internalAuthService: internalAuthService}
}

// FindAccountByEmail 按邮箱查询账号。
func (h *InternalAuthHandler) FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error) {
	return h.internalAuthService.FindAccountByEmail(ctx, req)
}

// FindAccountByTelephone 按手机号查询账号。
func (h *InternalAuthHandler) FindAccountByTelephone(ctx context.Context, req *authpb.FindAccountByTelephoneRequest) (*authpb.FindAccountByTelephoneResponse, error) {
	return h.internalAuthService.FindAccountByTelephone(ctx, req)
}

// UpdateLoginDisplay 回写登录展示字段。
func (h *InternalAuthHandler) UpdateLoginDisplay(ctx context.Context, req *authpb.UpdateLoginDisplayRequest) (*authpb.UpdateLoginDisplayResponse, error) {
	return h.internalAuthService.UpdateLoginDisplay(ctx, req)
}

// BatchCheckAccountStatus 批量检查账号状态。
func (h *InternalAuthHandler) BatchCheckAccountStatus(ctx context.Context, req *authpb.BatchCheckAccountStatusRequest) (*authpb.BatchCheckAccountStatusResponse, error) {
	return h.internalAuthService.BatchCheckAccountStatus(ctx, req)
}
