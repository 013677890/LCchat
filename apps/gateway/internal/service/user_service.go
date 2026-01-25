package service

import (
	"ChatServer/apps/gateway/internal/dto"
	"ChatServer/apps/gateway/internal/pb"
	"ChatServer/apps/gateway/internal/utils"
	userpb "ChatServer/apps/user/pb"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
	"context"
	"errors"
	"strconv"
	"time"
)

// UserService 用户服务接口
type UserService interface {
	// GetProfile 获取个人信息
	GetProfile(ctx context.Context) (*dto.GetProfileResponse, error)
	// GetOtherProfile 获取他人信息
	GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error)
}

// UserServiceImpl 用户服务实现
type UserServiceImpl struct {
	userClient pb.UserServiceClient
}

// NewUserService 创建用户服务实例
// userClient: 用户服务 gRPC 客户端
func NewUserService(userClient pb.UserServiceClient) UserService {
	return &UserServiceImpl{
		userClient: userClient,
	}
}

// GetProfile 获取个人信息
// ctx: 请求上下文
// 返回: 个人信息响应
func (s *UserServiceImpl) GetProfile(ctx context.Context) (*dto.GetProfileResponse, error) {
	startTime := time.Now()

	// 1. 调用用户服务获取个人信息(gRPC)
	grpcReq := &userpb.GetProfileRequest{}
	grpcResp, err := s.userClient.GetProfile(ctx, grpcReq)
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

	// 2. gRPC 调用成功，检查响应数据
	if grpcResp.UserInfo == nil {
		// 成功返回但 UserInfo 为空，属于非预期的异常情况
		logger.Error(ctx, "gRPC 成功响应但用户信息为空")
		return nil, errors.New(strconv.Itoa(consts.CodeInternalError))
	}

	return dto.ConvertGetProfileResponseFromProto(grpcResp), nil
}

// GetOtherProfile 获取他人信息
// ctx: 请求上下文
// req: 获取他人信息请求
// 返回: 他人信息响应
func (s *UserServiceImpl) GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoGetOtherProfileRequest(req)

	// 2. 调用用户服务获取他人信息(gRPC)
	grpcResp, err := s.userClient.GetOtherProfile(ctx, grpcReq)
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

	return dto.ConvertGetOtherProfileResponseFromProto(grpcResp), nil
}
