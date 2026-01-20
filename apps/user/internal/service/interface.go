package service

import (
	"ChatServer/apps/user/internal/dto"
	"context"
)

// ==================== 认证服务接口 ====================

// IAuthService 认证服务接口
// 职责：用户注册、登录、登出、Token管理、验证码
type IAuthService interface {
	// Register 用户注册
	Register(ctx context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error)

	// Login 用户登录（密码）
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)

	// LoginByCode 验证码登录
	LoginByCode(ctx context.Context, req *dto.LoginByCodeRequest) (*dto.LoginByCodeResponse, error)

	// SendVerifyCode 发送验证码
	SendVerifyCode(ctx context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error)

	// VerifyCode 校验验证码
	VerifyCode(ctx context.Context, req *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error)

	// RefreshToken 刷新Token
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error)

	// Logout 用户登出
	Logout(ctx context.Context, req *dto.LogoutRequest) error

	// ResetPassword 重置密码
	ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) error
}

// ==================== 用户信息服务接口 ====================

// IUserService 用户信息服务接口
// 职责：用户个人信息管理、头像、密码修改、账号设置、二维码、注销
type IUserService interface {
	// GetProfile 获取个人信息
	GetProfile(ctx context.Context) (*dto.GetProfileResponse, error)

	// GetOtherProfile 获取他人信息
	GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error)

	// UpdateProfile 更新基本信息
	UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error)

	// UploadAvatar 上传头像
	UploadAvatar(ctx context.Context, req *dto.UploadAvatarRequest) (*dto.UploadAvatarResponse, error)

	// ChangePassword 修改密码
	ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error

	// ChangeEmail 绑定/换绑邮箱
	ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error)

	// ChangeTelephone 绑定/换绑手机
	ChangeTelephone(ctx context.Context, req *dto.ChangeTelephoneRequest) (*dto.ChangeTelephoneResponse, error)

	// GetQRCode 获取用户二维码
	GetQRCode(ctx context.Context) (*dto.GetQRCodeResponse, error)

	// ParseQRCode 解析二维码
	ParseQRCode(ctx context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error)

	// DeleteAccount 注销账号
	DeleteAccount(ctx context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error)

	// BatchGetProfile 批量获取用户信息
	BatchGetProfile(ctx context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error)
}

// ==================== 好友服务接口 ====================

// IFriendService 好友服务接口
// 职责：搜索用户、好友申请、好友列表、备注标签
type IFriendService interface {
	// SearchUser 搜索用户
	SearchUser(ctx context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error)

	// SendFriendApply 发送好友申请
	SendFriendApply(ctx context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error)

	// GetFriendApplyList 获取好友申请列表
	GetFriendApplyList(ctx context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error)

	// GetSentApplyList 获取发出的申请列表
	GetSentApplyList(ctx context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error)

	// HandleFriendApply 处理好友申请
	HandleFriendApply(ctx context.Context, req *dto.HandleFriendApplyRequest) error

	// GetUnreadApplyCount 获取未读申请数量
	GetUnreadApplyCount(ctx context.Context) (*dto.GetUnreadApplyCountResponse, error)

	// MarkApplyAsRead 标记申请已读
	MarkApplyAsRead(ctx context.Context, req *dto.MarkApplyAsReadRequest) error

	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error)

	// SyncFriendList 好友增量同步
	SyncFriendList(ctx context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error)

	// DeleteFriend 删除好友
	DeleteFriend(ctx context.Context, req *dto.DeleteFriendRequest) error

	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, req *dto.SetFriendRemarkRequest) error

	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, req *dto.SetFriendTagRequest) error

	// GetTagList 获取标签列表
	GetTagList(ctx context.Context) (*dto.GetTagListResponse, error)

	// CheckIsFriend 判断是否好友
	CheckIsFriend(ctx context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error)

	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, req *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error)
}

// ==================== 黑名单服务接口 ====================

// IBlacklistService 黑名单服务接口
// 职责：拉黑、取消拉黑、黑名单列表、判断是否拉黑
type IBlacklistService interface {
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, req *dto.AddBlacklistRequest) error

	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, req *dto.RemoveBlacklistRequest) error

	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error)

	// CheckIsBlacklist 判断是否拉黑
	CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error)
}

// ==================== 设备会话服务接口 ====================

// IDeviceService 设备会话服务接口
// 职责：设备列表、踢出设备、在线状态查询
type IDeviceService interface {
	// GetDeviceList 获取设备列表
	GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error)

	// KickDevice 踢出设备
	KickDevice(ctx context.Context, req *dto.KickDeviceRequestNew) error

	// GetOnlineStatus 获取用户在线状态
	GetOnlineStatus(ctx context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error)

	// BatchGetOnlineStatus 批量获取在线状态
	BatchGetOnlineStatus(ctx context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error)
}

// ==================== 别名类型定义（用于向后兼容）====================

// AuthService 别名 IAuthService
type AuthService = IAuthService

// UserService 别名 IUserService
type UserService = IUserService

// FriendService 别名 IFriendService
type FriendService = IFriendService

// BlacklistService 别名 IBlacklistService
type BlacklistService = IBlacklistService

// DeviceService 别名 IDeviceService
type DeviceService = IDeviceService
