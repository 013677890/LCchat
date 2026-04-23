package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/converter"
	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

// buildDeviceUserAgent 生成精简版 UserAgent（保留必要信息）
func buildDeviceUserAgent(deviceInfo *pb.DeviceInfo) string {
	if deviceInfo == nil {
		return ""
	}
	platform := deviceInfo.GetPlatform()
	appVersion := deviceInfo.GetAppVersion()
	osVersion := deviceInfo.GetOsVersion()

	result := ""
	if platform != "" {
		result = platform
	}
	if appVersion != "" {
		if result != "" {
			result += "/" + appVersion
		} else {
			result = appVersion
		}
	}
	if osVersion != "" {
		if result != "" {
			result += " (" + osVersion + ")"
		} else {
			result = osVersion
		}
	}
	return result
}

func getRequiredDeviceID(ctx context.Context) (string, error) {
	deviceID := strings.TrimSpace(util.GetDeviceIDFromContext(ctx))
	if deviceID == "" {
		return "", apperr.New(consts.CodeParamError)
	}
	return deviceID, nil
}

// authServiceImpl 认证服务实现
type authServiceImpl struct {
	authRepo   repository.IAuthRepository
	deviceRepo repository.IDeviceRepository
}

// NewAuthService 创建认证服务实例
func NewAuthService(
	authRepo repository.IAuthRepository,
	deviceRepo repository.IDeviceRepository,
) AuthService {
	return &authServiceImpl{
		authRepo:   authRepo,
		deviceRepo: deviceRepo,
	}
}

// Register 用户注册
// 业务流程：
//  1. 校验验证码
//  2. 创建用户
//  3. 返回用户信息
//
// 错误码映射：
//   - codes.Unauthenticated: 验证码错误
//   - codes.Internal: 系统内部错误
//   - codes.AlreadyExists: 用户已存在
func (s *authServiceImpl) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// 访问日志已经由 gRPC 拦截器统一记录，这里不再重复记录“请求开始”日志。
	// 仅在明确的错误处理点、降级点和必要的成功里程碑处记录业务语义日志。

	// 1. 校验验证码（type=1: 注册）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 1)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeError)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	// 2. 创建用户

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}
	// 将密码哈希化
	user := &model.UserInfo{
		Uuid:      util.GenIDString(),
		Email:     req.Email,
		Password:  string(hashedPassword),
		Nickname:  req.Nickname,
		Telephone: strings.TrimSpace(req.Telephone),
		Status:    0,
		IsAdmin:   0,
	}
	var return_user *model.UserInfo
	// 向数据库中插入
	if return_user, err = s.authRepo.Create(ctx, user); err != nil {
		// 使用 errors.Is 判断是否是唯一键冲突
		if errors.Is(err, repository.ErrDuplicateKey) {
			return nil, apperr.New(consts.CodeUserAlreadyExist)
		}

		// 其他数据库错误
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建用户失败")
	}
	return &pb.RegisterResponse{
		UserUuid:  return_user.Uuid,
		Nickname:  return_user.Nickname,
		Email:     return_user.Email,
		Telephone: return_user.Telephone,
	}, nil
}

