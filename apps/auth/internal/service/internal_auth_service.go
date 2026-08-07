package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
)

// internalAuthServiceImpl 实现内部账号查询与同步逻辑。
type internalAuthServiceImpl struct {
	authRepo repository.IAuthRepository
}

// NewInternalAuthService 创建内部认证服务实例。
//
// internal-auth 只面向其他服务暴露最小账号能力：
//  1. 按邮箱查询账号存在性与状态；
//  2. 回写登录展示字段；
//  3. 批量查询账号状态供 user/relation/msg 做边界判断。
func NewInternalAuthService(authRepo repository.IAuthRepository) InternalAuthService {
	return &internalAuthServiceImpl{authRepo: authRepo}
}

// FindAccountByEmail 按邮箱查询账号。
// 业务流程：
//  1. 按邮箱查询 user_account；
//  2. 若账号不存在，则返回 Found=false 而不是业务错误；
//  3. 若存在，则仅返回内部调用所需的 user_uuid 与状态字段。
//
// 错误码映射：
//   - codes.Internal: 系统内部错误
func (s *internalAuthServiceImpl) FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error) {
	// internal 接口把“账号不存在”视为普通查询结果，而不是业务异常，方便上游直接分支处理。
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return &authpb.FindAccountByEmailResponse{Found: false}, nil
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "按邮箱查询账号失败")
	}

	// 命中账号后只返回内部依赖的最小字段集合，避免把敏感信息透传到其他服务。
	return buildFindAccountByEmailResponse(user), nil
}

// FindAccountByTelephone 按手机号查询账号。
//
// 当前手机号账号链路仍未恢复，因此内部调用统一返回 MethodNotAllowed，避免服务侧误接入半成品能力。
func (s *internalAuthServiceImpl) FindAccountByTelephone(ctx context.Context, req *authpb.FindAccountByTelephoneRequest) (*authpb.FindAccountByTelephoneResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "按手机号查询账号功能暂未实现")
}

// UpdateLoginDisplay 回写登录展示字段。
// 业务流程：
//  1. 校验请求中的 user_uuid；
//  2. 调用仓储层更新 login_nickname / login_avatar；
//  3. 依赖仓储层同时失效登录态相关缓存；
//  4. 返回空响应表示回写完成。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *internalAuthServiceImpl) UpdateLoginDisplay(ctx context.Context, req *authpb.UpdateLoginDisplayRequest) (*authpb.UpdateLoginDisplayResponse, error) {
	// 先挡住空 user_uuid，避免把全表更新风险下沉到 repository 层。
	if req == nil || req.UserUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	// 这里只回写 login_nickname / login_avatar 两个冗余字段，供登录态最小展示复用。
	if err := s.authRepo.UpdateLoginDisplay(ctx, req.UserUuid, req.Nickname, req.Avatar); err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return &authpb.UpdateLoginDisplayResponse{}, nil
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新登录展示字段失败")
	}
	return &authpb.UpdateLoginDisplayResponse{}, nil
}

// BatchCheckAccountStatus 批量检查账号状态。
// 业务流程：
//  1. 校验 user_uuids 非空；
//  2. 批量查询账号是否存在以及状态；
//  3. 将仓储层结果按内部 proto 结构重组；
//  4. 返回给调用方做批量边界判断。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *internalAuthServiceImpl) BatchCheckAccountStatus(ctx context.Context, req *authpb.BatchCheckAccountStatusRequest) (*authpb.BatchCheckAccountStatusResponse, error) {
	// 批量内部查询要求至少传一个 user_uuid，避免无意义的空批请求继续下沉。
	if req == nil || len(req.UserUuids) == 0 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 先从仓储层拿到“是否存在 + 账号状态”的中间结构，后续再统一转成 proto。
	items, err := s.authRepo.BatchGetAccountStatus(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量查询账号状态失败")
	}

	respItems := make([]*authpb.AccountStatusItem, 0, len(items))
	for _, item := range items {
		// 过滤 nil 并完成仓储模型到内部 proto 的最后一跳转换。
		if protoItem := buildAccountStatusItemProto(item); protoItem != nil {
			respItems = append(respItems, protoItem)
		}
	}
	return &authpb.BatchCheckAccountStatusResponse{Items: respItems}, nil
}
