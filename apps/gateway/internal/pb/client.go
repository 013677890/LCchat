package pb

import (
	"context"
	"fmt"
	"strings"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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
	authBreaker     *gobreaker.CircuitBreaker
	userBreaker     *gobreaker.CircuitBreaker
	relationBreaker *gobreaker.CircuitBreaker
}

// NewUserServiceClient 创建用户服务 gRPC 客户端实例
// authConn: 认证服务gRPC连接
// userConn: 用户服务gRPC连接
// friendConn: 好友服务gRPC连接
// blacklistConn: 黑名单服务gRPC连接
// deviceConn: 设备服务gRPC连接
// authBreaker/userBreaker/relationBreaker: 域级熔断器实例
func NewUserServiceClient(
	authConn, userConn, friendConn, blacklistConn, deviceConn *grpc.ClientConn,
	authBreaker, userBreaker, relationBreaker *gobreaker.CircuitBreaker,
) UserServiceClient {
	return &userServiceClientImpl{
		authClient:      authpb.NewAuthServiceClient(authConn),
		accountClient:   authpb.NewAccountServiceClient(authConn),
		internalAuth:    authpb.NewInternalAuthServiceClient(authConn),
		userClient:      userpb.NewUserServiceClient(userConn),
		friendClient:    relationpb.NewFriendServiceClient(friendConn),
		blacklistClient: relationpb.NewBlacklistServiceClient(blacklistConn),
		deviceClient:    authpb.NewDeviceServiceClient(deviceConn),
		authBreaker:     authBreaker,
		userBreaker:     userBreaker,
		relationBreaker: relationBreaker,
	}
}

// ==================== 认证服务方法实现 ====================

// Login 用户登录
func (c *userServiceClientImpl) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "Login", func() (*authpb.LoginResponse, error) {
		return c.authClient.Login(ctx, req)
	})
}

// LoginByCode 验证码登录
func (c *userServiceClientImpl) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "LoginByCode", func() (*authpb.LoginByCodeResponse, error) {
		return c.authClient.LoginByCode(ctx, req)
	})
}

// SendVerifyCode 发送验证码
func (c *userServiceClientImpl) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "SendVerifyCode", func() (*authpb.SendVerifyCodeResponse, error) {
		return c.authClient.SendVerifyCode(ctx, req)
	})
}

// VerifyCode 校验验证码
func (c *userServiceClientImpl) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "VerifyCode", func() (*authpb.VerifyCodeResponse, error) {
		return c.authClient.VerifyCode(ctx, req)
	})
}

// Register 用户注册
func (c *userServiceClientImpl) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "Register", func() (*authpb.RegisterResponse, error) {
		return c.authClient.Register(ctx, req)
	})
}

// RefreshToken 刷新Token
func (c *userServiceClientImpl) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "RefreshToken", func() (*authpb.RefreshTokenResponse, error) {
		return c.authClient.RefreshToken(ctx, req)
	})
}

// Logout 用户登出
func (c *userServiceClientImpl) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "Logout", func() (*authpb.LogoutResponse, error) {
		return c.authClient.Logout(ctx, req)
	})
}

// ResetPassword 重置密码
func (c *userServiceClientImpl) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) (*authpb.ResetPasswordResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AuthService", "ResetPassword", func() (*authpb.ResetPasswordResponse, error) {
		return c.authClient.ResetPassword(ctx, req)
	})
}

// FindAccountByEmail 按邮箱查找账号（内部调用）。
func (c *userServiceClientImpl) FindAccountByEmail(ctx context.Context, req *authpb.FindAccountByEmailRequest) (*authpb.FindAccountByEmailResponse, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, grpcx.MetadataInternalCaller, "gateway")
	return ExecuteWithBreaker(c.authBreaker, "auth.InternalAuthService", "FindAccountByEmail", func() (*authpb.FindAccountByEmailResponse, error) {
		return c.internalAuth.FindAccountByEmail(ctx, req)
	})
}

// ==================== 用户信息服务方法实现 ====================

// GetProfile 获取个人信息
func (c *userServiceClientImpl) GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "GetProfile", func() (*userpb.GetProfileResponse, error) {
		return c.userClient.GetProfile(ctx, req)
	})
}

// GetOtherProfile 获取他人信息
func (c *userServiceClientImpl) GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "GetOtherProfile", func() (*userpb.GetOtherProfileResponse, error) {
		return c.userClient.GetOtherProfile(ctx, req)
	})
}

// UpdateProfile 更新基本信息
func (c *userServiceClientImpl) UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "UpdateProfile", func() (*userpb.UpdateProfileResponse, error) {
		return c.userClient.UpdateProfile(ctx, req)
	})
}

