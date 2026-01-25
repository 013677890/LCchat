package service

import (
	"ChatServer/apps/user/internal/converter"
	"ChatServer/apps/user/internal/repository"
	"ChatServer/apps/user/internal/utils"
	pb "ChatServer/apps/user/pb"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
	"context"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// userServiceImpl 用户信息服务实现
type userServiceImpl struct {
	userRepo repository.IUserRepository
}

// NewUserService 创建用户信息服务实例
func NewUserService(userRepo repository.IUserRepository) UserService {
	return &userServiceImpl{
		userRepo: userRepo,
	}
}

// GetProfile 获取个人信息
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 查询用户信息
//  3. 转换为Protobuf格式并返回
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || userUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeUnauthorized))
	}

	// 2. 查询用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		logger.Error(ctx, "查询用户信息失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if userInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("user_uuid", userUUID),
		)
		return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
	}

	// 3. 转换为Protobuf格式并返回
	return &pb.GetProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(userInfo),
	}, nil
}

// GetOtherProfile 获取他人信息
// 业务流程：
//  1. 从context中获取当前用户UUID
//  2. 查询目标用户信息
//  3. 判断是否为好友关系
//  4. 非好友时脱敏邮箱和手机号
//  5. 返回用户信息
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
	// 1. 从context中获取当前用户UUID
	currentUserUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || currentUserUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeUnauthorized))
	}

	// 2. 查询目标用户信息
	targetUserInfo, err := s.userRepo.GetByUUID(ctx, req.UserUuid)
	if err != nil {
		logger.Error(ctx, "查询用户信息失败",
			logger.String("target_user_uuid", req.UserUuid),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if targetUserInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("target_user_uuid", req.UserUuid),
		)
		return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
	}

	// 3. 判断是否为好友关系（这里暂时使用friendRepo，如果未实现则先跳过）
	isFriend := false
	// TODO: 实现好友关系查询后启用以下代码
	// if s.friendRepo != nil {
	// 	isFriend, _ = s.friendRepo.IsFriend(ctx, currentUserUUID, req.UserUuid)
	// }

	// 4. 非好友时脱敏邮箱和手机号
	if !isFriend && targetUserInfo.Email != "" {
		// 脱敏邮箱：只显示前3位和@domain部分
		targetUserInfo.Email = utils.MaskEmail(targetUserInfo.Email)
	}
	if !isFriend && targetUserInfo.Telephone != "" {
		// 脱敏手机号：只显示前3位和后4位
		targetUserInfo.Telephone = utils.MaskPhone(targetUserInfo.Telephone)
	}

	// 5. 返回用户信息
	return &pb.GetOtherProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(targetUserInfo),
		IsFriend: isFriend,
	}, nil
}

// UpdateProfile 更新基本信息
func (s *userServiceImpl) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "更新基本信息功能暂未实现")
}

// UploadAvatar 上传头像
func (s *userServiceImpl) UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
	return nil, status.Error(codes.Unimplemented, "上传头像功能暂未实现")
}

// ChangePassword 修改密码
func (s *userServiceImpl) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) error {
	return status.Error(codes.Unimplemented, "修改密码功能暂未实现")
}

// ChangeEmail 绑定/换绑邮箱
func (s *userServiceImpl) ChangeEmail(ctx context.Context, req *pb.ChangeEmailRequest) (*pb.ChangeEmailResponse, error) {
	return nil, status.Error(codes.Unimplemented, "绑定/换绑邮箱功能暂未实现")
}

// ChangeTelephone 绑定/换绑手机
func (s *userServiceImpl) ChangeTelephone(ctx context.Context, req *pb.ChangeTelephoneRequest) (*pb.ChangeTelephoneResponse, error) {
	return nil, status.Error(codes.Unimplemented, "绑定/换绑手机功能暂未实现")
}

// GetQRCode 获取用户二维码
func (s *userServiceImpl) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取用户二维码功能暂未实现")
}

// ParseQRCode 解析二维码
func (s *userServiceImpl) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "解析二维码功能暂未实现")
}

// DeleteAccount 注销账号
func (s *userServiceImpl) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	return nil, status.Error(codes.Unimplemented, "注销账号功能暂未实现")
}

// BatchGetProfile 批量获取用户信息
func (s *userServiceImpl) BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "批量获取用户信息功能暂未实现")
}
