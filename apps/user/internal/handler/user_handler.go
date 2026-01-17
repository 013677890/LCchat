package handler

import (
	"ChatServer/apps/user/internal/dto"
	"ChatServer/apps/user/internal/service"
	pb "ChatServer/apps/user/pb"
	"context"
)

// UserServiceHandler gRPC服务Handler层
// 职责：
//   - 解析gRPC请求参数，转换为DTO
//   - 调用Service层执行业务逻辑
//   - 将DTO结果转换为gRPC Response
//   - 不包含任何业务逻辑（业务逻辑在Service层）
type UserServiceHandler struct {
	pb.UnimplementedUserServiceServer

	// Service层
	authService   service.AuthService
	userService   service.UserQueryService
	friendService service.FriendService
	deviceService service.DeviceService
}

// NewUserServiceHandler 创建Handler实例
func NewUserServiceHandler(
	authService service.AuthService,
	userService service.UserQueryService,
	friendService service.FriendService,
	deviceService service.DeviceService,
) *UserServiceHandler {
	return &UserServiceHandler{
		authService:   authService,
		userService:   userService,
		friendService: friendService,
		deviceService: deviceService,
	}
}

// ==================== 认证相关接口 ====================

// Login 用户登录
// 遵循gRPC标准错误处理：
//   - 成功时返回(response, nil)
//   - 失败时返回(nil, status.Error(...))
func (h *UserServiceHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 1. 转换gRPC请求为DTO
	loginReq := &dto.LoginRequest{
		Telephone:  req.Telephone,
		Password:   req.Password,
		DeviceInfo: req.DeviceInfo,
	}

	// 2. 调用Service层执行登录业务流程
	loginResp, err := h.authService.Login(ctx, loginReq)
	if err != nil {
		return nil, err
	}

	// 3. 转换DTO为gRPC响应
	return &pb.LoginResponse{
		UserInfo: dto.ConvertToProtoUserInfo(loginResp.UserInfo),
	}, nil
}

// Register 用户注册
func (h *UserServiceHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	registerReq := &dto.RegisterRequest{
		Telephone:  req.Telephone,
		Password:   req.Password,
		VerifyCode: req.VerifyCode,
		Nickname:   req.Nickname,
		DeviceInfo: req.DeviceInfo,
	}

	resp, err := h.authService.Register(ctx, registerReq)
	if err != nil {
		return nil, err
	}

	return &pb.RegisterResponse{
		UserUuid:  resp.UserUUID,
		Telephone: resp.Telephone,
		Nickname:  resp.Nickname,
	}, nil
}

// SendSmsCode 发送短信验证码
func (h *UserServiceHandler) SendSmsCode(ctx context.Context, req *pb.SendSmsCodeRequest) (*pb.SendSmsCodeResponse, error) {
	err := h.authService.SendSmsCode(ctx, req.Telephone, req.CodeType)
	if err != nil {
		return nil, err
	}
	return &pb.SendSmsCodeResponse{}, nil
}

// ValidateSmsCode 验证短信验证码
func (h *UserServiceHandler) ValidateSmsCode(ctx context.Context, req *pb.ValidateSmsCodeRequest) (*pb.ValidateSmsCodeResponse, error) {
	isValid, err := h.authService.ValidateSmsCode(ctx, req.Telephone, req.VerifyCode, req.CodeType)
	if err != nil {
		return nil, err
	}
	return &pb.ValidateSmsCodeResponse{IsValid: isValid}, nil
}

// RefreshToken 刷新Token
func (h *UserServiceHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	refreshReq := &dto.RefreshTokenRequest{
		UserUUID: req.UserUuid,
		DeviceID: req.DeviceId,
	}

	resp, err := h.authService.RefreshToken(ctx, refreshReq)
	if err != nil {
		return nil, err
	}

	return &pb.RefreshTokenResponse{
		UserInfo: dto.ConvertToProtoUserInfo(resp.UserInfo),
	}, nil
}

// Logout 用户登出
func (h *UserServiceHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	logoutReq := &dto.LogoutRequest{
		UserUUID: req.UserUuid,
		DeviceID: req.DeviceId,
	}

	err := h.authService.Logout(ctx, logoutReq)
	if err != nil {
		return nil, err
	}

	return &pb.LogoutResponse{}, nil
}

// ==================== 用户信息管理接口 ====================

// GetUserInfo 获取用户信息
func (h *UserServiceHandler) GetUserInfo(ctx context.Context, req *pb.GetUserInfoRequest) (*pb.GetUserInfoResponse, error) {
	getUserReq := &dto.GetUserInfoRequest{
		UserUUID:  req.UserUuid,
		Telephone: req.Telephone,
	}

	resp, err := h.userService.GetUserInfo(ctx, getUserReq)
	if err != nil {
		return nil, err
	}

	return &pb.GetUserInfoResponse{
		UserInfo: dto.ConvertToProtoUserInfo(resp.UserInfo),
	}, nil
}

