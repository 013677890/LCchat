package service

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

// accountServiceImpl 实现账号安全与生命周期相关逻辑。
type accountServiceImpl struct {
	authRepo   repository.IAuthRepository
	deviceRepo repository.IDeviceRepository
}

// NewAccountService 创建账号安全服务实例。
func NewAccountService(authRepo repository.IAuthRepository, deviceRepo repository.IDeviceRepository) AccountService {
	return &accountServiceImpl{authRepo: authRepo, deviceRepo: deviceRepo}
}

// ChangePassword 修改当前用户密码。
func (s *accountServiceImpl) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) error {
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	user, err := s.authRepo.GetByUserUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeUserNotFound)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return apperr.New(consts.CodePasswordError)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.NewPassword)); err == nil {
		return apperr.New(consts.CodePasswordSameAsOld)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}
	if err := s.authRepo.UpdatePassword(ctx, userUUID, string(hashedPassword)); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "更新密码失败")
	}

	// 保持与旧逻辑一致：暂不扩展“踢出其他设备”的历史 TODO。
	return nil
}

// ChangeEmail 换绑邮箱。
func (s *accountServiceImpl) ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error) {
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	exists, err := s.authRepo.ExistsByEmail(ctx, req.NewEmail)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查邮箱是否存在失败")
	}
	if exists {
		return nil, apperr.New(consts.CodeEmailAlreadyExist)
	}

	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.NewEmail, req.VerifyCode, 4)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	if _, err := s.authRepo.GetByUserUUID(ctx, userUUID); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	if err := s.authRepo.UpdateEmail(ctx, userUUID, req.NewEmail); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新邮箱失败")
	}
	if err := s.authRepo.DeleteVerifyCode(ctx, req.NewEmail, 4); err != nil {
		logger.Warn(ctx, "删除验证码失败", logger.ErrorField("error", err))
	}

	return &authpb.ChangeEmailResponse{Email: req.NewEmail}, nil
}

// ChangeTelephone 换绑手机号。
func (s *accountServiceImpl) ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "绑定/换绑手机功能暂未实现")
}

// DeleteAccount 注销账号并清理全部设备登录态。
func (s *accountServiceImpl) DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error) {
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	user, err := s.authRepo.GetByUserUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, apperr.New(consts.CodePasswordError)
	}

	// 账号注销后需要广播给 user/relation 做清理，因此在软删除事务里同时写入 outbox 事件。
	deleteAt := time.Now()
	payload, err := accountevent.Encode(accountevent.AccountDeletedPayload{
		UserUUID:  userUUID,
		DeletedAt: deleteAt,
	})
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "序列化注销事件失败")
	}

	if err := s.authRepo.DeleteWithOutboxEvent(ctx, userUUID, accountevent.EventTypeAccountDeleted, payload); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "注销账号失败")
	}
	if err := s.deviceRepo.DeleteByUserUUID(ctx, userUUID); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "清理用户设备登录态失败")
	}

	recoverDeadline := deleteAt.Add(30 * 24 * time.Hour)
	return &authpb.DeleteAccountResponse{
		DeleteAt:        deleteAt.Format(time.RFC3339),
		RecoverDeadline: recoverDeadline.Format(time.RFC3339),
	}, nil
}
