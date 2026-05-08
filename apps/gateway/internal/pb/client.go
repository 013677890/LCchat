package pb

import (
	"context"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
)

// userServiceClientImpl 用户服务 gRPC 客户端实现
type userServiceClientImpl struct {
	authClient      authpb.AuthServiceClient
	accountClient   authpb.AccountServiceClient
	internalAuth    authpb.InternalAuthServiceClient
	userClient      userpb.UserServiceClient
	friendClient    relationpb.FriendServiceClient
	blacklistClient relationpb.BlacklistServiceClient
	deviceClient    authpb.DeviceServiceClient
}

// NewUserServiceClient 创建用户服务 gRPC 客户端实例
// authConn: 认证服务gRPC连接
// userConn: 用户服务gRPC连接
// friendConn: 好友服务gRPC连接
// blacklistConn: 黑名单服务gRPC连接
// deviceConn: 设备服务gRPC连接
func NewUserServiceClient(
	authConn, userConn, friendConn, blacklistConn, deviceConn *grpc.ClientConn,
) UserServiceClient {
	return &userServiceClientImpl{
		authClient:      authpb.NewAuthServiceClient(authConn),
		accountClient:   authpb.NewAccountServiceClient(authConn),
		internalAuth:    authpb.NewInternalAuthServiceClient(authConn),
		userClient:      userpb.NewUserServiceClient(userConn),
		friendClient:    relationpb.NewFriendServiceClient(friendConn),
		blacklistClient: relationpb.NewBlacklistServiceClient(blacklistConn),
		deviceClient:    authpb.NewDeviceServiceClient(deviceConn),
	}
}

// ==================== 认证服务方法实现 ====================

// Login 用户登录
func (c *userServiceClientImpl) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	return executeGRPCCall("auth.AuthService", "Login", func() (*authpb.LoginResponse, error) {
		return c.authClient.Login(ctx, req)
	})
}

// LoginByCode 验证码登录
func (c *userServiceClientImpl) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	return executeGRPCCall("auth.AuthService", "LoginByCode", func() (*authpb.LoginByCodeResponse, error) {
		return c.authClient.LoginByCode(ctx, req)
	})
}

// SendVerifyCode 发送验证码
func (c *userServiceClientImpl) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	return executeGRPCCall("auth.AuthService", "SendVerifyCode", func() (*authpb.SendVerifyCodeResponse, error) {
		return c.authClient.SendVerifyCode(ctx, req)
	})
}

// VerifyCode 校验验证码
func (c *userServiceClientImpl) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	return executeGRPCCall("auth.AuthService", "VerifyCode", func() (*authpb.VerifyCodeResponse, error) {
		return c.authClient.VerifyCode(ctx, req)
	})
}

// Register 用户注册
func (c *userServiceClientImpl) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	return executeGRPCCall("auth.AuthService", "Register", func() (*authpb.RegisterResponse, error) {
		return c.authClient.Register(ctx, req)
	})
}

// RefreshToken 刷新Token
func (c *userServiceClientImpl) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	return executeGRPCCall("auth.AuthService", "RefreshToken", func() (*authpb.RefreshTokenResponse, error) {
		return c.authClient.RefreshToken(ctx, req)
	})
}

// Logout 用户登出
func (c *userServiceClientImpl) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	return executeGRPCCall("auth.AuthService", "Logout", func() (*authpb.LogoutResponse, error) {
		return c.authClient.Logout(ctx, req)
	})
}

// ResetPassword 重置密码
func (c *userServiceClientImpl) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
	return executeGRPCCall("auth.AuthService", "ResetPassword", func() (*authpb.ResetPasswordResponse, error) {
		return c.authClient.ResetPassword(ctx, req)
	})
}

