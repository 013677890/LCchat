package pb

import (
	userpb "ChatServer/apps/user/pb"
	"context"
)

// UserServiceClient 用户服务 gRPC 客户端接口
// 职责：封装对用户服务的 gRPC 调用
type UserServiceClient interface {
	// ==================== 认证服务 ====================
	// Login 用户登录
	Login(ctx context.Context, req *userpb.LoginRequest) (*userpb.LoginResponse, error)

	// LoginByCode 验证码登录
	LoginByCode(ctx context.Context, req *userpb.LoginByCodeRequest) (*userpb.LoginByCodeResponse, error)

	// SendVerifyCode 发送验证码
	SendVerifyCode(ctx context.Context, req *userpb.SendVerifyCodeRequest) (*userpb.SendVerifyCodeResponse, error)

	// VerifyCode 校验验证码
	VerifyCode(ctx context.Context, req *userpb.VerifyCodeRequest) (*userpb.VerifyCodeResponse, error)

	// RefreshToken 刷新Token
	RefreshToken(ctx context.Context, req *userpb.RefreshTokenRequest) (*userpb.RefreshTokenResponse, error)

	// Logout 用户登出
	Logout(ctx context.Context, req *userpb.LogoutRequest) (*userpb.LogoutResponse, error)

	// ResetPassword 重置密码
	ResetPassword(ctx context.Context, req *userpb.ResetPasswordRequest) (*userpb.ResetPasswordResponse, error)

	// ==================== 用户信息服务 ====================
	// GetProfile 获取个人信息
	GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error)

	// GetOtherProfile 获取他人信息
	GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error)

	// UpdateProfile 更新基本信息
	UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error)

	// UploadAvatar 上传头像
	UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error)

	// ChangePassword 修改密码
	ChangePassword(ctx context.Context, req *userpb.ChangePasswordRequest) (*userpb.ChangePasswordResponse, error)

	// ChangeEmail 绑定/换绑邮箱
	ChangeEmail(ctx context.Context, req *userpb.ChangeEmailRequest) (*userpb.ChangeEmailResponse, error)

	// ChangeTelephone 绑定/换绑手机
	ChangeTelephone(ctx context.Context, req *userpb.ChangeTelephoneRequest) (*userpb.ChangeTelephoneResponse, error)

	// GetQRCode 获取用户二维码
	GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error)

	// ParseQRCode 解析二维码
	ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error)

	// DeleteAccount 注销账号
	DeleteAccount(ctx context.Context, req *userpb.DeleteAccountRequest) (*userpb.DeleteAccountResponse, error)

	// BatchGetProfile 批量获取用户信息
	BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error)

	// ==================== 好友服务 ====================
	// SearchUser 搜索用户
	SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error)

	// SendFriendApply 发送好友申请
	SendFriendApply(ctx context.Context, req *userpb.SendFriendApplyRequest) (*userpb.SendFriendApplyResponse, error)

	// GetFriendApplyList 获取好友申请列表
	GetFriendApplyList(ctx context.Context, req *userpb.GetFriendApplyListRequest) (*userpb.GetFriendApplyListResponse, error)

	// GetSentApplyList 获取发出的申请列表
	GetSentApplyList(ctx context.Context, req *userpb.GetSentApplyListRequest) (*userpb.GetSentApplyListResponse, error)

	// HandleFriendApply 处理好友申请
	HandleFriendApply(ctx context.Context, req *userpb.HandleFriendApplyRequest) (*userpb.HandleFriendApplyResponse, error)

	// GetUnreadApplyCount 获取未读申请数量
	GetUnreadApplyCount(ctx context.Context, req *userpb.GetUnreadApplyCountRequest) (*userpb.GetUnreadApplyCountResponse, error)

	// MarkApplyAsRead 标记申请已读
	MarkApplyAsRead(ctx context.Context, req *userpb.MarkApplyAsReadRequest) (*userpb.MarkApplyAsReadResponse, error)

	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, req *userpb.GetFriendListRequest) (*userpb.GetFriendListResponse, error)

	// SyncFriendList 好友增量同步
	SyncFriendList(ctx context.Context, req *userpb.SyncFriendListRequest) (*userpb.SyncFriendListResponse, error)

	// DeleteFriend 删除好友
	DeleteFriend(ctx context.Context, req *userpb.DeleteFriendRequest) (*userpb.DeleteFriendResponse, error)

	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, req *userpb.SetFriendRemarkRequest) (*userpb.SetFriendRemarkResponse, error)

	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, req *userpb.SetFriendTagRequest) (*userpb.SetFriendTagResponse, error)

	// GetTagList 获取标签列表
	GetTagList(ctx context.Context, req *userpb.GetTagListRequest) (*userpb.GetTagListResponse, error)

	// CheckIsFriend 判断是否好友
	CheckIsFriend(ctx context.Context, req *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error)

	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, req *userpb.GetRelationStatusRequest) (*userpb.GetRelationStatusResponse, error)

	// ==================== 黑名单服务 ====================
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, req *userpb.AddBlacklistRequest) (*userpb.AddBlacklistResponse, error)

	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, req *userpb.RemoveBlacklistRequest) (*userpb.RemoveBlacklistResponse, error)

	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, req *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error)

	// CheckIsBlacklist 判断是否拉黑
	CheckIsBlacklist(ctx context.Context, req *userpb.CheckIsBlacklistRequest) (*userpb.CheckIsBlacklistResponse, error)

	// ==================== 设备会话服务 ====================
	// GetDeviceList 获取设备列表
	GetDeviceList(ctx context.Context, req *userpb.GetDeviceListRequest) (*userpb.GetDeviceListResponse, error)

	// KickDevice 踢出设备
	KickDevice(ctx context.Context, req *userpb.KickDeviceRequest) (*userpb.KickDeviceResponse, error)

	// GetOnlineStatus 获取用户在线状态
	GetOnlineStatus(ctx context.Context, req *userpb.GetOnlineStatusRequest) (*userpb.GetOnlineStatusResponse, error)

	// BatchGetOnlineStatus 批量获取在线状态
	BatchGetOnlineStatus(ctx context.Context, req *userpb.BatchGetOnlineStatusRequest) (*userpb.BatchGetOnlineStatusResponse, error)
}
