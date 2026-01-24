package service

import (
	"ChatServer/apps/gateway/internal/dto"
	"ChatServer/apps/gateway/internal/pb"
	"ChatServer/apps/gateway/internal/utils"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
	"context"
	"time"
	"errors"
	"strconv"
)

// AuthServiceImpl 认证服务实现
type AuthServiceImpl struct {
	userClient pb.UserServiceClient
}

// NewAuthService 创建认证服务实例
// userClient: 用户服务 gRPC 客户端
func NewAuthService(userClient pb.UserServiceClient) AuthService {
	return &AuthServiceImpl{
		userClient: userClient,
	}
}

// Login 用户登录
// ctx: 请求上下文
// req: 登录请求
// deviceId: 设备ID
// 返回: 完整的登录响应（包含Token和用户信息）
func (s *AuthServiceImpl) Login(ctx context.Context, req *dto.LoginRequest, deviceId string) (*dto.LoginResponse, error) {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoLoginRequest(req)

	// 2. 调用用户服务进行身份认证(gRPC)
	grpcResp, err := s.userClient.Login(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		code := utils.ExtractErrorCode(err)
		// 记录错误日志
		logger.Error(ctx, "调用用户服务 gRPC 失败",
			logger.ErrorField("error", err),
			logger.Int("business_code", code),
			logger.String("business_message", consts.GetMessage(code)),
			logger.Duration("duration", time.Since(startTime)),
		)

		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，检查响应数据
	if grpcResp.UserInfo == nil {
		// 成功返回但 UserInfo 为空，属于非预期的异常情况
		logger.Error(ctx, "gRPC 成功响应但用户信息为空")
		return nil, errors.New(strconv.Itoa(consts.CodeInternalError))
	}

	return dto.ConvertLoginResponseFromProto(grpcResp), nil
}

// Register 用户注册
// ctx: 请求上下文
// req: 注册请求
// 返回: 完整的注册响应（包含Token和用户信息）
func (s *AuthServiceImpl) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoRegisterRequest(req)

	// 2. 调用用户服务进行注册(gRPC)
	grpcResp, err := s.userClient.Register(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		code := utils.ExtractErrorCode(err)
		// 记录错误日志
		logger.Error(ctx, "调用用户服务 gRPC 失败",
			logger.ErrorField("error", err),
			logger.Int("business_code", code),
			logger.String("business_message", consts.GetMessage(code)),
			logger.Duration("duration", time.Since(startTime)),
		)
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，检查响应数据
	if grpcResp.UserUuid == "" {
		// 成功返回但 UserUuid 为空，属于非预期的异常情况
		logger.Error(ctx, "gRPC 成功响应但用户信息为空")
		return nil, errors.New(strconv.Itoa(consts.CodeInternalError))
	}

	return dto.ConvertRegisterResponseFromProto(grpcResp), nil
}

// SendVerifyCode 发送验证码
// ctx: 请求上下文
// req: 发送验证码请求
// 返回: 发送验证码响应
func (s *AuthServiceImpl) SendVerifyCode(ctx context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error) {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoSendVerifyCodeRequest(req)

	// 2. 调用用户服务进行发送验证码(gRPC)
	grpcResp, err := s.userClient.SendVerifyCode(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		code := utils.ExtractErrorCode(err)
		// 记录错误日志
		logger.Error(ctx, "调用用户服务 gRPC 失败",
			logger.ErrorField("error", err),
			logger.Int("business_code", code),
			logger.String("business_message", consts.GetMessage(code)),
			logger.Duration("duration", time.Since(startTime)),
		)
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	return dto.ConvertSendVerifyCodeResponseFromProto(grpcResp), nil
}