// Login 用户登录（密码）
// 业务流程：
//  1. 根据账号（邮箱）查询用户
//  2. 校验用户状态（是否被禁用）
//  3. 校验密码
//  4. 返回用户信息（供Gateway生成Token）
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Unauthenticated: 密码错误
//   - codes.PermissionDenied: 用户被禁用
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 处理 DeviceInfo 为空的情况
	if req.DeviceInfo == nil {
		req.DeviceInfo = &pb.DeviceInfo{
			DeviceName: "Unknown",
			Platform:   "Unknown",
		}
	}

	// 访问日志由统一拦截器记录，这里不重复记录登录入口日志。
	// 保留下面的异常日志和成功里程碑日志，用于定位业务关键节点。

	// 1. 根据账号查询用户（邮箱）
	user, err := s.authRepo.GetByEmail(ctx, req.Account)
	if err != nil {
		// 使用 errors.Is 判断错误类型
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}

		// 其他数据库错误
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	// 2. 校验用户状态
	if user.Status == 1 {
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	// 3. 将用户uuid写入context
	ctx = ctxmeta.WithUserUUID(ctx, user.Uuid)

	// 4. 校验密码
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		return nil, apperr.New(consts.CodePasswordError)
	}

	// 5. 从 context 中获取设备 ID 和客户端 IP
	deviceID, err := getRequiredDeviceID(ctx)
	if err != nil {
		return nil, err
	}
	clientIP := util.GetClientIPFromContext(ctx)

	// 6. 生成访问令牌
	accessToken, err := util.GenerateToken(user.Uuid, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成访问令牌失败")
	}

	// 7. 生成刷新令牌（使用 UUID）
	refreshToken := util.GenIDString()

	// 8. 写入 Redis（AccessToken 和 RefreshToken）
	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 AccessToken 失败")
	}

	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 RefreshToken 失败")
	}

	// 9. 设备会话落库（Upsert：存在则更新，不存在则插入）
	deviceSession := &model.DeviceSession{
		UserUuid:   user.Uuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  buildDeviceUserAgent(req.DeviceInfo),
		Status:     model.DeviceStatusOnline, // 在线
	}

	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Warn(ctx, "设备会话落库失败，按降级处理继续登录",
			logger.ErrorField("error", err),
		)
		// 注意：设备会话落库失败不应该阻止登录成功，因为 Token 已经生成
		// 这里只记录日志，不返回错误
	}

	// 10. 登录成功后立即写入活跃时间，确保在线状态可立即查询。
	if deviceID != "" {
		if err := s.deviceRepo.SetActiveTimestamp(ctx, user.Uuid, deviceID, time.Now().Unix()); err != nil {
			logger.Warn(ctx, "写入设备活跃时间失败",
				logger.String("user_uuid", user.Uuid),
				logger.String("device_id", deviceID),
				logger.ErrorField("error", err),
			)
		}
	}

	// 11. 登录成功
	return &pb.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(util.AccessExpire.Seconds()),
		UserInfo:     converter.ModelToProtoUserInfo(user),
	}, nil
}

// LoginByCode 验证码登录
// 业务流程：
//  1. 根据邮箱查询用户
//  2. 校验用户状态（是否被禁用）
//  3. 校验验证码
//  4. 生成Token并返回用户信息
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Unauthenticated: 验证码错误或已过期
//   - codes.PermissionDenied: 用户被禁用
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) LoginByCode(ctx context.Context, req *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
	// 处理 DeviceInfo 为空的情况
	if req.DeviceInfo == nil {
		req.DeviceInfo = &pb.DeviceInfo{
			DeviceName: "Unknown",
			Platform:   "Unknown",
		}
	}

	// 访问日志由统一拦截器记录，这里不重复记录验证码登录入口日志。
	// 保留下面的异常日志和成功里程碑日志，用于排查鉴权链路问题。

	// 1. 根据邮箱查询用户
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// 使用 errors.Is 判断错误类型
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}

		// 其他数据库错误
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	// 2. 校验用户状态
	if user.Status == 1 {
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	// 3. 获取并校验设备 ID（必须是统一设备标识）
	deviceID, err := getRequiredDeviceID(ctx)
	if err != nil {
		return nil, err
	}

	// 4. 校验验证码（type=2: 登录）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 2)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	// 验证成功后立即删除验证码（消耗验证码，防止重复使用）
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 2); err != nil {
		logger.Warn(ctx, "删除验证码失败",
			logger.ErrorField("error", err),
		)
		// 删除失败不影响登录流程，只记录警告日志
	}

	// 5. 将用户uuid写入context
	ctx = ctxmeta.WithUserUUID(ctx, user.Uuid)

	// 6. 从 context 中获取客户端 IP
	clientIP := util.GetClientIPFromContext(ctx)

	// 7. 生成访问令牌
	accessToken, err := util.GenerateToken(user.Uuid, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成访问令牌失败")
	}

	// 8. 生成刷新令牌（使用 UUID）
	refreshToken := util.GenIDString()

	// 9. 写入 Redis（AccessToken 和 RefreshToken）
	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 AccessToken 失败")
	}

	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 RefreshToken 失败")
	}

	// 10. 设备会话落库（Upsert：存在则更新，不存在则插入）
	deviceSession := &model.DeviceSession{
		UserUuid:   user.Uuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  buildDeviceUserAgent(req.DeviceInfo),
		Status:     model.DeviceStatusOnline, // 在线
	}

	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Warn(ctx, "设备会话落库失败，按降级处理继续登录",
			logger.ErrorField("error", err),
		)
		// 注意：设备会话落库失败不应该阻止登录成功，因为 Token 已经生成
		// 这里只记录日志，不返回错误
	}

	// 11. 登录成功后立即写入活跃时间，确保在线状态可立即查询。
	if deviceID != "" {
		if err := s.deviceRepo.SetActiveTimestamp(ctx, user.Uuid, deviceID, time.Now().Unix()); err != nil {
			logger.Warn(ctx, "写入设备活跃时间失败",
				logger.String("user_uuid", user.Uuid),
				logger.String("device_id", deviceID),
				logger.ErrorField("error", err),
			)
		}
	}

	// 12. 登录成功，记录日志
	return &pb.LoginByCodeResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(util.AccessExpire.Seconds()),
		UserInfo:     converter.ModelToProtoUserInfo(user),
	}, nil
}

