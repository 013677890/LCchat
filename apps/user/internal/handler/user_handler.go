package handler

import (
	"ChatServer/apps/user/internal/service"
	pb "ChatServer/apps/user/pb"
	"context"
)

// UserHandler 用户信息服务Handler
type UserHandler struct {
	pb.UnimplementedUserServiceServer

	userService service.IUserService
}

// NewUserHandler 创建用户信息Handler实例
func NewUserHandler(authService service.IAuthService, userService service.IUserService, friendService service.IFriendService, deviceService service.IDeviceService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// GetProfile 获取个人信息
func (h *UserHandler) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	return nil, nil
}

// GetOtherProfile 获取他人信息
func (h *UserHandler) GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
	return nil, nil
}

// UpdateProfile 更新基本信息
func (h *UserHandler) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	return nil, nil
}

// UploadAvatar 上传头像
func (h *UserHandler) UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
	return nil, nil
}

// ChangePassword 修改密码
func (h *UserHandler) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	return nil, nil
}

// ChangeEmail 绑定/换绑邮箱
func (h *UserHandler) ChangeEmail(ctx context.Context, req *pb.ChangeEmailRequest) (*pb.ChangeEmailResponse, error) {
	return nil, nil
}

// ChangeTelephone 绑定/换绑手机
func (h *UserHandler) ChangeTelephone(ctx context.Context, req *pb.ChangeTelephoneRequest) (*pb.ChangeTelephoneResponse, error) {
	return nil, nil
}

// GetQRCode 获取用户二维码
func (h *UserHandler) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	return nil, nil
}

// ParseQRCode 解析二维码
func (h *UserHandler) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	return nil, nil
}

// DeleteAccount 注销账号
func (h *UserHandler) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	return nil, nil
}

// BatchGetProfile 批量获取用户信息
func (h *UserHandler) BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
	return nil, nil
}