// FindAccountByEmail 按邮箱查找账号（内部调用）。
func (c *userServiceClientImpl) FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error) {
	// auth 连接在建连阶段已经配置了“仅对内部方法注入 x-internal-caller”，
	// 这里不再单独手写 metadata，避免调用点与连接装配策略分裂。
	return executeGRPCCall("auth.InternalAuthService", "FindAccountByEmail", func() (*authpb.FindAccountByEmailResponse, error) {
		return c.internalAuth.FindAccountByEmail(ctx, req)
	})
}

// ==================== 用户信息服务方法实现 ====================

// GetProfile 获取个人信息
func (c *userServiceClientImpl) GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
	return executeGRPCCall("user.UserService", "GetProfile", func() (*userpb.GetProfileResponse, error) {
		return c.userClient.GetProfile(ctx, req)
	})
}

// GetOtherProfile 获取他人信息
func (c *userServiceClientImpl) GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
	return executeGRPCCall("user.UserService", "GetOtherProfile", func() (*userpb.GetOtherProfileResponse, error) {
		return c.userClient.GetOtherProfile(ctx, req)
	})
}

// UpdateProfile 更新基本信息
func (c *userServiceClientImpl) UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error) {
	return executeGRPCCall("user.UserService", "UpdateProfile", func() (*userpb.UpdateProfileResponse, error) {
		return c.userClient.UpdateProfile(ctx, req)
	})
}

// UploadAvatar 上传头像
func (c *userServiceClientImpl) UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error) {
	return executeGRPCCall("user.UserService", "UploadAvatar", func() (*userpb.UploadAvatarResponse, error) {
		return c.userClient.UploadAvatar(ctx, req)
	})
}

// ChangePassword 修改密码
func (c *userServiceClientImpl) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {
	return executeGRPCCall("auth.AccountService", "ChangePassword", func() (*authpb.ChangePasswordResponse, error) {
		return c.accountClient.ChangePassword(ctx, req)
	})
}

// ChangeEmail 绑定/换绑邮箱
func (c *userServiceClientImpl) ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error) {
	return executeGRPCCall("auth.AccountService", "ChangeEmail", func() (*authpb.ChangeEmailResponse, error) {
		return c.accountClient.ChangeEmail(ctx, req)
	})
}

// ChangeTelephone 绑定/换绑手机
func (c *userServiceClientImpl) ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error) {
	return executeGRPCCall("auth.AccountService", "ChangeTelephone", func() (*authpb.ChangeTelephoneResponse, error) {
		return c.accountClient.ChangeTelephone(ctx, req)
	})
}

// GetQRCode 获取用户二维码
func (c *userServiceClientImpl) GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
	return executeGRPCCall("user.UserService", "GetQRCode", func() (*userpb.GetQRCodeResponse, error) {
		return c.userClient.GetQRCode(ctx, req)
	})
}

// ParseQRCode 解析二维码
func (c *userServiceClientImpl) ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error) {
	return executeGRPCCall("user.UserService", "ParseQRCode", func() (*userpb.ParseQRCodeResponse, error) {
		return c.userClient.ParseQRCode(ctx, req)
	})
}

// DeleteAccount 注销账号
func (c *userServiceClientImpl) DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error) {
	return executeGRPCCall("auth.AccountService", "DeleteAccount", func() (*authpb.DeleteAccountResponse, error) {
		return c.accountClient.DeleteAccount(ctx, req)
	})
}

// BatchGetProfile 批量获取用户信息
func (c *userServiceClientImpl) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	return executeGRPCCall("user.UserService", "BatchGetProfile", func() (*userpb.BatchGetProfileResponse, error) {
		return c.userClient.BatchGetProfile(ctx, req)
	})
}

// ==================== 好友服务方法实现 ====================

// SearchUser 搜索用户
func (c *userServiceClientImpl) SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
	return executeGRPCCall("user.UserService", "SearchUser", func() (*userpb.SearchUserResponse, error) {
		return c.userClient.SearchUser(ctx, req)
	})
}