// UpdateUserInfo 更新用户信息
func (h *UserServiceHandler) UpdateUserInfo(ctx context.Context, req *pb.UpdateUserInfoRequest) (*pb.UpdateUserInfoResponse, error) {
	updateReq := &dto.UpdateUserInfoRequest{
		UserUUID:  req.UserUuid,
		Nickname:  req.Nickname,
		Signature: req.Signature,
		Birthday:  req.Birthday,
		Gender:    int(req.Gender),
	}

	resp, err := h.userService.UpdateUserInfo(ctx, updateReq)
	if err != nil {
		return nil, err
	}

	return &pb.UpdateUserInfoResponse{
		UserInfo: dto.ConvertToProtoUserInfo(resp.UserInfo),
	}, nil
}

// UpdateAvatar 更新用户头像
func (h *UserServiceHandler) UpdateAvatar(ctx context.Context, req *pb.UpdateAvatarRequest) (*pb.UpdateAvatarResponse, error) {
	updateReq := &dto.UpdateAvatarRequest{
		UserUUID:  req.UserUuid,
		AvatarURL: req.AvatarUrl,
	}

	resp, err := h.userService.UpdateAvatar(ctx, updateReq)
	if err != nil {
		return nil, err
	}

	return &pb.UpdateAvatarResponse{
		AvatarUrl: resp.AvatarURL,
	}, nil
}

// ChangePassword 修改密码
func (h *UserServiceHandler) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	changeReq := &dto.ChangePasswordRequest{
		UserUUID:    req.UserUuid,
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	}

	err := h.userService.ChangePassword(ctx, changeReq)
	if err != nil {
		return nil, err
	}

	return &pb.ChangePasswordResponse{}, nil
}

// BindEmail 绑定邮箱
func (h *UserServiceHandler) BindEmail(ctx context.Context, req *pb.BindEmailRequest) (*pb.BindEmailResponse, error) {
	bindReq := &dto.BindEmailRequest{
		UserUUID:   req.UserUuid,
		Email:      req.Email,
		VerifyCode: req.VerifyCode,
	}

	err := h.userService.BindEmail(ctx, bindReq)
	if err != nil {
		return nil, err
	}

	return &pb.BindEmailResponse{}, nil
}

// ==================== 好友关系管理接口 ====================

// SearchUser 搜索用户
func (h *UserServiceHandler) SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
	searchReq := &dto.SearchUserRequest{
		Keyword:     req.Keyword,
		CurrentUUID: req.CurrentUuid,
		Pagination:  req.Pagination,
	}

	resp, err := h.friendService.SearchUser(ctx, searchReq)
	if err != nil {
		return nil, err
	}

	// 转换用户列表
	users := make([]*pb.UserInfo, 0, len(resp.Users))
	for _, user := range resp.Users {
		users = append(users, dto.ConvertToProtoUserInfo(user))
	}

	return &pb.SearchUserResponse{
		Users:      users,
		Pagination: resp.Pagination,
	}, nil
}

// SendFriendRequest 发送好友申请
func (h *UserServiceHandler) SendFriendRequest(ctx context.Context, req *pb.SendFriendRequestRequest) (*pb.SendFriendRequestResponse, error) {
	sendReq := &dto.SendFriendRequestRequest{
		ApplicantUUID: req.ApplicantUuid,
		TargetUUID:    req.TargetUuid,
		Reason:        req.Reason,
	}

	resp, err := h.friendService.SendFriendRequest(ctx, sendReq)
	if err != nil {
		return nil, err
	}

	return &pb.SendFriendRequestResponse{
		RequestId: resp.RequestID,
	}, nil
}

// HandleFriendRequest 处理好友申请
func (h *UserServiceHandler) HandleFriendRequest(ctx context.Context, req *pb.HandleFriendRequestRequest) (*pb.HandleFriendRequestResponse, error) {
	handleReq := &dto.HandleFriendRequestRequest{
		RequestID:   req.RequestId,
		HandlerUUID: req.HandlerUuid,
		Action:      req.Action,
		Remark:      req.Remark,
	}

	err := h.friendService.HandleFriendRequest(ctx, handleReq)
	if err != nil {
		return nil, err
	}

	return &pb.HandleFriendRequestResponse{}, nil
}

