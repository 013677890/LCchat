package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

// authServiceImpl 实现认证服务的核心登录注册流程。
type authServiceImpl struct {
	authRepo   repository.IAuthRepository
	deviceRepo repository.IDeviceRepository
}

// NewAuthService 创建认证服务实例。
func NewAuthService(authRepo repository.IAuthRepository, deviceRepo repository.IDeviceRepository) AuthService {
	return &authServiceImpl{
		authRepo:   authRepo,
		deviceRepo: deviceRepo,
	}
}

// Register 用户注册。
func (s *authServiceImpl) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	// 先校验注册验证码，避免无效请求占用数据库写资源。
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 1)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeError)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}

	// 沿用当前 user_info 模型完成账号创建，避免在服务拆分阶段额外引入表迁移风险。
	user := &model.UserInfo{
		Uuid:      util.GenIDString(),
		Email:     req.Email,
		Password:  string(hashedPassword),
		Nickname:  req.Nickname,
		Telephone: strings.TrimSpace(req.Telephone),
		Status:    0,
		IsAdmin:   0,
	}

	// 注册成功后需要异步打通资料初始化闭环，因此在同一事务里写入 user_created outbox 事件。
	eventID := util.GenIDString()
	payload, err := accountevent.Encode(accountevent.UserCreatedPayload{
		EventID:  eventID,
		UserUUID: user.Uuid,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	})
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "序列化注册事件失败")
	}

	createdUser, err := s.authRepo.CreateWithOutboxEvent(ctx, user, accountevent.EventTypeUserCreated, payload)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateKey) {
			return nil, apperr.New(consts.CodeUserAlreadyExist)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建用户失败")
	}

	return &authpb.RegisterResponse{
		UserUuid:  createdUser.Uuid,
		Nickname:  createdUser.Nickname,
		Email:     createdUser.Email,
		Telephone: createdUser.Telephone,
	}, nil
}

// Login 用户密码登录。
func (s *authServiceImpl) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	if req.DeviceInfo == nil {
		req.DeviceInfo = &authpb.DeviceInfo{DeviceName: "Unknown", Platform: "Unknown"}
	}

	user, err := s.authRepo.GetByEmail(ctx, req.Account)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}
	if user.Status == 1 {
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	ctx = ctxmeta.WithUserUUID(ctx, user.Uuid)
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, apperr.New(consts.CodePasswordError)
	}

	deviceID, err := getRequiredDeviceID(ctx)
	if err != nil {
		return nil, err
	}
	clientIP := util.GetClientIPFromContext(ctx)

	accessToken, err := util.GenerateToken(user.Uuid, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成访问令牌失败")
	}
	refreshToken := util.GenIDString()

	// 登录成功后优先写入 token，再尽力补充设备会话与在线态信息。
	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 AccessToken 失败")
	}
	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 RefreshToken 失败")
	}

	deviceSession := &model.DeviceSession{
		UserUuid:   user.Uuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  buildDeviceUserAgent(req.DeviceInfo),
		Status:     model.DeviceStatusOnline,
	}
	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Warn(ctx, "设备会话落库失败，按降级处理继续登录", logger.ErrorField("error", err))
	}
	if err := s.deviceRepo.SetActiveTimestamp(ctx, user.Uuid, deviceID, time.Now().Unix()); err != nil {
		logger.Warn(ctx, "写入设备活跃时间失败",
			logger.String("user_uuid", user.Uuid),
			logger.String("device_id", deviceID),
			logger.ErrorField("error", err),
		)
	}

	return &authpb.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(util.AccessExpire.Seconds()),
		UserInfo:     buildLoginUserInfo(user),
	}, nil
}

// LoginByCode 用户验证码登录。
func (s *authServiceImpl) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	if req.DeviceInfo == nil {
		req.DeviceInfo = &authpb.DeviceInfo{DeviceName: "Unknown", Platform: "Unknown"}
	}

	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}
	if user.Status == 1 {
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	deviceID, err := getRequiredDeviceID(ctx)
	if err != nil {
		return nil, err
	}

	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 2)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 2); err != nil {
		logger.Warn(ctx, "删除验证码失败", logger.ErrorField("error", err))
	}

	ctx = ctxmeta.WithUserUUID(ctx, user.Uuid)
	clientIP := util.GetClientIPFromContext(ctx)
	accessToken, err := util.GenerateToken(user.Uuid, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成访问令牌失败")
	}
	refreshToken := util.GenIDString()

	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 AccessToken 失败")
	}
	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 RefreshToken 失败")
	}

	deviceSession := &model.DeviceSession{
		UserUuid:   user.Uuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  buildDeviceUserAgent(req.DeviceInfo),
		Status:     model.DeviceStatusOnline,
	}
	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Warn(ctx, "设备会话落库失败，按降级处理继续登录", logger.ErrorField("error", err))
	}
	if err := s.deviceRepo.SetActiveTimestamp(ctx, user.Uuid, deviceID, time.Now().Unix()); err != nil {
		logger.Warn(ctx, "写入设备活跃时间失败",
			logger.String("user_uuid", user.Uuid),
			logger.String("device_id", deviceID),
			logger.ErrorField("error", err),
		)
	}

	return &authpb.LoginByCodeResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(util.AccessExpire.Seconds()),
		UserInfo:     buildLoginUserInfo(user),
	}, nil
}

