package service

import (
	"context"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
)

// AuthService 认证服务接口
// 职责：
//   - 调用下游用户服务进行认证
//   - 生成访问令牌（Access Token 和 Refresh Token）
type AuthService interface {
	// Login 用户登录
	// ctx: 请求上下文
	// req: 登录请求
	// deviceId: 设备唯一标识
	// 返回: 完整的登录响应（包含Token和用户信息）
	Login(ctx context.Context, req *dto.LoginRequest, deviceId string) (*dto.LoginResponse, error)
	// Register 用户注册
	// ctx: 请求上下文
	// req: 注册请求
	// 返回: 完整的注册响应（包含Token和用户信息）
	Register(ctx context.Context, req *dto.RegisterRequest) (*dto.RegisterResponse, error)
	// SendVerifyCode 发送验证码
	// ctx: 请求上下文
	// req: 发送验证码请求
	// 返回: 发送验证码响应
	SendVerifyCode(ctx context.Context, req *dto.SendVerifyCodeRequest) (*dto.SendVerifyCodeResponse, error)
	// LoginByCode 验证码登录
	// ctx: 请求上下文
	// req: 验证码登录请求
	// deviceId: 设备唯一标识
	// 返回: 完整的登录响应（包含Token和用户信息）
	LoginByCode(ctx context.Context, req *dto.LoginByCodeRequest, deviceId string) (*dto.LoginByCodeResponse, error)
	// Logout 用户登出
	// ctx: 请求上下文
	// req: 登出请求
	// 返回: 登出响应
	Logout(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error)
	// ResetPassword 重置密码
	// ctx: 请求上下文
	// req: 重置密码请求
	// 返回: 重置密码响应
	ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.ResetPasswordResponse, error)
	// RefreshToken 刷新Token
	// ctx: 请求上下文
	// req: 刷新Token请求
	// 返回: 刷新Token响应
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error)
	// VerifyCode 校验验证码
	// ctx: 请求上下文
	// req: 校验验证码请求
	// 返回: 校验验证码响应
	VerifyCode(ctx context.Context, req *dto.VerifyCodeRequest) (*dto.VerifyCodeResponse, error)
}

// FriendService 好友服务接口
// 职责：
//   - 调用下游用户服务进行好友相关操作
type FriendService interface {
	// SendFriendApply 发送好友申请
	SendFriendApply(ctx context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error)
	// GetFriendApplyList 获取好友申请列表
	GetFriendApplyList(ctx context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error)
	// GetSentApplyList 获取发出的申请列表
	GetSentApplyList(ctx context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error)
	// HandleFriendApply 处理好友申请
	HandleFriendApply(ctx context.Context, req *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error)
	// GetUnreadApplyCount 获取未读申请数量
	GetUnreadApplyCount(ctx context.Context, req *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error)
	// MarkApplyAsRead 标记申请已读
	MarkApplyAsRead(ctx context.Context, req *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error)
	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error)
	// SyncFriendList 好友增量同步
	SyncFriendList(ctx context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error)
	// DeleteFriend 删除好友
	DeleteFriend(ctx context.Context, req *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error)
	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, req *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error)
	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, req *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error)
	// GetTagList 获取标签列表
	GetTagList(ctx context.Context, req *dto.GetTagListRequest) (*dto.GetTagListResponse, error)
	// CheckIsFriend 判断是否好友
	CheckIsFriend(ctx context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error)
	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, req *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error)
}

// BlacklistService 黑名单服务接口
// 职责：
//   - 调用下游用户服务进行黑名单相关操作
type BlacklistService interface {
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, req *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error)
	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, req *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error)
	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error)
	// CheckIsBlacklist 判断是否拉黑
	CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error)
}

// DeviceService 设备服务接口
// 职责：
//   - 调用下游用户服务进行设备相关操作
type DeviceService interface {
	// GetDeviceList 获取设备列表
	GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error)
	// KickDevice 踢出设备
	KickDevice(ctx context.Context, req *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error)
	// GetOnlineStatus 获取用户在线状态
	GetOnlineStatus(ctx context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error)
	// BatchGetOnlineStatus 批量获取在线状态
	BatchGetOnlineStatus(ctx context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error)
}

// UserService 用户服务接口
type UserService interface {
	// GetProfile 获取个人信息
	GetProfile(ctx context.Context) (*dto.GetProfileResponse, error)
	// GetOtherProfile 获取他人信息
	GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error)
	// SearchUser 搜索用户
	SearchUser(ctx context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error)
	// UpdateProfile 更新基本信息
	UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error)
	// UploadAvatar 上传头像
	UploadAvatar(ctx context.Context, avatarURL string) (string, error)
	// ChangePassword 修改密码
	ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error
	// ChangeEmail 绑定/换绑邮箱
	ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error)
	// GetQRCode 获取用户二维码
	GetQRCode(ctx context.Context) (*dto.GetQRCodeResponse, error)
	// ParseQRCode 解析二维码
	ParseQRCode(ctx context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error)
	// BatchGetProfile 批量获取用户信息
	BatchGetProfile(ctx context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error)
	// DeleteAccount 注销账号
	DeleteAccount(ctx context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error)
}

