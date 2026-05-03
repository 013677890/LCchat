package service

import (
	"context"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// ==================== 用户信息服务接口 ====================

// IUserService 用户信息服务接口
// 职责：用户个人信息管理、头像、密码修改、账号设置、二维码、注销
type IUserService interface {
	// GetProfile 获取个人信息
	GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error)

	// GetOtherProfile 获取他人信息
	GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error)

	// SearchUser 搜索用户
	SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error)

	// UpdateProfile 更新基本信息
	UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error)

	// UploadAvatar 上传头像
	UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error)

	// GetQRCode 获取用户二维码
	GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error)

	// ParseQRCode 解析二维码
	ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error)

	// BatchGetProfile 批量获取用户信息
	BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error)
}

// IInternalProfileService 用户内部资料服务接口。
// 职责：为其他内部服务提供最小必要的资料视图。
type IInternalProfileService interface {
	// CreateProfile 创建或确认默认资料存在。
	CreateProfile(ctx context.Context, req *pb.CreateProfileRequest) (*pb.CreateProfileResponse, error)

	// BatchGetUserCard 批量获取用户卡片信息。
	BatchGetUserCard(ctx context.Context, req *pb.BatchGetUserCardRequest) (*pb.BatchGetUserCardResponse, error)

	// BatchGetPublicProfile 批量获取公开资料信息。
	BatchGetPublicProfile(ctx context.Context, req *pb.BatchGetPublicProfileRequest) (*pb.BatchGetPublicProfileResponse, error)
}

// ==================== 好友服务接口 ====================

// IFriendService 好友服务接口
// 职责：搜索用户、好友申请、好友列表、备注标签
type IFriendService interface {
	// SendFriendApply 发送好友申请
	SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error)

	// GetFriendApplyList 获取好友申请列表
	GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error)

	// GetSentApplyList 获取发出的申请列表
	GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error)

	// HandleFriendApply 处理好友申请
	HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error

	// GetUnreadApplyCount 获取未读申请数量
	GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error)

	// MarkApplyAsRead 标记申请已读
	MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error

	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error)

	// SyncFriendList 好友增量同步
	SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error)

	// DeleteFriend 删除好友
	DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error

	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error

	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error

	// GetTagList 获取标签列表
	GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error)

	// CheckIsFriend 判断是否好友
	CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error)

	// BatchCheckIsFriend 批量判断是否好友
	BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error)

	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error)
}

// ==================== 黑名单服务接口 ====================

// IBlacklistService 黑名单服务接口
// 职责：拉黑、取消拉黑、黑名单列表、判断是否拉黑
type IBlacklistService interface {
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) error

	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) error

	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error)

	// CheckIsBlacklist 判断是否拉黑
	CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error)
}

type IGroupService interface {
	// GetGroupMembers 获取群组有效成员列表。
	GetGroupMembers(ctx context.Context, req *pb.GetGroupMembersRequest) (*pb.GetGroupMembersResponse, error)
}

// ==================== 别名类型定义（用于向后兼容）====================

// UserService 别名 IUserService
type UserService = IUserService

// InternalProfileService 别名 IInternalProfileService
type InternalProfileService = IInternalProfileService

// FriendService 别名 IFriendService
type FriendService = IFriendService

// BlacklistService 别名 IBlacklistService
type BlacklistService = IBlacklistService

// GroupService 别名 IGroupService
type GroupService = IGroupService
