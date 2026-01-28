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
	// UpdateProfile 更新基本信息
	UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error)
	// ChangePassword 修改密码
	ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error
	// ChangeEmail 绑定/换绑邮箱
	ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error)
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

	// 1. 从context中获取当前用户UUID
	currentUserUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || currentUserUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return nil, errors.New(strconv.Itoa(consts.CodeUnauthorized))
	}

	// 2. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoGetOtherProfileRequest(req)

	// 3. 并发调用用户服务和好友服务获取信息
	// 使用goroutine并发调用两个服务
	type userResult struct {
		resp *userpb.GetOtherProfileResponse
		err  error
	}
	type friendResult struct {
		resp *userpb.CheckIsFriendResponse
		err  error
	}

	userChan := make(chan userResult, 1)
	friendChan := make(chan friendResult, 1)

	// 并发调用用户服务
	go func() {
		grpcResp, err := s.userClient.GetOtherProfile(ctx, grpcReq)
		userChan <- userResult{resp: grpcResp, err: err}
	}()

	// 并发调用好友服务判断是否为好友
	go func() {
		friendReq := &userpb.CheckIsFriendRequest{
			UserUuid: currentUserUUID,
			PeerUuid: req.UserUUID,
		}
		friendResp, err := s.userClient.CheckIsFriend(ctx, friendReq)
		friendChan <- friendResult{resp: friendResp, err: err}
	}()

	// 等待两个服务调用完成
	userRes := <-userChan
	friendRes := <-friendChan

	// 4. 检查用户服务调用结果
	if userRes.err != nil {
		// gRPC 调用失败，提取业务错误码
		code := utils.ExtractErrorCode(userRes.err)
		// 记录错误日志
		logger.Error(ctx, "调用用户服务 gRPC 失败",
			logger.ErrorField("error", userRes.err),
			logger.Int("business_code", code),
			logger.String("business_message", consts.GetMessage(code)),
			logger.Duration("duration", time.Since(startTime)),
		)
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, userRes.err
	}

	// 5. gRPC 调用成功，检查响应数据
	if userRes.resp.UserInfo == nil {
		// 成功返回但 UserInfo 为空，属于非预期的异常情况
		logger.Error(ctx, "gRPC 成功响应但用户信息为空")
		return nil, errors.New(strconv.Itoa(consts.CodeInternalError))
	}

	// 6. 检查好友服务调用结果（非关键错误，只记录日志）
	isFriend := false
	if friendRes.err != nil {
		logger.Warn(ctx, "调用好友服务失败",
			logger.ErrorField("error", friendRes.err),
		)
	} else if friendRes.resp != nil {
		isFriend = friendRes.resp.IsFriend
	}

	// 7. 非好友时脱敏邮箱和手机号
	userInfo := userRes.resp.UserInfo
	if !isFriend && userInfo.Email != "" {
		// 脱敏邮箱：只显示前3位和@domain部分
		userInfo.Email = utils.MaskEmail(userInfo.Email)
	}
	if !isFriend && userInfo.Telephone != "" {
		// 脱敏手机号：只显示前3位和后4位
		userInfo.Telephone = utils.MaskTelephone(userInfo.Telephone)
	}

	// 8. 返回用户信息
	return dto.ConvertGetOtherProfileResponseFromProto(userRes.resp, isFriend), nil
}

// UpdateProfile 更新基本信息
// ctx: 请求上下文
// req: 更新基本信息请求
// 返回: 更新后的个人信息响应
func (s *UserServiceImpl) UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoUpdateProfileRequest(req)

	// 2. 调用用户服务更新基本信息(gRPC)
	grpcResp, err := s.userClient.UpdateProfile(ctx, grpcReq)
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

	return dto.ConvertUpdateProfileResponseFromProto(grpcResp), nil
}

// ChangePassword 修改密码
// ctx: 请求上下文
// req: 修改密码请求
// 返回: 错误
func (s *UserServiceImpl) ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoChangePasswordRequest(req)

	// 2. 调用用户服务修改密码(gRPC)
	_, err := s.userClient.ChangePassword(ctx, grpcReq)
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
		return err
	}

	return nil
}

// ChangeEmail 绑定/换绑邮箱
// ctx: 请求上下文
// req: 换绑邮箱请求
// 返回: 换绑邮箱响应
func (s *UserServiceImpl) ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	startTime := time.Now()

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoChangeEmailRequest(req)

	// 2. 调用用户服务换绑邮箱(gRPC)
	grpcResp, err := s.userClient.ChangeEmail(ctx, grpcReq)
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

	return dto.ConvertChangeEmailResponseFromProto(grpcResp), nil
}
