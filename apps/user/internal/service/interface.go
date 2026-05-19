package service

import (
	"context"

	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// UserService 资料域对外服务接口。
type UserService interface {
	// GetProfile 获取个人信息。
	GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error)

	// GetOtherProfile 获取他人信息。
	GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error)

	// SearchUser 搜索用户。
	SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error)

	// UpdateProfile 更新基本信息。
	UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error)

	// UploadAvatar 上传头像。
	UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error)

	// GetQRCode 获取用户二维码。
	GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error)

	// ParseQRCode 解析二维码。
	ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error)

	// BatchGetProfile 批量获取用户信息。
	BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error)
}

// InternalProfileService 为其他内部服务提供最小必要的资料视图。
type InternalProfileService interface {
	// CreateProfile 创建或确认默认资料存在。
	CreateProfile(ctx context.Context, req *pb.CreateProfileRequest) (*pb.CreateProfileResponse, error)

	// BatchGetUserCard 批量获取用户卡片信息。
	BatchGetUserCard(ctx context.Context, req *pb.BatchGetUserCardRequest) (*pb.BatchGetUserCardResponse, error)

	// BatchGetPublicProfile 批量获取公开资料信息。
	BatchGetPublicProfile(ctx context.Context, req *pb.BatchGetPublicProfileRequest) (*pb.BatchGetPublicProfileResponse, error)
}