// GetFriendRequests 获取好友申请列表
func (h *UserServiceHandler) GetFriendRequests(ctx context.Context, req *pb.GetFriendRequestsRequest) (*pb.GetFriendRequestsResponse, error) {
	getReq := &dto.GetFriendRequestsRequest{
		UserUUID:   req.UserUuid,
		Pagination: req.Pagination,
	}

	resp, err := h.friendService.GetFriendRequests(ctx, getReq)
	if err != nil {
		return nil, err
	}

	// 转换申请列表
	requests := make([]*pb.FriendRequestInfo, 0, len(resp.Requests))
	for _, r := range resp.Requests {
		requests = append(requests, &pb.FriendRequestInfo{
			Id:                r.ID,
			ApplicantUuid:     r.ApplicantUUID,
			ApplicantNickname: r.ApplicantNickname,
			ApplicantAvatar:   r.ApplicantAvatar,
			Reason:            r.Reason,
			Status:            r.Status,
			IsRead:            r.IsRead,
			CreatedAt:         r.CreatedAt.Unix() * 1000,
		})
	}

	return &pb.GetFriendRequestsResponse{
		Requests:   requests,
		Pagination: resp.Pagination,
	}, nil
}

// GetFriendList 获取好友列表
func (h *UserServiceHandler) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	getReq := &dto.GetFriendListRequest{
		UserUUID:   req.UserUuid,
		Pagination: req.Pagination,
	}

	resp, err := h.friendService.GetFriendList(ctx, getReq)
	if err != nil {
		return nil, err
	}

	// 转换好友列表
	friends := make([]*pb.FriendInfo, 0, len(resp.Friends))
	for _, f := range resp.Friends {
		friends = append(friends, &pb.FriendInfo{
			Uuid:      f.UUID,
			Nickname:  f.Nickname,
			Avatar:    f.Avatar,
			Remark:    f.Remark,
			Gender:    f.Gender,
			Signature: f.Signature,
			CreatedAt: f.CreatedAt.Unix() * 1000,
			Telephone: f.Telephone,
		})
	}

	return &pb.GetFriendListResponse{
		Friends:    friends,
		Pagination: resp.Pagination,
	}, nil
}

// DeleteFriend 删除好友
func (h *UserServiceHandler) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) (*pb.DeleteFriendResponse, error) {
	deleteReq := &dto.DeleteFriendRequest{
		UserUUID:   req.UserUuid,
		TargetUUID: req.TargetUuid,
	}

	err := h.friendService.DeleteFriend(ctx, deleteReq)
	if err != nil {
		return nil, err
	}

	return &pb.DeleteFriendResponse{}, nil
}

// SetFriendRemark 设置好友备注
func (h *UserServiceHandler) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) (*pb.SetFriendRemarkResponse, error) {
	setReq := &dto.SetFriendRemarkRequest{
		UserUUID:   req.UserUuid,
		TargetUUID: req.TargetUuid,
		Remark:     req.Remark,
	}

	err := h.friendService.SetFriendRemark(ctx, setReq)
	if err != nil {
		return nil, err
	}

	return &pb.SetFriendRemarkResponse{}, nil
}

// BlockUser 拉黑用户
func (h *UserServiceHandler) BlockUser(ctx context.Context, req *pb.BlockUserRequest) (*pb.BlockUserResponse, error) {
	blockReq := &dto.BlockUserRequest{
		UserUUID:   req.UserUuid,
		TargetUUID: req.TargetUuid,
	}

	err := h.friendService.BlockUser(ctx, blockReq)
	if err != nil {
		return nil, err
	}

	return &pb.BlockUserResponse{}, nil
}

// UnblockUser 解除拉黑
func (h *UserServiceHandler) UnblockUser(ctx context.Context, req *pb.UnblockUserRequest) (*pb.UnblockUserResponse, error) {
	unblockReq := &dto.UnblockUserRequest{
		UserUUID:   req.UserUuid,
		TargetUUID: req.TargetUuid,
	}

	err := h.friendService.UnblockUser(ctx, unblockReq)
	if err != nil {
		return nil, err
	}

	return &pb.UnblockUserResponse{}, nil
}

// GetBlacklist 获取黑名单
func (h *UserServiceHandler) GetBlacklist(ctx context.Context, req *pb.GetBlacklistRequest) (*pb.GetBlacklistResponse, error) {
	getReq := &dto.GetBlacklistRequest{
		UserUUID:   req.UserUuid,
		Pagination: req.Pagination,
	}

	resp, err := h.friendService.GetBlacklist(ctx, getReq)
	if err != nil {
		return nil, err
	}

	// 转换黑名单列表
	blacklist := make([]*pb.BlacklistUser, 0, len(resp.Blacklist))
	for _, b := range resp.Blacklist {
		blacklist = append(blacklist, &pb.BlacklistUser{
			Uuid:      b.UUID,
			Nickname:  b.Nickname,
			Avatar:    b.Avatar,
			BlockedAt: b.BlockedAt.Unix() * 1000,
		})
	}

	return &pb.GetBlacklistResponse{
		Blacklist:  blacklist,
		Pagination: resp.Pagination,
	}, nil
}