// UploadAvatar 上传头像
func (c *userServiceClientImpl) UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "UploadAvatar", func() (*userpb.UploadAvatarResponse, error) {
		return c.userClient.UploadAvatar(ctx, req)
	})
}

// ChangePassword 修改密码
func (c *userServiceClientImpl) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AccountService", "ChangePassword", func() (*authpb.ChangePasswordResponse, error) {
		return c.accountClient.ChangePassword(ctx, req)
	})
}

// ChangeEmail 绑定/换绑邮箱
func (c *userServiceClientImpl) ChangeEmail(ctx context.Context, req *authpb.ChangeEmailRequest) (*authpb.ChangeEmailResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AccountService", "ChangeEmail", func() (*authpb.ChangeEmailResponse, error) {
		return c.accountClient.ChangeEmail(ctx, req)
	})
}

// ChangeTelephone 绑定/换绑手机
func (c *userServiceClientImpl) ChangeTelephone(ctx context.Context, req *authpb.ChangeTelephoneRequest) (*authpb.ChangeTelephoneResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AccountService", "ChangeTelephone", func() (*authpb.ChangeTelephoneResponse, error) {
		return c.accountClient.ChangeTelephone(ctx, req)
	})
}

// GetQRCode 获取用户二维码
func (c *userServiceClientImpl) GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "GetQRCode", func() (*userpb.GetQRCodeResponse, error) {
		return c.userClient.GetQRCode(ctx, req)
	})
}

// ParseQRCode 解析二维码
func (c *userServiceClientImpl) ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "ParseQRCode", func() (*userpb.ParseQRCodeResponse, error) {
		return c.userClient.ParseQRCode(ctx, req)
	})
}

// DeleteAccount 注销账号
func (c *userServiceClientImpl) DeleteAccount(ctx context.Context, req *authpb.DeleteAccountRequest) (*authpb.DeleteAccountResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.AccountService", "DeleteAccount", func() (*authpb.DeleteAccountResponse, error) {
		return c.accountClient.DeleteAccount(ctx, req)
	})
}

// BatchGetProfile 批量获取用户信息
func (c *userServiceClientImpl) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "BatchGetProfile", func() (*userpb.BatchGetProfileResponse, error) {
		return c.userClient.BatchGetProfile(ctx, req)
	})
}

// ==================== 好友服务方法实现 ====================

// SearchUser 搜索用户
func (c *userServiceClientImpl) SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
	return ExecuteWithBreaker(c.userBreaker, "user.UserService", "SearchUser", func() (*userpb.SearchUserResponse, error) {
		return c.userClient.SearchUser(ctx, req)
	})
}

// SendFriendApply 发送好友申请
func (c *userServiceClientImpl) SendFriendApply(ctx context.Context, req *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "SendFriendApply", func() (*relationpb.SendFriendApplyResponse, error) {
		return c.friendClient.SendFriendApply(ctx, req)
	})
}

// GetFriendApplyList 获取好友申请列表
func (c *userServiceClientImpl) GetFriendApplyList(ctx context.Context, req *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "GetFriendApplyList", func() (*relationpb.GetFriendApplyListResponse, error) {
		return c.friendClient.GetFriendApplyList(ctx, req)
	})
}

// GetSentApplyList 获取发出的申请列表
func (c *userServiceClientImpl) GetSentApplyList(ctx context.Context, req *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "GetSentApplyList", func() (*relationpb.GetSentApplyListResponse, error) {
		return c.friendClient.GetSentApplyList(ctx, req)
	})
}

// HandleFriendApply 处理好友申请
func (c *userServiceClientImpl) HandleFriendApply(ctx context.Context, req *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "HandleFriendApply", func() (*relationpb.HandleFriendApplyResponse, error) {
		return c.friendClient.HandleFriendApply(ctx, req)
	})
}

// GetUnreadApplyCount 获取未读申请数量
func (c *userServiceClientImpl) GetUnreadApplyCount(ctx context.Context, req *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "GetUnreadApplyCount", func() (*relationpb.GetUnreadApplyCountResponse, error) {
		return c.friendClient.GetUnreadApplyCount(ctx, req)
	})
}

// MarkApplyAsRead 标记申请已读
func (c *userServiceClientImpl) MarkApplyAsRead(ctx context.Context, req *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "MarkApplyAsRead", func() (*relationpb.MarkApplyAsReadResponse, error) {
		return c.friendClient.MarkApplyAsRead(ctx, req)
	})
}

// GetFriendList 获取好友列表
func (c *userServiceClientImpl) GetFriendList(ctx context.Context, req *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "GetFriendList", func() (*relationpb.GetFriendListResponse, error) {
		return c.friendClient.GetFriendList(ctx, req)
	})
}