// MsgService 消息服务接口
// 职责：
//   - 调用下游消息服务进行消息和会话相关操作
type MsgService interface {
	// SendMessage 发送消息
	SendMessage(ctx context.Context, req *dto.SendMessageRequest) (*dto.SendMessageResponse, error)
	// PullMessages 拉取历史消息
	PullMessages(ctx context.Context, req *dto.PullMessagesRequest) (*dto.PullMessagesResponse, error)
	// GetMessagesByIds 批量获取指定消息
	GetMessagesByIds(ctx context.Context, req *dto.GetMessagesByIdsRequest) (*dto.GetMessagesByIdsResponse, error)
	// RecallMessage 撤回消息
	RecallMessage(ctx context.Context, req *dto.RecallMessageRequest) error
	// GetConversations 获取会话列表
	GetConversations(ctx context.Context, req *dto.GetConversationsRequest) (*dto.GetConversationsResponse, error)
	// MarkRead 标记会话已读
	MarkRead(ctx context.Context, req *dto.MarkReadRequest) (*dto.MarkReadResponse, error)
	// DeleteConversation 删除会话
	DeleteConversation(ctx context.Context, req *dto.DeleteConversationRequest) error
	// UpdateConversationSettings 更新会话设置
	UpdateConversationSettings(ctx context.Context, req *dto.UpdateConvSettingsRequest) error
}

// GroupService 群组服务接口。
// 职责：调用下游 group-service 暴露群资料与成员写读能力。
type GroupService interface {
	// CreateGroup 创建群。
	CreateGroup(ctx context.Context, req *dto.CreateGroupRequest) (*dto.CreateGroupResponse, error)
	// DismissGroup 解散群。
	DismissGroup(ctx context.Context, req *dto.DismissGroupRequest) error
	// GetGroupInfo 获取群资料。
	GetGroupInfo(ctx context.Context, req *dto.GetGroupInfoRequest) (*dto.GroupInfoDTO, error)
	// UpdateGroupInfo 更新群资料。
	UpdateGroupInfo(ctx context.Context, req *dto.UpdateGroupInfoRequest) error
	// UpdateGroupNotice 独立更新群公告。
	UpdateGroupNotice(ctx context.Context, req *dto.UpdateGroupNoticeRequest) error
	// TransferGroupOwner 转让群主。
	TransferGroupOwner(ctx context.Context, req *dto.TransferGroupOwnerRequest) error
	// UpdateMemberRole 更新群成员角色。
	UpdateMemberRole(ctx context.Context, req *dto.UpdateGroupMemberRoleRequest) error
	// ApplyJoinGroup 申请加入群聊。
	ApplyJoinGroup(ctx context.Context, req *dto.ApplyJoinGroupRequest) (*dto.ApplyJoinGroupResponse, error)
	// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
	CancelJoinGroupApplication(ctx context.Context, req *dto.CancelJoinGroupApplicationRequest) error
	// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
	GetMyJoinGroupApplication(ctx context.Context, req *dto.GetMyJoinGroupApplicationRequest) (*dto.GetMyJoinGroupApplicationResponse, error)
	// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
	ListMyJoinGroupApplications(ctx context.Context, req *dto.ListMyJoinGroupApplicationsRequest) (*dto.ListMyJoinGroupApplicationsResponse, error)
	// ReviewJoinGroup 审批入群申请。
	ReviewJoinGroup(ctx context.Context, req *dto.ReviewJoinGroupRequest) error
	// ListJoinRequests 获取待审批入群申请列表。
	ListJoinRequests(ctx context.Context, req *dto.ListJoinRequestsRequest) (*dto.ListJoinRequestsResponse, error)
	// ListReviewedJoinRequests 获取群已审批申请列表。
	ListReviewedJoinRequests(ctx context.Context, req *dto.ListReviewedJoinRequestsRequest) (*dto.ListReviewedJoinRequestsResponse, error)
	// AddMember 添加群成员。
	AddMember(ctx context.Context, req *dto.AddGroupMemberRequest) error
	// LeaveGroup 当前用户主动退出群聊。
	LeaveGroup(ctx context.Context, req *dto.LeaveGroupRequest) error
	// RemoveMember 移除群成员。
	RemoveMember(ctx context.Context, req *dto.RemoveGroupMemberRequest) error
	// GetMemberList 获取群成员列表。
	GetMemberList(ctx context.Context, req *dto.GetGroupMemberListRequest) (*dto.GetGroupMemberListResponse, error)
	// SearchGroupMembers 搜索群成员。
	SearchGroupMembers(ctx context.Context, req *dto.SearchGroupMembersRequest) (*dto.SearchGroupMembersResponse, error)
	// UpdateMyGroupNickname 更新当前用户自己的群名片。
	UpdateMyGroupNickname(ctx context.Context, req *dto.UpdateMyGroupNicknameRequest) error
	// MuteGroupMember 设置或取消成员单人禁言。
	MuteGroupMember(ctx context.Context, req *dto.MuteGroupMemberRequest) error
	// UpdateGroupMuteSetting 更新全员禁言开关。
	UpdateGroupMuteSetting(ctx context.Context, req *dto.UpdateGroupMuteSettingRequest) error
	// GetGroupList 获取当前用户的群列表。
	GetGroupList(ctx context.Context) (*dto.GetGroupListResponse, error)
	// GetGroupMemberIDs 获取群成员 UUID 列表。
	GetGroupMemberIDs(ctx context.Context, req *dto.GetGroupMemberIDsRequest) (*dto.GetGroupMemberIDsResponse, error)
	// GetJoinRequestPendingCount 获取群待审批入群申请数量。
	GetJoinRequestPendingCount(ctx context.Context, req *dto.GetJoinRequestPendingCountRequest) (*dto.GetJoinRequestPendingCountResponse, error)
}