// ==================== 设备会话管理接口 ====================

// CreateDeviceSession 创建设备会话
func (h *UserServiceHandler) CreateDeviceSession(ctx context.Context, req *pb.CreateDeviceSessionRequest) (*pb.CreateDeviceSessionResponse, error) {
	createReq := &dto.CreateDeviceSessionRequest{
		UserUUID:   req.UserUuid,
		DeviceInfo: req.DeviceInfo,
		IP:         req.Ip,
		UserAgent:  req.UserAgent,
	}

	err := h.deviceService.CreateDeviceSession(ctx, createReq)
	if err != nil {
		return nil, err
	}

	return &pb.CreateDeviceSessionResponse{}, nil
}

// GetDeviceSessions 获取设备列表
func (h *UserServiceHandler) GetDeviceSessions(ctx context.Context, req *pb.GetDeviceSessionsRequest) (*pb.GetDeviceSessionsResponse, error) {
	getReq := &dto.GetDeviceSessionsRequest{
		UserUUID: req.UserUuid,
	}

	resp, err := h.deviceService.GetDeviceSessions(ctx, getReq)
	if err != nil {
		return nil, err
	}

	// 转换设备列表
	devices := make([]*pb.DeviceSessionInfo, 0, len(resp.Devices))
	for _, d := range resp.Devices {
		devices = append(devices, &pb.DeviceSessionInfo{
			DeviceId:   d.DeviceID,
			DeviceName: d.DeviceName,
			Platform:   d.Platform,
			OsVersion:  d.OSVersion,
			AppVersion: d.AppVersion,
			Ip:         d.IP,
			Status:     d.Status,
			LastSeenAt: d.LastSeenAt.Unix() * 1000,
			CreatedAt:  d.CreatedAt.Unix() * 1000,
		})
	}

	return &pb.GetDeviceSessionsResponse{
		Devices: devices,
	}, nil
}

// UpdateDeviceOnlineState 更新设备在线状态
func (h *UserServiceHandler) UpdateDeviceOnlineState(ctx context.Context, req *pb.UpdateDeviceOnlineStateRequest) (*pb.UpdateDeviceOnlineStateResponse, error) {
	updateReq := &dto.UpdateDeviceOnlineStateRequest{
		UserUUID: req.UserUuid,
		DeviceID: req.DeviceId,
		Status:   req.Status,
	}

	err := h.deviceService.UpdateDeviceOnlineState(ctx, updateReq)
	if err != nil {
		return nil, err
	}

	return &pb.UpdateDeviceOnlineStateResponse{}, nil
}

// KickDevice 踢出设备
func (h *UserServiceHandler) KickDevice(ctx context.Context, req *pb.KickDeviceRequest) (*pb.KickDeviceResponse, error) {
	kickReq := &dto.KickDeviceRequest{
		OperatorUUID:   req.OperatorUuid,
		TargetDeviceID: req.TargetDeviceId,
	}

	err := h.deviceService.KickDevice(ctx, kickReq)
	if err != nil {
		return nil, err
	}

	return &pb.KickDeviceResponse{}, nil
}

// BatchGetUsers 批量获取用户信息
func (h *UserServiceHandler) BatchGetUsers(ctx context.Context, req *pb.BatchGetUsersRequest) (*pb.BatchGetUsersResponse, error) {
	batchReq := &dto.BatchGetUsersRequest{
		UserUUIDs: req.UserUuids,
	}

	resp, err := h.userService.BatchGetUsers(ctx, batchReq)
	if err != nil {
		return nil, err
	}

	// 转换用户信息映射
	users := make(map[string]*pb.UserInfo)
	for uuid, user := range resp.Users {
		users[uuid] = dto.ConvertToProtoUserInfo(user)
	}

	return &pb.BatchGetUsersResponse{
		Users: users,
	}, nil
}

// GetUsersOnlineState 批量获取用户在线状态
func (h *UserServiceHandler) GetUsersOnlineState(ctx context.Context, req *pb.GetUsersOnlineStateRequest) (*pb.GetUsersOnlineStateResponse, error) {
	getReq := &dto.GetUsersOnlineStateRequest{
		UserUUIDs: req.UserUuids,
	}

	resp, err := h.deviceService.GetUsersOnlineState(ctx, getReq)
	if err != nil {
		return nil, err
	}

	// 转换在线状态映射
	onlineStates := make(map[string]*pb.OnlineState)
	for uuid, state := range resp.OnlineStates {
		onlineStates[uuid] = &pb.OnlineState{
			IsOnline:     state.IsOnline,
			DeviceIds:    state.DeviceIDs,
			LastActiveAt: state.LastActiveAt.Unix() * 1000,
		}
	}

	return &pb.GetUsersOnlineStateResponse{
		OnlineStates: onlineStates,
	}, nil
}