// SendVerifyCode 发送验证码。
func (s *authServiceImpl) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	if !util.ValidateEmail(req.Email) {
		return nil, apperr.New(consts.CodeInvalidEmail)
	}

	ip := util.GetClientIPFromContext(ctx)
	isLimited, err := s.authRepo.VerifyVerifyCodeRateLimit(ctx, req.Email, ip)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "验证码限流检查失败")
	}
	if isLimited {
		return nil, apperr.New(consts.CodeSendTooFrequent)
	}

	code, err := util.GenerateVerifyCode(6)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成验证码失败")
	}
	if err := s.authRepo.StoreVerifyCode(ctx, req.Email, code, req.Type, 2*time.Minute); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "存储验证码失败")
	}
	if err := s.authRepo.IncrementVerifyCodeCount(ctx, req.Email, ip); err != nil {
		logger.Warn(ctx, "递增验证码计数失败", logger.ErrorField("error", err))
	}
	if err := util.SendVerifyCodeEmail(req.Email, code, 2); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "发送验证码邮件失败")
	}

	return &authpb.SendVerifyCodeResponse{ExpireSeconds: 120}, nil
}

// VerifyCode 校验验证码。
func (s *authServiceImpl) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, req.Type)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	return &authpb.VerifyCodeResponse{Valid: isValid}, nil
}

// RefreshToken 刷新 AccessToken。
func (s *authServiceImpl) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeInvalidToken)
	}
	deviceID := util.GetDeviceIDFromContext(ctx)
	if deviceID == "" {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	storedRefreshToken, err := s.deviceRepo.GetRefreshToken(ctx, userUUID, deviceID)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeDeviceNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取 Refresh Token 失败")
	}
	if storedRefreshToken != req.RefreshToken {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	newAccessToken, err := util.GenerateToken(userUUID, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成 Access Token 失败")
	}
	if err := s.deviceRepo.StoreAccessToken(ctx, userUUID, deviceID, newAccessToken, util.AccessExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新 Access Token 失败")
	}
	if err := s.deviceRepo.TouchDeviceInfoTTL(ctx, userUUID); err != nil {
		logger.Warn(ctx, "续期设备信息缓存失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
	}

	return &authpb.RefreshTokenResponse{
		AccessToken: newAccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(util.AccessExpire.Seconds()),
	}, nil
}

// Logout 退出当前设备登录态。
func (s *authServiceImpl) Logout(ctx context.Context, req *authpb.LogoutRequest) error {
	if req == nil || req.DeviceId == "" {
		return apperr.New(consts.CodeParamError)
	}

	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeInternalError)
	}
	if err := s.deviceRepo.DeleteTokens(ctx, userUUID, req.DeviceId); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "删除 Token 失败")
	}
	if err := s.deviceRepo.UpdateOnlineStatus(ctx, userUUID, req.DeviceId, model.DeviceStatusLoggedOut); err != nil {
		if !errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.Wrap(err, consts.CodeInternalError, "更新设备注销状态失败")
		}
		logger.Warn(ctx, "登出时设备会话不存在，按幂等成功处理",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", req.DeviceId),
		)
	}
	if err := s.deviceRepo.SetActiveTimestamp(ctx, userUUID, req.DeviceId, time.Now().Unix()); err != nil {
		logger.Warn(ctx, "写入登出活跃时间失败",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", req.DeviceId),
			logger.ErrorField("error", err),
		)
	}
	return nil
}

// ResetPassword 通过验证码重置密码。
func (s *authServiceImpl) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) error {
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeUserNotFound)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 3)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			return apperr.New(consts.CodeVerifyCodeExpire)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return apperr.New(consts.CodeVerifyCodeError)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.NewPassword)); err == nil {
		return apperr.New(consts.CodePasswordSameAsOld)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}
	if err := s.authRepo.UpdatePassword(ctx, user.Uuid, string(hashedPassword)); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "更新密码失败")
	}
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 3); err != nil {
		logger.Warn(ctx, "删除验证码失败", logger.ErrorField("error", err))
	}
	return nil
}