// SendFriendApply 发送好友申请
func (c *userServiceClientImpl) SendFriendApply(ctx context.Context, req *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error) {
	return executeGRPCCall("relation.FriendService", "SendFriendApply", func() (*relationpb.SendFriendApplyResponse, error) {
		return c.friendClient.SendFriendApply(ctx, req)
	})
}

// GetFriendApplyList 获取好友申请列表
func (c *userServiceClientImpl) GetFriendApplyList(ctx context.Context, req *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
	return executeGRPCCall("relation.FriendService", "GetFriendApplyList", func() (*relationpb.GetFriendApplyListResponse, error) {
		return c.friendClient.GetFriendApplyList(ctx, req)
	})
}

// GetSentApplyList 获取发出的申请列表
func (c *userServiceClientImpl) GetSentApplyList(ctx context.Context, req *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error) {
	return executeGRPCCall("relation.FriendService", "GetSentApplyList", func() (*relationpb.GetSentApplyListResponse, error) {
		return c.friendClient.GetSentApplyList(ctx, req)
	})
}

// HandleFriendApply 处理好友申请
func (c *userServiceClientImpl) HandleFriendApply(ctx context.Context, req *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error) {
	return executeGRPCCall("relation.FriendService", "HandleFriendApply", func() (*relationpb.HandleFriendApplyResponse, error) {
		return c.friendClient.HandleFriendApply(ctx, req)
	})
}

// GetUnreadApplyCount 获取未读申请数量
func (c *userServiceClientImpl) GetUnreadApplyCount(ctx context.Context, req *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error) {
	return executeGRPCCall("relation.FriendService", "GetUnreadApplyCount", func() (*relationpb.GetUnreadApplyCountResponse, error) {
		return c.friendClient.GetUnreadApplyCount(ctx, req)
	})
}

// MarkApplyAsRead 标记申请已读
func (c *userServiceClientImpl) MarkApplyAsRead(ctx context.Context, req *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error) {
	return executeGRPCCall("relation.FriendService", "MarkApplyAsRead", func() (*relationpb.MarkApplyAsReadResponse, error) {
		return c.friendClient.MarkApplyAsRead(ctx, req)
	})
}

// GetFriendList 获取好友列表
func (c *userServiceClientImpl) GetFriendList(ctx context.Context, req *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error) {
	return executeGRPCCall("relation.FriendService", "GetFriendList", func() (*relationpb.GetFriendListResponse, error) {
		return c.friendClient.GetFriendList(ctx, req)
	})
}

// SyncFriendList 好友增量同步
func (c *userServiceClientImpl) SyncFriendList(ctx context.Context, req *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error) {
	return executeGRPCCall("relation.FriendService", "SyncFriendList", func() (*relationpb.SyncFriendListResponse, error) {
		return c.friendClient.SyncFriendList(ctx, req)
	})
}

// DeleteFriend 删除好友
func (c *userServiceClientImpl) DeleteFriend(ctx context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error) {
	return executeGRPCCall("relation.FriendService", "DeleteFriend", func() (*relationpb.DeleteFriendResponse, error) {
		return c.friendClient.DeleteFriend(ctx, req)
	})
}

// SetFriendRemark 设置好友备注
func (c *userServiceClientImpl) SetFriendRemark(ctx context.Context, req *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error) {
	return executeGRPCCall("relation.FriendService", "SetFriendRemark", func() (*relationpb.SetFriendRemarkResponse, error) {
		return c.friendClient.SetFriendRemark(ctx, req)
	})
}

// SetFriendTag 设置好友标签
func (c *userServiceClientImpl) SetFriendTag(ctx context.Context, req *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error) {
	return executeGRPCCall("relation.FriendService", "SetFriendTag", func() (*relationpb.SetFriendTagResponse, error) {
		return c.friendClient.SetFriendTag(ctx, req)
	})
}

