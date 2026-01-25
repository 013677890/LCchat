package service

import (
	"ChatServer/apps/user/internal/converter"
	"ChatServer/apps/user/internal/repository"
	"ChatServer/apps/user/internal/utils"
	pb "ChatServer/apps/user/pb"
	"ChatServer/consts"
	"ChatServer/model"
	"ChatServer/pkg/logger"
	"ChatServer/pkg/util"
	"context"
	"errors"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
	// 记录注册请求
	logger.Info(ctx, "用户注册请求",
		logger.String("email", req.Email),
		logger.String("password", req.Password),
		logger.String("verify_code", req.VerifyCode),
		logger.String("nickname", req.Nickname),
		logger.String("telephone", req.Telephone),
	)

	// 1. 校验验证码（type=1: 注册）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 1)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeError))
		}
		logger.Error(ctx, "校验验证码失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if !isValid {
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeError))
	}

	// 2. 创建用户

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Error(ctx, "生成密码哈希失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	// 将密码哈希化
	user := &model.UserInfo{
		Uuid:      util.GenIDString(),
		Email:     req.Email,
		Password:  string(hashedPassword),
		Nickname:  req.Nickname,
		Telephone: req.Telephone,
		Status:    0,
		IsAdmin:   0,
	}
	var return_user *model.UserInfo
	// 向数据库中插入
	if return_user, err = s.authRepo.Create(ctx, user); err != nil {
		// 使用 errors.Is 判断是否是唯一键冲突
		if errors.Is(err, repository.ErrDuplicateKey) {
			logger.Warn(ctx, "用户已存在",
				logger.String("email", req.Email),
				logger.ErrorField("error", err), // 这里会包含原始的 GORM 错误信息
			)
			return nil, status.Error(codes.AlreadyExists, strconv.Itoa(consts.CodeUserAlreadyExist))
		}

		// 其他数据库错误
		logger.Error(ctx, "创建用户失败",
			logger.String("email", req.Email),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
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

	// 记录登录请求（账号脱敏）
	logger.Info(ctx, "用户登录请求",
		logger.String("account", utils.MaskPhone(req.Account)),
		logger.String("device_name", req.DeviceInfo.GetDeviceName()),
		logger.String("platform", req.DeviceInfo.GetPlatform()),
	)

	// 1. 根据账号查询用户（邮箱）
	user, err := s.authRepo.GetByEmail(ctx, req.Account)
	if err != nil {
		// 使用 errors.Is 判断错误类型
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
		}

		// 其他数据库错误
		logger.Error(ctx, "查询用户失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 2. 校验用户状态
	if user.Status == 1 {
		return nil, status.Error(codes.PermissionDenied, strconv.Itoa(consts.CodeUserDisabled))
	}

	// 3. 将用户uuid写入context
	ctx = context.WithValue(ctx, "user_uuid", user.Uuid)

	// 4. 校验密码
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodePasswordError))
	}

	// 5. 从 context 中获取设备 ID 和客户端 IP
	deviceID := util.GetDeviceIDFromContext(ctx)
	if deviceID == "" {
		// 如果没有从 context 中获取到，使用设备名称作为临时方案
		deviceID = req.DeviceInfo.GetDeviceName()
		logger.Warn(ctx, "DeviceID not found in context, using device name as fallback",
			logger.String("device_name", deviceID),
		)
	}
	clientIP := util.GetClientIPFromContext(ctx)

	// 6. 生成访问令牌
	accessToken, err := util.GenerateToken(user.Uuid, deviceID)
	if err != nil {
		logger.Error(ctx, "生成访问令牌失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 7. 生成刷新令牌（使用 UUID）
	refreshToken := util.GenIDString()

	// 8. 写入 Redis（AccessToken 和 RefreshToken）
	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		logger.Error(ctx, "AccessToken 写入 Redis 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		logger.Error(ctx, "RefreshToken 写入 Redis 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 9. 设备会话落库（Upsert：存在则更新，不存在则插入）
	deviceSession := &model.DeviceSession{
		UserUuid:   user.Uuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  req.DeviceInfo.GetOsVersion(), // 可以根据实际需求调整
		Status:     0,                             // 0: 在线
	}

	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Error(ctx, "设备会话落库失败",
			logger.ErrorField("error", err),
		)
		// 注意：设备会话落库失败不应该阻止登录成功，因为 Token 已经生成
		// 这里只记录日志，不返回错误
	}

	// 10. 登录成功
	logger.Info(ctx, "用户登录成功",
		logger.String("account", utils.MaskPhone(req.Account)),
		logger.String("platform", req.DeviceInfo.GetPlatform()),
	)

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

	// 记录验证码登录请求（邮箱脱敏）
	logger.Info(ctx, "验证码登录请求",
		logger.String("email", utils.MaskPhone(req.Email)),
		logger.String("device_name", req.DeviceInfo.GetDeviceName()),
		logger.String("platform", req.DeviceInfo.GetPlatform()),
	)

	// 1. 根据邮箱查询用户
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// 使用 errors.Is 判断错误类型
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
		}

		// 其他数据库错误
		logger.Error(ctx, "查询用户失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 2. 校验用户状态
	if user.Status == 1 {
		return nil, status.Error(codes.PermissionDenied, strconv.Itoa(consts.CodeUserDisabled))
	}

	// 3. 校验验证码（type=2: 登录）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 2)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeExpire))
		}
		logger.Error(ctx, "校验验证码失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if !isValid {
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeError))
	}

	// 验证成功后立即删除验证码（消耗验证码，防止重复使用）
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 2); err != nil {
		logger.Warn(ctx, "删除验证码失败",
			logger.ErrorField("error", err),
		)
		// 删除失败不影响登录流程，只记录警告日志
	}

	// 4. 将用户uuid写入context
	ctx = context.WithValue(ctx, "user_uuid", user.Uuid)

	// 5. 从 context 中获取设备 ID 和客户端 IP
	deviceID := util.GetDeviceIDFromContext(ctx)
	if deviceID == "" {
		// 如果没有从 context 中获取到，使用设备名称作为临时方案
		deviceID = req.DeviceInfo.GetDeviceName()
		logger.Warn(ctx, "DeviceID not found in context, using device name as fallback",
			logger.String("device_name", deviceID),
		)
	}
	clientIP := util.GetClientIPFromContext(ctx)

	// 6. 生成访问令牌
	accessToken, err := util.GenerateToken(user.Uuid, deviceID)
	if err != nil {
		logger.Error(ctx, "生成访问令牌失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 7. 生成刷新令牌（使用 UUID）
	refreshToken := util.GenIDString()

	// 8. 写入 Redis（AccessToken 和 RefreshToken）
	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		logger.Error(ctx, "AccessToken 写入 Redis 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		logger.Error(ctx, "RefreshToken 写入 Redis 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 9. 设备会话落库（Upsert：存在则更新，不存在则插入）
	deviceSession := &model.DeviceSession{
		UserUuid:   user.Uuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  req.DeviceInfo.GetOsVersion(), // 可以根据实际需求调整
		Status:     0,                             // 0: 在线
	}

	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Error(ctx, "设备会话落库失败",
			logger.ErrorField("error", err),
		)
		// 注意：设备会话落库失败不应该阻止登录成功，因为 Token 已经生成
		// 这里只记录日志，不返回错误
	}

	// 10. 登录成功，记录日志
	logger.Info(ctx, "验证码登录成功",
		logger.String("email", utils.MaskPhone(req.Email)),
		logger.String("platform", req.DeviceInfo.GetPlatform()),
	)

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
	// 记录发送验证码请求
	logger.Info(ctx, "发送验证码请求",
		logger.String("email", req.Email),
	)

	// 1. 校验邮箱格式
	if !util.ValidateEmail(req.Email) {
		logger.Warn(ctx, "邮箱格式无效",
			logger.String("email", req.Email),
		)
		return nil, status.Error(codes.InvalidArgument, strconv.Itoa(consts.CodeInvalidEmail))
	}

	// 2. 限流检查（防止频繁发送）
	ip := util.GetClientIPFromContext(ctx)
	isLimited, err := s.authRepo.VerifyVerifyCodeRateLimit(ctx, req.Email, ip)
	if err != nil {
		logger.Error(ctx, "验证码限流检查失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if isLimited {
		return nil, status.Error(codes.ResourceExhausted, strconv.Itoa(consts.CodeSendTooFrequent))
	}

	// 3. 生成6位验证码
	code, err := util.GenerateVerifyCode(6)
	if err != nil {
		logger.Error(ctx, "生成验证码失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 4. 存储验证码到Redis（2分钟过期）
	err = s.authRepo.StoreVerifyCode(ctx, req.Email, code, req.Type, 2*time.Minute)
	if err != nil {
		logger.Error(ctx, "存储验证码失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
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
		logger.Error(ctx, "发送验证码邮件失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	logger.Info(ctx, "验证码发送成功",
		logger.String("email", req.Email),
	)

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
	// 记录校验验证码请求（邮箱脱敏）
	logger.Info(ctx, "校验验证码请求",
		logger.String("email", utils.MaskPhone(req.Email)),
		logger.Int("type", int(req.Type)),
	)

	// 1. 校验验证码（type参数：1:注册 2:登录 3:重置密码 4:换绑邮箱）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, req.Type)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeExpire))
		}
		logger.Error(ctx, "校验验证码失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 2. 返回验证结果
	logger.Info(ctx, "验证码校验结果",
		logger.String("email", utils.MaskPhone(req.Email)),
		logger.Bool("valid", isValid),
	)

	return &pb.VerifyCodeResponse{
		Valid: isValid,
	}, nil
}

// RefreshToken 刷新Token
// 业务流程：
//  1. 解析 Refresh Token（包含 user_uuid 和 device_id）
//  2. 验证 Refresh Token 是否在 Redis 中存在
//  3. 生成新的 Access Token
//  4. 更新 Redis 中的 Access Token
//  5. 返回新的 Access Token
//
// 错误码映射：
//   - codes.InvalidArgument: Refresh Token 无效
//   - codes.DeadlineExceeded: Refresh Token 已过期
//   - codes.NotFound: 设备会话不存在
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	// 记录刷新Token请求
	logger.Info(ctx, "刷新Token请求")

	// 1. 解析 Refresh Token（包含 user_uuid 和 device_id）
	claims, err := util.ParseToken(req.RefreshToken)
	if err != nil {
		// Refresh Token 解析失败
		logger.Warn(ctx, "Refresh Token 解析失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.InvalidArgument, strconv.Itoa(consts.CodeInvalidToken))
	}

	// 2. 验证 Refresh Token 是否在 Redis 中存在
	storedRefreshToken, err := s.deviceRepo.GetRefreshToken(ctx, claims.UserUUID, claims.DeviceID)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			logger.Warn(ctx, "Refresh Token 不存在",
				logger.String("user_uuid", claims.UserUUID),
				logger.String("device_id", claims.DeviceID),
			)
			return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeDeviceNotFound))
		}
		logger.Error(ctx, "获取 Refresh Token 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 3. 校验 Refresh Token 是否匹配
	if storedRefreshToken != req.RefreshToken {
		logger.Warn(ctx, "Refresh Token 不匹配",
			logger.String("user_uuid", claims.UserUUID),
			logger.String("device_id", claims.DeviceID),
		)
		return nil, status.Error(codes.InvalidArgument, strconv.Itoa(consts.CodeInvalidToken))
	}

	// 4. 生成新的 Access Token
	newAccessToken, err := util.GenerateToken(claims.UserUUID, claims.DeviceID)
	if err != nil {
		logger.Error(ctx, "生成 Access Token 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 5. 更新 Redis 中的 Access Token
	if err := s.deviceRepo.StoreAccessToken(ctx, claims.UserUUID, claims.DeviceID, newAccessToken, util.AccessExpire); err != nil {
		logger.Error(ctx, "更新 Access Token 失败",
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 6. 刷新成功
	logger.Info(ctx, "Token 刷新成功",
		logger.String("user_uuid", claims.UserUUID),
		logger.String("device_id", claims.DeviceID),
	)

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
	// 记录登出请求
	logger.Info(ctx, "用户登出请求",
		logger.String("device_id", req.DeviceId),
	)

	// 1. 从 context 中获取 user_uuid
	userUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || userUUID == "" {
		logger.Error(ctx, "从 context 中获取 user_uuid 失败")
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 2. 删除 Redis 中的 Token
	if err := s.deviceRepo.DeleteTokens(ctx, userUUID, req.DeviceId); err != nil {
		logger.Error(ctx, "删除 Token 失败",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", req.DeviceId),
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 3. 登出成功
	logger.Info(ctx, "用户登出成功",
		logger.String("user_uuid", userUUID),
		logger.String("device_id", req.DeviceId),
	)

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
	// 记录重置密码请求（邮箱脱敏）
	logger.Info(ctx, "用户重置密码请求",
		logger.String("email", utils.MaskPhone(req.Email)),
	)

	// 1. 根据邮箱查询用户
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// 使用 errors.Is 判断错误类型
		if errors.Is(err, repository.ErrRecordNotFound) {
			return status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
		}

		// 其他数据库错误
		logger.Error(ctx, "查询用户失败",
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 2. 校验验证码（type=3: 重置密码）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 3)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeExpire))
		}
		logger.Error(ctx, "校验验证码失败",
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if !isValid {
		return status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeError))
	}

	// 3. 校验新密码是否与旧密码相同
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.NewPassword))
	if err == nil {
		// 密码相同
		return status.Error(codes.FailedPrecondition, strconv.Itoa(consts.CodePasswordSameAsOld))
	}

	// 4. 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		logger.Error(ctx, "生成密码哈希失败",
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 5. 更新密码
	err = s.authRepo.UpdatePassword(ctx, user.Uuid, string(hashedPassword))
	if err != nil {
		logger.Error(ctx, "更新密码失败",
			logger.String("user_uuid", user.Uuid),
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 6. 删除验证码（消耗验证码，防止重复使用）
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 3); err != nil {
		logger.Warn(ctx, "删除验证码失败",
			logger.ErrorField("error", err),
		)
		// 删除失败不影响重置密码流程，只记录警告日志
	}

	// 7. 重置成功
	logger.Info(ctx, "用户密码重置成功",
		logger.String("email", utils.MaskPhone(req.Email)),
	)

	return nil
}