// SendVerifyCode 发送验证码
func (s *authServiceImpl) SendVerifyCode(ctx context.Context, req *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
	// 发送验证码属于高频入口，请求级访问日志已经由统一拦截器记录。
	// 这里仅保留后续错误处理和发送成功的业务结果日志。

	// 1. 校验邮箱格式
	if !util.ValidateEmail(req.Email) {
		logger.Warn(ctx, "邮箱格式无效",
			logger.String("email", req.Email),
		)
		return nil, apperr.New(consts.CodeInvalidEmail)
	}

	// 2. 限流检查（防止频繁发送）
	ip := util.GetClientIPFromContext(ctx)
	isLimited, err := s.authRepo.VerifyVerifyCodeRateLimit(ctx, req.Email, ip)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "验证码限流检查失败")
	}
	if isLimited {
		return nil, apperr.New(consts.CodeSendTooFrequent)
	}

	// 3. 生成6位验证码
	code, err := util.GenerateVerifyCode(6)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成验证码失败")
	}

	// 4. 存储验证码到Redis（2分钟过期）
	err = s.authRepo.StoreVerifyCode(ctx, req.Email, code, req.Type, 2*time.Minute)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "存储验证码失败")
	}

	// 5. 递增限流计数
	err = s.authRepo.IncrementVerifyCodeCount(ctx, req.Email, ip)
	if err != nil {
		logger.Warn(ctx, "递增验证码计数失败",
			logger.ErrorField("error", err),
		)
		// 不影响主流程，只记录日志
	}

	// 6. 发送验证码邮件
	err = util.SendVerifyCodeEmail(req.Email, code, 2) // 2分钟有效期
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "发送验证码邮件失败")
	}

	return &pb.SendVerifyCodeResponse{
		ExpireSeconds: 120, // 2分钟=120秒
	}, nil
}

// VerifyCode 校验验证码
// 业务流程：
//  1. 校验验证码是否正确
//  2. 返回验证结果（不消耗验证码）
//
// 错误码映射：
//   - codes.Unauthenticated: 验证码错误或已过期
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) VerifyCode(ctx context.Context, req *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
	// 访问日志由统一拦截器记录，这里不重复记录校验验证码入口日志。
	// 保留下方业务判断日志即可满足问题排查需要。

	// 1. 校验验证码（type参数：1:注册 2:登录 3:重置密码 4:换绑邮箱）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, req.Type)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}

	// 2. 返回验证结果
	return &pb.VerifyCodeResponse{
		Valid: isValid,
	}, nil
}