// GetTagList 获取标签列表
func (c *userServiceClientImpl) GetTagList(ctx context.Context, req *relationpb.GetTagListRequest) (*relationpb.GetTagListResponse, error) {
	return executeGRPCCall("relation.FriendService", "GetTagList", func() (*relationpb.GetTagListResponse, error) {
		return c.friendClient.GetTagList(ctx, req)
	})
}

// CheckIsFriend 判断是否好友
func (c *userServiceClientImpl) CheckIsFriend(ctx context.Context, req *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error) {
	return executeGRPCCall("relation.FriendService", "CheckIsFriend", func() (*relationpb.CheckIsFriendResponse, error) {
		return c.friendClient.CheckIsFriend(ctx, req)
	})
}

// BatchCheckIsFriend 批量判断是否好友
func (c *userServiceClientImpl) BatchCheckIsFriend(ctx context.Context, req *relationpb.BatchCheckIsFriendRequest) (*relationpb.BatchCheckIsFriendResponse, error) {
	return executeGRPCCall("relation.FriendService", "BatchCheckIsFriend", func() (*relationpb.BatchCheckIsFriendResponse, error) {
		return c.friendClient.BatchCheckIsFriend(ctx, req)
	})
}

// GetRelationStatus 获取关系状态
func (c *userServiceClientImpl) GetRelationStatus(ctx context.Context, req *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error) {
	return executeGRPCCall("relation.FriendService", "GetRelationStatus", func() (*relationpb.GetRelationStatusResponse, error) {
		return c.friendClient.GetRelationStatus(ctx, req)
	})
}

// ==================== 黑名单服务方法实现 ====================

// AddBlacklist 拉黑用户
func (c *userServiceClientImpl) AddBlacklist(ctx context.Context, req *relationpb.AddBlacklistRequest) (*relationpb.AddBlacklistResponse, error) {
	return executeGRPCCall("relation.BlacklistService", "AddBlacklist", func() (*relationpb.AddBlacklistResponse, error) {
		return c.blacklistClient.AddBlacklist(ctx, req)
	})
}

// RemoveBlacklist 取消拉黑
func (c *userServiceClientImpl) RemoveBlacklist(ctx context.Context, req *relationpb.RemoveBlacklistRequest) (*relationpb.RemoveBlacklistResponse, error) {
	return executeGRPCCall("relation.BlacklistService", "RemoveBlacklist", func() (*relationpb.RemoveBlacklistResponse, error) {
		return c.blacklistClient.RemoveBlacklist(ctx, req)
	})
}

// GetBlacklistList 获取黑名单列表
func (c *userServiceClientImpl) GetBlacklistList(ctx context.Context, req *relationpb.GetBlacklistListRequest) (*relationpb.GetBlacklistListResponse, error) {
	return executeGRPCCall("relation.BlacklistService", "GetBlacklistList", func() (*relationpb.GetBlacklistListResponse, error) {
		return c.blacklistClient.GetBlacklistList(ctx, req)
	})
}

// CheckIsBlacklist 判断是否拉黑
func (c *userServiceClientImpl) CheckIsBlacklist(ctx context.Context, req *relationpb.CheckIsBlacklistRequest) (*relationpb.CheckIsBlacklistResponse, error) {
	return executeGRPCCall("relation.BlacklistService", "CheckIsBlacklist", func() (*relationpb.CheckIsBlacklistResponse, error) {
		return c.blacklistClient.CheckIsBlacklist(ctx, req)
	})
}

// ==================== 设备会话服务方法实现 ====================

// GetDeviceList 获取设备列表
func (c *userServiceClientImpl) GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
	return executeGRPCCall("auth.DeviceService", "GetDeviceList", func() (*authpb.GetDeviceListResponse, error) {
		return c.deviceClient.GetDeviceList(ctx, req)
	})
}

// KickDevice 踢出设备
func (c *userServiceClientImpl) KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
	return executeGRPCCall("auth.DeviceService", "KickDevice", func() (*authpb.KickDeviceResponse, error) {
		return c.deviceClient.KickDevice(ctx, req)
	})
}

