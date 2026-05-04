package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
)

// internalAuthServiceImpl 实现内部账号查询与同步逻辑。
type internalAuthServiceImpl struct {
	authRepo repository.IAuthRepository
}

// NewInternalAuthService 创建内部认证服务实例。
func NewInternalAuthService(authRepo repository.IAuthRepository) InternalAuthService {
	return &internalAuthServiceImpl{authRepo: authRepo}
}

// FindAccountByEmail 按邮箱查询账号。
func (s *internalAuthServiceImpl) FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error) {
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return &authpb.FindAccountByEmailResponse{Found: false}, nil
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "按邮箱查询账号失败")
	}

	return buildFindAccountByEmailResponse(user), nil
}

// FindAccountByTelephone 按手机号查询账号。
func (s *internalAuthServiceImpl) FindAccountByTelephone(ctx context.Context, req *authpb.FindAccountByTelephoneRequest) (*authpb.FindAccountByTelephoneResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "按手机号查询账号功能暂未实现")
}

// UpdateLoginDisplay 回写登录展示字段。
func (s *internalAuthServiceImpl) UpdateLoginDisplay(ctx context.Context, req *authpb.UpdateLoginDisplayRequest) (*authpb.UpdateLoginDisplayResponse, error) {
	if req == nil || req.UserUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	if err := s.authRepo.UpdateLoginDisplay(ctx, req.UserUuid, req.Nickname, req.Avatar); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新登录展示字段失败")
	}
	return &authpb.UpdateLoginDisplayResponse{}, nil
}

// BatchCheckAccountStatus 批量检查账号状态。
func (s *internalAuthServiceImpl) BatchCheckAccountStatus(ctx context.Context, req *authpb.BatchCheckAccountStatusRequest) (*authpb.BatchCheckAccountStatusResponse, error) {
	if req == nil || len(req.UserUuids) == 0 {
		return nil, apperr.New(consts.CodeParamError)
	}

	items, err := s.authRepo.BatchGetAccountStatus(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量查询账号状态失败")
	}

	respItems := make([]*authpb.AccountStatusItem, 0, len(items))
	for _, item := range items {
		if protoItem := buildAccountStatusItemProto(item); protoItem != nil {
			respItems = append(respItems, protoItem)
		}
	}
	return &authpb.BatchCheckAccountStatusResponse{Items: respItems}, nil
}