// RefreshToken 刷新Token
// 业务流程：
//  1. 从 context 中获取 user_uuid 和 device_id（由 Gateway 写入）
//  2. 验证 Refresh Token 是否在 Redis 中存在且匹配
//  3. 生成新的 Access Token
//  4. 更新 Redis 中的 Access Token
//  5. 返回新的 Access Token
//
// 错误码映射：
//   - codes.InvalidArgument: Refresh Token 无效
//   - codes.NotFound: 设备会话不存在
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	// 刷新 Token 的访问日志已经由统一拦截器记录，这里只保留后续错误与成功结果日志。

	// 1. 从 context 中获取 user_uuid 和 device_id
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	deviceID := util.GetDeviceIDFromContext(ctx)
	if deviceID == "" {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	// 2. 验证 Refresh Token 是否在 Redis 中存在
	storedRefreshToken, err := s.deviceRepo.GetRefreshToken(ctx, userUUID, deviceID)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeDeviceNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取 Refresh Token 失败")
	}

	// 3. 校验 Refresh Token 是否匹配
	if storedRefreshToken != req.RefreshToken {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	// 4. 生成新的 Access Token
	newAccessToken, err := util.GenerateToken(userUUID, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成 Access Token 失败")
	}

	// 5. 更新 Redis 中的 Access Token
	if err := s.deviceRepo.StoreAccessToken(ctx, userUUID, deviceID, newAccessToken, util.AccessExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新 Access Token 失败")
	}

	// 6. 续期设备信息缓存 TTL
	if err := s.deviceRepo.TouchDeviceInfoTTL(ctx, userUUID); err != nil {
		logger.Warn(ctx, "续期设备信息缓存失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
	}

	// 6. 刷新成功
	return &pb.RefreshTokenResponse{
		AccessToken: newAccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(util.AccessExpire.Seconds()),
	}, nil
}

// Logout 用户登出
// 业务流程：
//  1. 从 context 中获取 user_uuid（由 JWT 中间件解析）
//  2. 删除 Redis 中的 Access Token 和 Refresh Token
//  3. 返回成功
//
// 错误码映射：
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) Logout(ctx context.Context, req *pb.LogoutRequest) error {
	if req == nil || req.DeviceId == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 访问日志由统一拦截器记录，这里不重复记录登出入口日志。
	// 保留下面的异常日志和登出成功日志用于审计关键动作。

	// 1. 从 context 中获取 user_uuid
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeInternalError)
	}

	// 2. 删除 Redis 中的 Token
	if err := s.deviceRepo.DeleteTokens(ctx, userUUID, req.DeviceId); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "删除 Token 失败")
	}

	// 3. 登出语义为注销设备会话（status=2），设备不存在视为幂等成功。
	if err := s.deviceRepo.UpdateOnlineStatus(ctx, userUUID, req.DeviceId, model.DeviceStatusLoggedOut); err != nil {
		if !errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.Wrap(err, consts.CodeInternalError, "更新设备注销状态失败")
		}
		logger.Warn(ctx, "登出时设备会话不存在，按幂等成功处理",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", req.DeviceId),
		)
	}

	// 4. 写入最后活跃时间（尽力而为，不阻塞登出）。
	if err := s.deviceRepo.SetActiveTimestamp(ctx, userUUID, req.DeviceId, time.Now().Unix()); err != nil {
		logger.Warn(ctx, "写入登出活跃时间失败",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", req.DeviceId),
			logger.ErrorField("error", err),
		)
	}

	// 5. 登出成功
	return nil
}

// ResetPassword 重置密码
// 业务流程：
//  1. 根据邮箱查询用户
//  2. 校验验证码
//  3. 校验新密码是否与旧密码相同
//  4. 生成新密码哈希
//  5. 更新密码
//  6. 删除验证码
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Unauthenticated: 验证码错误或已过期
//   - codes.FailedPrecondition: 新密码不能与旧密码相同
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) error {
	// 访问日志由统一拦截器记录，这里不重复记录重置密码入口日志。
	// 保留下方错误处理和最终成功日志，用于审计敏感操作。

	// 1. 根据邮箱查询用户
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// 使用 errors.Is 判断错误类型
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeUserNotFound)
		}

		// 其他数据库错误
		return apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	// 2. 校验验证码（type=3: 重置密码）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 3)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return apperr.New(consts.CodeVerifyCodeExpire)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return apperr.New(consts.CodeVerifyCodeError)
	}

	// 3. 校验新密码是否与旧密码相同
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.NewPassword))
	if err == nil {
		// 密码相同
		return apperr.New(consts.CodePasswordSameAsOld)
	}

	// 4. 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}

	// 5. 更新密码
	err = s.authRepo.UpdatePassword(ctx, user.Uuid, string(hashedPassword))
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "更新密码失败")
	}

	// 6. 删除验证码（消耗验证码，防止重复使用）
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 3); err != nil {
		logger.Warn(ctx, "删除验证码失败",
			logger.ErrorField("error", err),
		)
		// 删除失败不影响重置密码流程，只记录警告日志
	}

	// 7. 重置成功
	return nil
}