// SyncFriendList 好友增量同步
func (c *userServiceClientImpl) SyncFriendList(ctx context.Context, req *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "SyncFriendList", func() (*relationpb.SyncFriendListResponse, error) {
		return c.friendClient.SyncFriendList(ctx, req)
	})
}

// DeleteFriend 删除好友
func (c *userServiceClientImpl) DeleteFriend(ctx context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "DeleteFriend", func() (*relationpb.DeleteFriendResponse, error) {
		return c.friendClient.DeleteFriend(ctx, req)
	})
}

// SetFriendRemark 设置好友备注
func (c *userServiceClientImpl) SetFriendRemark(ctx context.Context, req *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "SetFriendRemark", func() (*relationpb.SetFriendRemarkResponse, error) {
		return c.friendClient.SetFriendRemark(ctx, req)
	})
}

// SetFriendTag 设置好友标签
func (c *userServiceClientImpl) SetFriendTag(ctx context.Context, req *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "SetFriendTag", func() (*relationpb.SetFriendTagResponse, error) {
		return c.friendClient.SetFriendTag(ctx, req)
	})
}

// GetTagList 获取标签列表
func (c *userServiceClientImpl) GetTagList(ctx context.Context, req *relationpb.GetTagListRequest) (*relationpb.GetTagListResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "GetTagList", func() (*relationpb.GetTagListResponse, error) {
		return c.friendClient.GetTagList(ctx, req)
	})
}

// CheckIsFriend 判断是否好友
func (c *userServiceClientImpl) CheckIsFriend(ctx context.Context, req *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "CheckIsFriend", func() (*relationpb.CheckIsFriendResponse, error) {
		return c.friendClient.CheckIsFriend(ctx, req)
	})
}

// BatchCheckIsFriend 批量判断是否好友
func (c *userServiceClientImpl) BatchCheckIsFriend(ctx context.Context, req *relationpb.BatchCheckIsFriendRequest) (*relationpb.BatchCheckIsFriendResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "BatchCheckIsFriend", func() (*relationpb.BatchCheckIsFriendResponse, error) {
		return c.friendClient.BatchCheckIsFriend(ctx, req)
	})
}

// GetRelationStatus 获取关系状态
func (c *userServiceClientImpl) GetRelationStatus(ctx context.Context, req *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.FriendService", "GetRelationStatus", func() (*relationpb.GetRelationStatusResponse, error) {
		return c.friendClient.GetRelationStatus(ctx, req)
	})
}

// ==================== 黑名单服务方法实现 ====================

// AddBlacklist 拉黑用户
func (c *userServiceClientImpl) AddBlacklist(ctx context.Context, req *relationpb.AddBlacklistRequest) (*relationpb.AddBlacklistResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.BlacklistService", "AddBlacklist", func() (*relationpb.AddBlacklistResponse, error) {
		return c.blacklistClient.AddBlacklist(ctx, req)
	})
}

// RemoveBlacklist 取消拉黑
func (c *userServiceClientImpl) RemoveBlacklist(ctx context.Context, req *relationpb.RemoveBlacklistRequest) (*relationpb.RemoveBlacklistResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.BlacklistService", "RemoveBlacklist", func() (*relationpb.RemoveBlacklistResponse, error) {
		return c.blacklistClient.RemoveBlacklist(ctx, req)
	})
}

// GetBlacklistList 获取黑名单列表
func (c *userServiceClientImpl) GetBlacklistList(ctx context.Context, req *relationpb.GetBlacklistListRequest) (*relationpb.GetBlacklistListResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.BlacklistService", "GetBlacklistList", func() (*relationpb.GetBlacklistListResponse, error) {
		return c.blacklistClient.GetBlacklistList(ctx, req)
	})
}

// CheckIsBlacklist 判断是否拉黑
func (c *userServiceClientImpl) CheckIsBlacklist(ctx context.Context, req *relationpb.CheckIsBlacklistRequest) (*relationpb.CheckIsBlacklistResponse, error) {
	return ExecuteWithBreaker(c.relationBreaker, "relation.BlacklistService", "CheckIsBlacklist", func() (*relationpb.CheckIsBlacklistResponse, error) {
		return c.blacklistClient.CheckIsBlacklist(ctx, req)
	})
}

// ==================== 设备会话服务方法实现 ====================

// GetDeviceList 获取设备列表
func (c *userServiceClientImpl) GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.DeviceService", "GetDeviceList", func() (*authpb.GetDeviceListResponse, error) {
		return c.deviceClient.GetDeviceList(ctx, req)
	})
}

// KickDevice 踢出设备
func (c *userServiceClientImpl) KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.DeviceService", "KickDevice", func() (*authpb.KickDeviceResponse, error) {
		return c.deviceClient.KickDevice(ctx, req)
	})
}

