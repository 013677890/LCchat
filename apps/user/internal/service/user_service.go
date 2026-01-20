package service

import (
	"ChatServer/apps/user/internal/dto"
	"ChatServer/apps/user/internal/repository"
	"context"

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
func (s *userServiceImpl) GetProfile(ctx context.Context) (*dto.GetProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取个人信息功能暂未实现")
}

// GetOtherProfile 获取他人信息
func (s *userServiceImpl) GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取他人信息功能暂未实现")
}

// UpdateProfile 更新基本信息
func (s *userServiceImpl) UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "更新基本信息功能暂未实现")
}

// UploadAvatar 上传头像
func (s *userServiceImpl) UploadAvatar(ctx context.Context, req *dto.UploadAvatarRequest) (*dto.UploadAvatarResponse, error) {
	return nil, status.Error(codes.Unimplemented, "上传头像功能暂未实现")
}

// ChangePassword 修改密码
func (s *userServiceImpl) ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error {
	return status.Error(codes.Unimplemented, "修改密码功能暂未实现")
}

// ChangeEmail 绑定/换绑邮箱
func (s *userServiceImpl) ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	return nil, status.Error(codes.Unimplemented, "绑定/换绑邮箱功能暂未实现")
}

// ChangeTelephone 绑定/换绑手机
func (s *userServiceImpl) ChangeTelephone(ctx context.Context, req *dto.ChangeTelephoneRequest) (*dto.ChangeTelephoneResponse, error) {
	return nil, status.Error(codes.Unimplemented, "绑定/换绑手机功能暂未实现")
}

// GetQRCode 获取用户二维码
func (s *userServiceImpl) GetQRCode(ctx context.Context) (*dto.GetQRCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取用户二维码功能暂未实现")
}

// ParseQRCode 解析二维码
func (s *userServiceImpl) ParseQRCode(ctx context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "解析二维码功能暂未实现")
}

// DeleteAccount 注销账号
func (s *userServiceImpl) DeleteAccount(ctx context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
	return nil, status.Error(codes.Unimplemented, "注销账号功能暂未实现")
}

// BatchGetProfile 批量获取用户信息
func (s *userServiceImpl) BatchGetProfile(ctx context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "批量获取用户信息功能暂未实现")
}
