package pb

import (
	"context"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// MsgServiceClient 消息服务 gRPC 客户端接口
// 职责：封装对消息服务的 gRPC 调用
type MsgServiceClient interface {
	// ==================== 消息发送 ====================
	// SendMessage 发送消息
	SendMessage(ctx context.Context, req *msgpb.SendMessageRequest) (*msgpb.SendMessageResponse, error)
	// ==================== 消息拉取 ====================
	// PullMessages 按会话增量拉取历史消息
	PullMessages(ctx context.Context, req *msgpb.PullMessagesRequest) (*msgpb.PullMessagesResponse, error)
	// GetMessagesByIds 批量获取指定消息
	GetMessagesByIds(ctx context.Context, req *msgpb.GetMessagesByIdsRequest) (*msgpb.GetMessagesByIdsResponse, error)
	// ==================== 消息操作 ====================
	// RecallMessage 撤回消息
	RecallMessage(ctx context.Context, req *msgpb.RecallMessageRequest) (*msgpb.RecallMessageResponse, error)
	// ==================== 会话管理 ====================
	// GetConversations 获取会话列表
	GetConversations(ctx context.Context, req *msgpb.GetConversationsRequest) (*msgpb.GetConversationsResponse, error)
	// MarkRead 标记会话已读
	MarkRead(ctx context.Context, req *msgpb.MarkReadRequest) (*msgpb.MarkReadResponse, error)
	// DeleteConversation 删除会话
	DeleteConversation(ctx context.Context, req *msgpb.DeleteConversationRequest) (*msgpb.DeleteConversationResponse, error)
	// UpdateConversationSettings 更新会话设置
	UpdateConversationSettings(ctx context.Context, req *msgpb.UpdateConvSettingsRequest) (*msgpb.UpdateConvSettingsResponse, error)
}

// GroupServiceClient 群组服务 gRPC 客户端接口。
// 职责：封装 gateway 对 group-service 的出站调用。
type GroupServiceClient interface {
	// CreateGroup 创建群。
	CreateGroup(ctx context.Context, req *grouppb.CreateGroupRequest) (*grouppb.CreateGroupResponse, error)
	// DismissGroup 解散群。
	DismissGroup(ctx context.Context, req *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error)
	// GetGroupInfo 获取群资料。
	GetGroupInfo(ctx context.Context, req *grouppb.GetGroupInfoRequest) (*grouppb.GetGroupInfoResponse, error)
	// UpdateGroupInfo 更新群资料。
	UpdateGroupInfo(ctx context.Context, req *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error)
	// UpdateGroupNotice 独立更新群公告。
	UpdateGroupNotice(ctx context.Context, req *grouppb.UpdateGroupNoticeRequest) (*grouppb.UpdateGroupNoticeResponse, error)
	// TransferGroupOwner 转让群主。
	TransferGroupOwner(ctx context.Context, req *grouppb.TransferGroupOwnerRequest) (*grouppb.TransferGroupOwnerResponse, error)
	// UpdateMemberRole 更新群成员角色。
	UpdateMemberRole(ctx context.Context, req *grouppb.UpdateMemberRoleRequest) (*grouppb.UpdateMemberRoleResponse, error)
	// ApplyJoinGroup 申请加入群聊。
	ApplyJoinGroup(ctx context.Context, req *grouppb.ApplyJoinGroupRequest) (*grouppb.ApplyJoinGroupResponse, error)
	// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
	CancelJoinGroupApplication(ctx context.Context, req *grouppb.CancelJoinGroupApplicationRequest) (*grouppb.CancelJoinGroupApplicationResponse, error)
	// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
	GetMyJoinGroupApplication(ctx context.Context, req *grouppb.GetMyJoinGroupApplicationRequest) (*grouppb.GetMyJoinGroupApplicationResponse, error)
	// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
	ListMyJoinGroupApplications(ctx context.Context, req *grouppb.ListMyJoinGroupApplicationsRequest) (*grouppb.ListMyJoinGroupApplicationsResponse, error)
	// ReviewJoinGroup 审批入群申请。
	ReviewJoinGroup(ctx context.Context, req *grouppb.ReviewJoinGroupRequest) (*grouppb.ReviewJoinGroupResponse, error)
	// ListJoinRequests 获取待审批入群申请列表。
	ListJoinRequests(ctx context.Context, req *grouppb.ListJoinRequestsRequest) (*grouppb.ListJoinRequestsResponse, error)
	// ListReviewedJoinRequests 获取群已审批申请列表。
	ListReviewedJoinRequests(ctx context.Context, req *grouppb.ListReviewedJoinRequestsRequest) (*grouppb.ListReviewedJoinRequestsResponse, error)
	// AddMember 添加群成员。
	AddMember(ctx context.Context, req *grouppb.AddMemberRequest) (*grouppb.AddMemberResponse, error)
	// LeaveGroup 当前登录用户主动退出群聊。
	LeaveGroup(ctx context.Context, req *grouppb.LeaveGroupRequest) (*grouppb.LeaveGroupResponse, error)
	// RemoveMember 移除群成员。
	RemoveMember(ctx context.Context, req *grouppb.RemoveMemberRequest) (*grouppb.RemoveMemberResponse, error)
	// GetMemberList 获取群成员列表。
	GetMemberList(ctx context.Context, req *grouppb.GetMemberListRequest) (*grouppb.GetMemberListResponse, error)
	// SearchGroupMembers 搜索群成员。
	SearchGroupMembers(ctx context.Context, req *grouppb.SearchGroupMembersRequest) (*grouppb.SearchGroupMembersResponse, error)
	// UpdateMyGroupNickname 更新当前用户自己的群名片。
	UpdateMyGroupNickname(ctx context.Context, req *grouppb.UpdateMyGroupNicknameRequest) (*grouppb.UpdateMyGroupNicknameResponse, error)
	// UpdateGroupMemberNickname 管理员或群主修改指定成员群名片。
	UpdateGroupMemberNickname(ctx context.Context, req *grouppb.UpdateGroupMemberNicknameRequest) (*grouppb.UpdateGroupMemberNicknameResponse, error)
	// MuteGroupMember 设置或取消指定成员单人禁言。
	MuteGroupMember(ctx context.Context, req *grouppb.MuteGroupMemberRequest) (*grouppb.MuteGroupMemberResponse, error)
	// UpdateGroupMuteSetting 更新全员禁言开关。
	UpdateGroupMuteSetting(ctx context.Context, req *grouppb.UpdateGroupMuteSettingRequest) (*grouppb.UpdateGroupMuteSettingResponse, error)
	// GetGroupList 获取当前用户群列表。
	GetGroupList(ctx context.Context, req *grouppb.GetGroupListRequest) (*grouppb.GetGroupListResponse, error)
	// SearchGroups 按群名或完整群号搜索正常群。
	SearchGroups(ctx context.Context, req *grouppb.SearchGroupsRequest) (*grouppb.SearchGroupsResponse, error)
	// GetGroupMemberIds 获取群成员 UUID 列表。
	GetGroupMemberIds(ctx context.Context, req *grouppb.GetGroupMemberIdsRequest) (*grouppb.GetGroupMemberIdsResponse, error)
	// CheckGroupMember 检查群成员关系与角色。
	CheckGroupMember(ctx context.Context, req *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error)
	// CheckGroupSendPermission 检查指定用户是否允许发送群消息。
	CheckGroupSendPermission(ctx context.Context, req *grouppb.CheckGroupSendPermissionRequest) (*grouppb.CheckGroupSendPermissionResponse, error)
	// GetJoinRequestPendingCount 获取群待审批入群申请数量。
	GetJoinRequestPendingCount(ctx context.Context, req *grouppb.GetJoinRequestPendingCountRequest) (*grouppb.GetJoinRequestPendingCountResponse, error)
}

// UserServiceClient 用户服务 gRPC 客户端接口
// 职责：封装对用户服务的 gRPC 调用
type UserServiceClient interface {
	// ==================== 认证服务 ====================
	// Login 用户登录
	Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error)
	// LoginByCode 验证码登录
	LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error)
	// SendVerifyCode 发送验证码
	SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error)
	// VerifyCode 校验验证码
	VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error)
	// Register 用户注册
	Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error)
	// RefreshToken 刷新Token
	RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error)
	// Logout 用户登出
	Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error)
	// ResetPassword 重置密码
	ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error)
	// FindAccountByEmail 按邮箱查找账号（内部调用）
	FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error)
	// ==================== 用户信息服务 ====================
	// GetProfile 获取个人信息
	GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error)
	// GetOtherProfile 获取他人信息
	GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error)
	// SearchUser 搜索用户
	SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error)
	// UpdateProfile 更新基本信息
	UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error)
	// UploadAvatar 上传头像
	UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error)
	// ChangePassword 修改密码
	ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error)
	// ChangeEmail 绑定/换绑邮箱
	ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error)
	// ChangeTelephone 绑定/换绑手机
	ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error)
	// GetQRCode 获取用户二维码
	GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error)
	// ParseQRCode 解析二维码
	ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error)
	// DeleteAccount 注销账号
	DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error)
	// BatchGetProfile 批量获取用户信息
	BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error)
	// ==================== 好友服务 ====================
	// SendFriendApply 发送好友申请
	SendFriendApply(ctx context.Context, req *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error)
	// GetFriendApplyList 获取好友申请列表
	GetFriendApplyList(ctx context.Context, req *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error)
	// GetSentApplyList 获取发出的申请列表
	GetSentApplyList(ctx context.Context, req *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error)
	// HandleFriendApply 处理好友申请
	HandleFriendApply(ctx context.Context, req *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error)
	// GetUnreadApplyCount 获取未读申请数量
	GetUnreadApplyCount(ctx context.Context, req *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error)
	// MarkApplyAsRead 标记申请已读
	MarkApplyAsRead(ctx context.Context, req *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error)
	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, req *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error)
	// SyncFriendList 好友增量同步
	SyncFriendList(ctx context.Context, req *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error)
	// DeleteFriend 删除好友
	DeleteFriend(ctx context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error)
	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, req *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error)
	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, req *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error)
	// GetTagList 获取标签列表
	GetTagList(ctx context.Context, req *relationpb.GetTagListRequest) (*relationpb.GetTagListResponse, error)
	// CheckIsFriend 判断是否好友
	CheckIsFriend(ctx context.Context, req *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error)
	// BatchCheckIsFriend 批量判断是否好友
	BatchCheckIsFriend(ctx context.Context, req *relationpb.BatchCheckIsFriendRequest) (*relationpb.BatchCheckIsFriendResponse, error)
	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, req *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error)
	// ==================== 黑名单服务 ====================
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, req *relationpb.AddBlacklistRequest) (*relationpb.AddBlacklistResponse, error)
	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, req *relationpb.RemoveBlacklistRequest) (*relationpb.RemoveBlacklistResponse, error)
	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, req *relationpb.GetBlacklistListRequest) (*relationpb.GetBlacklistListResponse, error)
	// CheckIsBlacklist 判断是否拉黑
	CheckIsBlacklist(ctx context.Context, req *relationpb.CheckIsBlacklistRequest) (*relationpb.CheckIsBlacklistResponse, error)
	// ==================== 设备会话服务 ====================
	// GetDeviceList 获取设备列表
	GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error)
	// KickDevice 踢出设备
	KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error)
	// GetOnlineStatus 获取用户在线状态
	GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error)
	// BatchGetOnlineStatus 批量获取在线状态
	BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error)
}
