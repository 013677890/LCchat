package pb

import (
	"context"

	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// 这一组方法统一落在 user-service 相关 client 上，
// 保持 gateway facade 文件划分与下游服务边界一致。

// GetProfile 获取个人信息。
func (c *userServiceClientImpl) GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
	return c.userClient.GetProfile(ctx, req)
}

// GetOtherProfile 获取他人信息。
func (c *userServiceClientImpl) GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
	return c.userClient.GetOtherProfile(ctx, req)
}

// SearchUser 搜索用户。
func (c *userServiceClientImpl) SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
	return c.userClient.SearchUser(ctx, req)
}

// UpdateProfile 更新基本信息。
func (c *userServiceClientImpl) UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error) {
	return c.userClient.UpdateProfile(ctx, req)
}

// UploadAvatar 上传头像。
func (c *userServiceClientImpl) UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error) {
	return c.userClient.UploadAvatar(ctx, req)
}

// GetQRCode 获取用户二维码。
func (c *userServiceClientImpl) GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
	return c.userClient.GetQRCode(ctx, req)
}

// ParseQRCode 解析二维码。
func (c *userServiceClientImpl) ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error) {
	return c.userClient.ParseQRCode(ctx, req)
}

// BatchGetProfile 批量获取用户信息。
func (c *userServiceClientImpl) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	return c.userClient.BatchGetProfile(ctx, req)
}
