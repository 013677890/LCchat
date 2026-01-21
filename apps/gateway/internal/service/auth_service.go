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

	/*
	// 4. 生成访问令牌
	accessToken, err := s.generateAccessToken(ctx, grpcResp.UserInfo.Uuid, deviceId)
	if err != nil {
		return nil, &BusinessError{
			Code:    consts.CodeInternalError,
			Message: "生成访问令牌失败",
		}
	}

	// 5. 生成刷新令牌
	refreshToken, err := s.generateRefreshToken(ctx, grpcResp.UserInfo.Uuid, deviceId)
	if err != nil {
		return nil, &BusinessError{
			Code:    consts.CodeInternalError,
			Message: "生成刷新令牌失败",
		}
	}
*/
	// 记录成功日志
	logger.Info(ctx, "登录服务处理成功",
		logger.String("user_uuid", grpcResp.UserInfo.Uuid),
		logger.Duration("duration", time.Since(startTime)),
	)

	return dto.ConvertLoginResponseFromProto(grpcResp), nil
}


