package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

// accountServiceImpl 实现账号安全与生命周期相关逻辑。
type accountServiceImpl struct {
	authRepo   repository.IAuthRepository
	deviceRepo repository.IDeviceRepository
}

// NewAccountService 创建账号安全服务实例。
//
// 该服务负责密码、邮箱与账号生命周期等“账号域”动作：
//  1. 读取并校验 user_account 凭据状态；
//  2. 编排验证码、密码哈希与账号注销事件；
//  3. 在必要时联动 deviceRepo 清理设备登录态。
func NewAccountService(authRepo repository.IAuthRepository, deviceRepo repository.IDeviceRepository) AccountService {
	return &accountServiceImpl{authRepo: authRepo, deviceRepo: deviceRepo}
}

// ChangePassword 修改当前用户密码。
// 业务流程：
//  1. 从上下文中提取当前登录用户 UUID；
//  2. 查询账号并校验旧密码是否正确；
//  3. 拒绝把新密码设置成与旧密码相同；
//  4. 生成新的密码哈希并更新 user_account；
//  5. 保持与旧逻辑一致，暂不扩展“踢出其他设备”的后续动作。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.NotFound: 用户不存在
//   - codes.InvalidArgument: 原密码错误或新旧密码相同
//   - codes.Internal: 系统内部错误
func (s *accountServiceImpl) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) error {
	// 先确认当前请求已经绑定登录态，否则后续无法定位要修改哪一个账号。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 读取账号当前密码哈希，后续旧密码校验和新密码去重都依赖这份快照。
	user, err := s.authRepo.GetByUserUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return apperr.New(consts.CodeUserNotFound)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	// 先校验旧密码，避免未授权请求直接覆盖密码哈希。
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		return apperr.New(consts.CodePasswordError)
	}

	// 再阻止“新旧密码完全相同”的无意义更新，保持和旧单体一致的业务反馈。
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.NewPassword)); err == nil {
		return apperr.New(consts.CodePasswordSameAsOld)
	}

	// 只有全部校验通过后才生成新的 bcrypt 哈希，避免对无效请求做额外 CPU 消耗。
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}

	// 最终只更新 password_hash 字段；其他账号资料仍留在原表中保持不变。
	if err := s.authRepo.UpdatePassword(ctx, userUUID, string(hashedPassword)); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "更新密码失败")
	}

	// 保持与旧逻辑一致：暂不扩展“踢出其他设备”的历史 TODO。
	return nil
}

// ChangeEmail 换绑邮箱。
// 业务流程：
//  1. 从上下文中提取当前登录用户 UUID；
//  2. 校验新邮箱格式与换绑邮箱验证码；
//  3. 再次确认账号存在后更新邮箱字段；
//  4. 尽力删除已消费验证码，并返回新的邮箱信息。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.NotFound: 用户不存在
//   - codes.AlreadyExists: 邮箱已被占用
//   - codes.InvalidArgument: 验证码错误或已过期
//   - codes.Internal: 系统内部错误
func (s *accountServiceImpl) ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error) {
	// 换绑邮箱必须建立在已登录账号基础上，否则无法判断谁在执行换绑动作。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	newEmail := strings.TrimSpace(req.NewEmail)
	if !util.ValidateEmail(newEmail) {
		return nil, apperr.New(consts.CodeInvalidEmail)
	}

	// 换绑验证码按 type=4 存储；这里明确区分“验证码过期”和“验证码不匹配”。
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, newEmail, req.VerifyCode, 4)
	if err != nil {
		if errors.Is(err, repoerr.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}

	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	// 在真正更新前再次确认账号存在，避免使用过期登录态写入已删除账号。
	if _, err := s.authRepo.GetByUserUUID(ctx, userUUID); err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	// 数据库唯一索引是邮箱占用的最终裁决点；并发换绑冲突按业务冲突返回。
	if err := s.authRepo.UpdateEmail(ctx, userUUID, newEmail); err != nil {
		if errors.Is(err, repoerr.ErrDuplicateKey) {
			return nil, apperr.New(consts.CodeEmailAlreadyExist)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新邮箱失败")
	}

	// 验证码删除只做 best-effort，不因为清理失败而回滚已成功的换绑结果。
	if err := s.authRepo.DeleteVerifyCode(ctx, newEmail, 4); err != nil {
		logger.Warn(ctx, "删除验证码失败", logger.ErrorField("error", err))
	}

	return buildChangeEmailResponse(newEmail), nil
}

// ChangeTelephone 换绑手机号。
//
// 当前四拆阶段尚未把手机号绑定链路迁回 auth 域，因此继续显式返回“暂未实现”，避免
// 上游误以为该能力已经可用。
func (s *accountServiceImpl) ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "绑定/换绑手机功能暂未实现")
}

// DeleteAccount 注销账号并清理全部设备登录态。
// 业务流程：
//  1. 从上下文中提取当前登录用户 UUID；
//  2. 查询账号并校验注销密码；
//  3. 组装 account_deleted outbox 事件载荷；
//  4. 在事务内软删除账号并写入 outbox 事件；
//  5. 清理该用户的全部设备登录态，最后返回恢复截止时间。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.NotFound: 用户不存在
//   - codes.InvalidArgument: 密码错误
//   - codes.Internal: 系统内部错误
func (s *accountServiceImpl) DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error) {
	// 注销账号前必须确认当前调用方已经登录，否则无法定位注销对象。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 先取出账号快照，后续要依赖密码哈希做“本人确认”校验。
	user, err := s.authRepo.GetByUserUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	// 注销属于高风险动作，必须用当前密码再次确认身份。
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, apperr.New(consts.CodePasswordError)
	}

	// 账号注销后需要广播给 user/relation 做清理，因此在软删除事务里同时写入 outbox 事件。
	deleteAt := time.Now()
	eventID := util.GenIDString()

	// 事件里只保留跨服务清理所需的最小字段，避免把账号敏感信息继续外发。
	payload, err := accountevent.Encode(accountevent.AccountDeletedPayload{
		EventID:   eventID,
		UserUUID:  userUUID,
		DeletedAt: deleteAt,
	})

	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "序列化注销事件失败")
	}

	// 第一步：软删除 user_account，并在同一事务中写入 account_deleted 事件。
	if err := s.authRepo.DeleteWithOutboxEvent(ctx, userUUID, accountevent.EventTypeAccountDeleted, payload); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "注销账号失败")
	}

	// 第二步：清理所有设备登录态，避免已注销账号继续带着旧 token 访问系统。
	if err := s.deviceRepo.DeleteByUserUUID(ctx, userUUID); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "清理用户设备登录态失败")
	}

	// 恢复截止时间目前固定按 30 天计算，供上游直接展示给用户。
	recoverDeadline := deleteAt.Add(30 * 24 * time.Hour)
	return buildDeleteAccountResponse(deleteAt, recoverDeadline), nil
}