// GetOnlineStatus 获取用户在线状态
func (c *userServiceClientImpl) GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
	return executeGRPCCall("auth.DeviceService", "GetOnlineStatus", func() (*authpb.GetOnlineStatusResponse, error) {
		return c.deviceClient.GetOnlineStatus(ctx, req)
	})
}

// BatchGetOnlineStatus 批量获取在线状态
func (c *userServiceClientImpl) BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
	return executeGRPCCall("auth.DeviceService", "BatchGetOnlineStatus", func() (*authpb.BatchGetOnlineStatusResponse, error) {
		return c.deviceClient.BatchGetOnlineStatus(ctx, req)
	})
}

// ==================== 通用工具函数 ====================

const gatewayMaxCallRecvMsgSize = 4 * 1024 * 1024

// newGatewayConnection 把 gateway 出站 gRPC 连接的公共装配统一收口到一个地方。
//
// 统一内容包括：
//   - metadata 透传；
//   - 方法级 timeout；
//   - 出站聚合日志；
//   - 精确 service 级 retry；
//   - 可选的 method-scoped internal-caller；
//   - gateway 特有的熔断保护。
func newGatewayConnection(
	addr string,
	retry *grpcx.ClientRetryConfig,
	internalMethods []string,
	breaker *gobreaker.CircuitBreaker,
) (*grpc.ClientConn, error) {
	var internalCaller *grpcx.InternalCallerClientConfig
	if len(internalMethods) > 0 {
		internalCaller = &grpcx.InternalCallerClientConfig{
			Caller:  "gateway",
			Methods: internalMethods,
		}
	}

	return grpcx.NewClient(grpcx.ClientOptions{
		Address:        addr,
		Timeout:        &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry:          retry,
		InternalCaller: internalCaller,
		Observers: []grpcx.ClientCallObserver{
			middleware.GRPCMetricsObserver(),
		},
		ExtraUnaryInterceptors: []grpc.UnaryClientInterceptor{
			grpcx.CircuitBreakerUnaryClientInterceptor(breaker),
		},
		MaxRecvMsgSize: gatewayMaxCallRecvMsgSize,
	})
}

// CreateCircuitBreaker 创建熔断器实例
// name: 熔断器名称
// 返回: 熔断器实例
func CreateCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,                // 半开状态下最多允许 3 个请求尝试
		Interval:    15 * time.Second, // 清除计数的时间间隔
		Timeout:     45 * time.Second, // 熔断器开启后多久尝试进入半开状态
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 失败率超过 50% 且连续失败次数超过 5 次时触发熔断
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info(context.Background(), "熔断器状态变化",
				logger.String("name", name),
				logger.String("from", from.String()),
				logger.String("to", to.String()),
			)
		},
	})
}

// executeGRPCCall 现在只保留轻量 facade 包装。
// gateway 的 gRPC 指标观测已经下沉到建连阶段的 grpcx observer，
// 避免 facade 方法同时承担“发起调用 + 手动计时上报”两层职责。
func executeGRPCCall[T any](_ string, _ string, fn func() (T, error)) (T, error) {
	return fn()
}

// CreateAuthServiceConnection 创建认证服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateAuthServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// auth 连接同时承载公开接口与内部接口，
	// 因此 internal-caller 只对白名单内部方法注入，避免污染公开调用。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(
		"auth.AuthService",
		"auth.DeviceService",
		"auth.AccountService",
		"auth.InternalAuthService",
	), []string{
		"/auth.InternalAuthService/FindAccountByEmail",
	}, breaker)
}

// CreateUserServiceConnection 创建用户服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateUserServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig("user.UserService"), nil, breaker)
}

// CreateRelationFriendServiceConnection 创建 relation-friend 服务 gRPC 连接。
// addr: relation 服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateRelationFriendServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(
		"relation.FriendService",
		"relation.BlacklistService",
	), nil, breaker)
}