// GetOnlineStatus 获取用户在线状态
func (c *userServiceClientImpl) GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.DeviceService", "GetOnlineStatus", func() (*authpb.GetOnlineStatusResponse, error) {
		return c.deviceClient.GetOnlineStatus(ctx, req)
	})
}

// BatchGetOnlineStatus 批量获取在线状态
func (c *userServiceClientImpl) BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
	return ExecuteWithBreaker(c.authBreaker, "auth.DeviceService", "BatchGetOnlineStatus", func() (*authpb.BatchGetOnlineStatusResponse, error) {
		return c.deviceClient.BatchGetOnlineStatus(ctx, req)
	})
}

// ==================== 通用工具函数 ====================
// CreateConnection 通用的 gRPC 连接创建函数
// addr: 服务地址，格式为 "host:port"
// serviceConfigJSON: gRPC ServiceConfig JSON 字符串，包含重试策略等
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateConnection(addr string, serviceConfigJSON string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfigJSON), // 应用重试策略
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(4*1024*1024), // 4MB接收大小
		),
		// 注入熔断拦截器
		grpc.WithChainUnaryInterceptor(
			middleware.GRPCMetadataInterceptor(),          // 透传 trace/user/device/ip
			middleware.GRPCLoggerInterceptor(),            // 记录请求日志
			middleware.CircuitBreakerInterceptor(breaker), // 熔断器拦截器
		),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
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

// ExecuteWithBreaker 是一个独立的通用函数，不再挂载在 userServiceClientImpl 下
// breaker: 传入熔断器实例
// serviceName: 调用的服务名称 (如 "user.Service", "msg.MsgService") 用于 Prometheus 监控
// method: 方法名
// fn: 具体的业务逻辑闭包
func ExecuteWithBreaker[T any](breaker *gobreaker.CircuitBreaker, serviceName string, method string, fn func() (T, error)) (T, error) {
	start := time.Now()
	var resp T
	var err error

	// 这里的 Execute 签名取决于你使用的熔断器库
	// 假设是 sony/gobreaker，它返回 (interface{}, error)
	_, breakerErr := breaker.Execute(func() (interface{}, error) {
		result, innerErr := fn()
		resp = result // 通过闭包捕获外部变量 resp
		return result, innerErr
	})

	if breakerErr != nil {
		err = breakerErr
	}

	duration := time.Since(start).Seconds()
	// 假设 middleware 是一个全局包
	middleware.RecordGRPCRequest(serviceName, method, duration, err)

	if err != nil {
		var zero T // 高效返回零值
		return zero, err
	}

	return resp, nil
}

// ==================== gRPC 连接和熔断器初始化工具函数 ====================

// gRPC 服务配置模板，定义重试策略
const retryPolicyTemplate = `{"methodConfig": [%s]}`

const retryPolicyItemTemplate = `{
	"name": [{"service": "%s"}],
	"waitForReady": true,
	"timeout": "2s",
	"retryPolicy": {
		"maxAttempts": 5,
		"initialBackoff": "0.1s",
		"maxBackoff": "1s",
		"backoffMultiplier": 2,
		"retryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED", "UNKNOWN"]
	}
}`

// getRetryPolicy 获取指定服务的重试策略
func getRetryPolicy(serviceNames ...string) string {
	items := make([]string, 0, len(serviceNames))
	for _, serviceName := range serviceNames {
		items = append(items, fmt.Sprintf(retryPolicyItemTemplate, serviceName))
	}
	return fmt.Sprintf(retryPolicyTemplate, strings.Join(items, ","))
}

// CreateAuthServiceConnection 创建认证服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateAuthServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy(
		"auth.AuthService",
		"auth.DeviceService",
		"auth.AccountService",
		"auth.InternalAuthService",
	), breaker)
}

// CreateUserServiceConnection 创建用户服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateUserServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy("user.UserService"), breaker)
}

// CreateFriendServiceConnection 创建好友服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateFriendServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy("relation.FriendService"), breaker)
}

// CreateBlacklistServiceConnection 创建黑名单服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateBlacklistServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy("relation.BlacklistService"), breaker)
}

// CreateDeviceServiceConnection 创建设备服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateDeviceServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy("auth.DeviceService"), breaker)
}

// CreateRelationFriendServiceConnection 创建 relation-friend 服务 gRPC 连接。
// addr: relation 服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateRelationFriendServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy("relation.FriendService", "relation.BlacklistService"), breaker)
}

// CreateRelationBlacklistServiceConnection 创建 relation-blacklist 服务 gRPC 连接。
// addr: relation 服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateRelationBlacklistServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, getRetryPolicy("relation.BlacklistService"), breaker)
}
