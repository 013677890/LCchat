package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// UserHandler 用户信息服务Handler
type UserHandler struct {
	pb.UnimplementedUserServiceServer

	userService service.IUserService
}

// NewUserHandler 创建用户信息Handler实例
func NewUserHandler(userService service.IUserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// GetProfile 获取个人信息
func (h *UserHandler) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	return h.userService.GetProfile(ctx, req)
}

// GetOtherProfile 获取他人信息
func (h *UserHandler) GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
	return h.userService.GetOtherProfile(ctx, req)
}

// SearchUser 搜索用户
func (h *UserHandler) SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
	return h.userService.SearchUser(ctx, req)
}

// UpdateProfile 更新基本信息
func (h *UserHandler) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	return h.userService.UpdateProfile(ctx, req)
}

// UploadAvatar 上传头像
func (h *UserHandler) UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
	return h.userService.UploadAvatar(ctx, req)
}

// GetQRCode 获取用户二维码
func (h *UserHandler) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	return h.userService.GetQRCode(ctx, req)
}

// ParseQRCode 解析二维码
func (h *UserHandler) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	return h.userService.ParseQRCode(ctx, req)
}

// BatchGetProfile 批量获取用户信息
func (h *UserHandler) BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
	return h.userService.BatchGetProfile(ctx, req)
}
