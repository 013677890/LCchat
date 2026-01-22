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

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
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
func (s *authServiceImpl) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "注册功能暂未实现")
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
	// 记录登录请求（账号脱敏）
	logger.Info(ctx, "用户登录请求",
		logger.String("account", utils.MaskPhone(req.Account)),
		logger.String("device_name", req.DeviceInfo.GetDeviceName()),
		logger.String("platform", req.DeviceInfo.GetPlatform()),
	)

	// 1. 根据账号查询用户（邮箱）
	user, err := s.authRepo.GetByEmail(ctx, req.Account)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
		} else {
			logger.Error(ctx, "查询用户失败",
				logger.String("account", utils.MaskEmail(req.Account)),
				logger.ErrorField("error", err),
			)
			return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
		}
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
			logger.String("user_uuid", user.Uuid),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 7. 生成刷新令牌（使用 UUID）
	refreshToken := util.GenIDString()

	// 8. 写入 Redis（AccessToken 和 RefreshToken）
	if err := s.deviceRepo.StoreAccessToken(ctx, user.Uuid, deviceID, accessToken, util.AccessExpire); err != nil {
		logger.Error(ctx, "AccessToken 写入 Redis 失败",
			logger.String("user_uuid", user.Uuid),
			logger.String("device_id", deviceID),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if err := s.deviceRepo.StoreRefreshToken(ctx, user.Uuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		logger.Error(ctx, "RefreshToken 写入 Redis 失败",
			logger.String("user_uuid", user.Uuid),
			logger.String("device_id", deviceID),
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
			logger.String("user_uuid", user.Uuid),
			logger.String("device_id", deviceID),
			logger.ErrorField("error", err),
		)
		// 注意：设备会话落库失败不应该阻止登录成功，因为 Token 已经生成
		// 这里只记录日志，不返回错误
	}

	// 10. 登录成功
	logger.Info(ctx, "用户登录成功",
		logger.String("user_uuid", user.Uuid),
		logger.String("account", utils.MaskPhone(req.Account)),
		logger.String("device_id", deviceID),
		logger.String("platform", req.DeviceInfo.GetPlatform()),
		logger.String("client_ip", clientIP),
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
func (s *authServiceImpl) LoginByCode(ctx context.Context, req *pb.LoginByCodeRequest) (*pb.LoginByCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "验证码登录功能暂未实现")
}

// SendVerifyCode 发送验证码
func (s *authServiceImpl) SendVerifyCode(ctx context.Context, req *pb.SendVerifyCodeRequest) (*pb.SendVerifyCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "发送验证码功能暂未实现")
}

// VerifyCode 校验验证码
func (s *authServiceImpl) VerifyCode(ctx context.Context, req *pb.VerifyCodeRequest) (*pb.VerifyCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "校验验证码功能暂未实现")
}

// RefreshToken 刷新Token
func (s *authServiceImpl) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	return nil, status.Error(codes.Unimplemented, "刷新Token功能暂未实现")
}

// Logout 用户登出
func (s *authServiceImpl) Logout(ctx context.Context, req *pb.LogoutRequest) error {
	return status.Error(codes.Unimplemented, "登出功能暂未实现")
}

// ResetPassword 重置密码
func (s *authServiceImpl) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) error {
	return status.Error(codes.Unimplemented, "重置密码功能暂未实现")
}
