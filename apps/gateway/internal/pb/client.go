package pb

import (
	userpb "ChatServer/apps/user/pb"
	"context"
	"time"

	"ChatServer/apps/gateway/internal/middleware"
	"ChatServer/pkg/logger"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// userServiceClientImpl 用户服务 gRPC 客户端实现
type userServiceClientImpl struct {
	authClient      userpb.AuthServiceClient
	userClient      userpb.UserServiceClient
	friendClient    userpb.FriendServiceClient
	blacklistClient userpb.BlacklistServiceClient
	deviceClient    userpb.DeviceServiceClient
	breaker         *gobreaker.CircuitBreaker
}

// NewUserServiceClient 创建用户服务 gRPC 客户端实例
// authConn: 认证服务gRPC连接
// userConn: 用户服务gRPC连接
// friendConn: 好友服务gRPC连接
// blacklistConn: 黑名单服务gRPC连接
// deviceConn: 设备服务gRPC连接
// breaker: 熔断器实例
func NewUserServiceClient(authConn, userConn, friendConn, blacklistConn, deviceConn *grpc.ClientConn, breaker *gobreaker.CircuitBreaker) UserServiceClient {
	return &userServiceClientImpl{
		authClient:      userpb.NewAuthServiceClient(authConn),
		userClient:      userpb.NewUserServiceClient(userConn),
		friendClient:    userpb.NewFriendServiceClient(friendConn),
		blacklistClient: userpb.NewBlacklistServiceClient(blacklistConn),
		deviceClient:    userpb.NewDeviceServiceClient(deviceConn),
		breaker:         breaker,
	}
}

// ==================== 认证服务方法实现 ====================

// Login 用户登录
func (c *userServiceClientImpl) Login(ctx context.Context, req *userpb.LoginRequest) (*userpb.LoginResponse, error) {
	return ExecuteWithBreaker(c.breaker, "Login", func() (*userpb.LoginResponse, error) {
		return c.authClient.Login(ctx, req)
	})
}

// LoginByCode 验证码登录
func (c *userServiceClientImpl) LoginByCode(ctx context.Context, req *userpb.LoginByCodeRequest) (*userpb.LoginByCodeResponse, error) {
	return ExecuteWithBreaker(c.breaker, "LoginByCode", func() (*userpb.LoginByCodeResponse, error) {
		return c.authClient.LoginByCode(ctx, req)
	})
}

// SendVerifyCode 发送验证码
func (c *userServiceClientImpl) SendVerifyCode(ctx context.Context, req *userpb.SendVerifyCodeRequest) (*userpb.SendVerifyCodeResponse, error) {
	return ExecuteWithBreaker(c.breaker, "SendVerifyCode", func() (*userpb.SendVerifyCodeResponse, error) {
		return c.authClient.SendVerifyCode(ctx, req)
	})
}

// VerifyCode 校验验证码
func (c *userServiceClientImpl) VerifyCode(ctx context.Context, req *userpb.VerifyCodeRequest) (*userpb.VerifyCodeResponse, error) {
	return ExecuteWithBreaker(c.breaker, "VerifyCode", func() (*userpb.VerifyCodeResponse, error) {
		return c.authClient.VerifyCode(ctx, req)
	})
}

// RefreshToken 刷新Token
func (c *userServiceClientImpl) RefreshToken(ctx context.Context, req *userpb.RefreshTokenRequest) (*userpb.RefreshTokenResponse, error) {
	return ExecuteWithBreaker(c.breaker, "RefreshToken", func() (*userpb.RefreshTokenResponse, error) {
		return c.authClient.RefreshToken(ctx, req)
	})
}

// Logout 用户登出
func (c *userServiceClientImpl) Logout(ctx context.Context, req *userpb.LogoutRequest) (*userpb.LogoutResponse, error) {
	return ExecuteWithBreaker(c.breaker, "Logout", func() (*userpb.LogoutResponse, error) {
		return c.authClient.Logout(ctx, req)
	})
}

// ResetPassword 重置密码
func (c *userServiceClientImpl) ResetPassword(ctx context.Context, req *userpb.ResetPasswordRequest) (*userpb.ResetPasswordResponse, error) {
	return ExecuteWithBreaker(c.breaker, "ResetPassword", func() (*userpb.ResetPasswordResponse, error) {
		return c.authClient.ResetPassword(ctx, req)
	})
}

// ==================== 用户信息服务方法实现 ====================

// GetProfile 获取个人信息
func (c *userServiceClientImpl) GetProfile(ctx context.Context, req *userpb.GetProfileRequest) (*userpb.GetProfileResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetProfile", func() (*userpb.GetProfileResponse, error) {
		return c.userClient.GetProfile(ctx, req)
	})
}

// GetOtherProfile 获取他人信息
func (c *userServiceClientImpl) GetOtherProfile(ctx context.Context, req *userpb.GetOtherProfileRequest) (*userpb.GetOtherProfileResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetOtherProfile", func() (*userpb.GetOtherProfileResponse, error) {
		return c.userClient.GetOtherProfile(ctx, req)
	})
}

// UpdateProfile 更新基本信息
func (c *userServiceClientImpl) UpdateProfile(ctx context.Context, req *userpb.UpdateProfileRequest) (*userpb.UpdateProfileResponse, error) {
	return ExecuteWithBreaker(c.breaker, "UpdateProfile", func() (*userpb.UpdateProfileResponse, error) {
		return c.userClient.UpdateProfile(ctx, req)
	})
}

// UploadAvatar 上传头像
func (c *userServiceClientImpl) UploadAvatar(ctx context.Context, req *userpb.UploadAvatarRequest) (*userpb.UploadAvatarResponse, error) {
	return ExecuteWithBreaker(c.breaker, "UploadAvatar", func() (*userpb.UploadAvatarResponse, error) {
		return c.userClient.UploadAvatar(ctx, req)
	})
}

// ChangePassword 修改密码
func (c *userServiceClientImpl) ChangePassword(ctx context.Context, req *userpb.ChangePasswordRequest) (*userpb.ChangePasswordResponse, error) {
	return ExecuteWithBreaker(c.breaker, "ChangePassword", func() (*userpb.ChangePasswordResponse, error) {
		return c.userClient.ChangePassword(ctx, req)
	})
}

// ChangeEmail 绑定/换绑邮箱
func (c *userServiceClientImpl) ChangeEmail(ctx context.Context, req *userpb.ChangeEmailRequest) (*userpb.ChangeEmailResponse, error) {
	return ExecuteWithBreaker(c.breaker, "ChangeEmail", func() (*userpb.ChangeEmailResponse, error) {
		return c.userClient.ChangeEmail(ctx, req)
	})
}

// ChangeTelephone 绑定/换绑手机
func (c *userServiceClientImpl) ChangeTelephone(ctx context.Context, req *userpb.ChangeTelephoneRequest) (*userpb.ChangeTelephoneResponse, error) {
	return ExecuteWithBreaker(c.breaker, "ChangeTelephone", func() (*userpb.ChangeTelephoneResponse, error) {
		return c.userClient.ChangeTelephone(ctx, req)
	})
}

// GetQRCode 获取用户二维码
func (c *userServiceClientImpl) GetQRCode(ctx context.Context, req *userpb.GetQRCodeRequest) (*userpb.GetQRCodeResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetQRCode", func() (*userpb.GetQRCodeResponse, error) {
		return c.userClient.GetQRCode(ctx, req)
	})
}

// ParseQRCode 解析二维码
func (c *userServiceClientImpl) ParseQRCode(ctx context.Context, req *userpb.ParseQRCodeRequest) (*userpb.ParseQRCodeResponse, error) {
	return ExecuteWithBreaker(c.breaker, "ParseQRCode", func() (*userpb.ParseQRCodeResponse, error) {
		return c.userClient.ParseQRCode(ctx, req)
	})
}

// DeleteAccount 注销账号
func (c *userServiceClientImpl) DeleteAccount(ctx context.Context, req *userpb.DeleteAccountRequest) (*userpb.DeleteAccountResponse, error) {
	return ExecuteWithBreaker(c.breaker, "DeleteAccount", func() (*userpb.DeleteAccountResponse, error) {
		return c.userClient.DeleteAccount(ctx, req)
	})
}

// BatchGetProfile 批量获取用户信息
func (c *userServiceClientImpl) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	return ExecuteWithBreaker(c.breaker, "BatchGetProfile", func() (*userpb.BatchGetProfileResponse, error) {
		return c.userClient.BatchGetProfile(ctx, req)
	})
}

// ==================== 好友服务方法实现 ====================

// SearchUser 搜索用户
func (c *userServiceClientImpl) SearchUser(ctx context.Context, req *userpb.SearchUserRequest) (*userpb.SearchUserResponse, error) {
	return ExecuteWithBreaker(c.breaker, "SearchUser", func() (*userpb.SearchUserResponse, error) {
		return c.friendClient.SearchUser(ctx, req)
	})
}

// SendFriendApply 发送好友申请
func (c *userServiceClientImpl) SendFriendApply(ctx context.Context, req *userpb.SendFriendApplyRequest) (*userpb.SendFriendApplyResponse, error) {
	return ExecuteWithBreaker(c.breaker, "SendFriendApply", func() (*userpb.SendFriendApplyResponse, error) {
		return c.friendClient.SendFriendApply(ctx, req)
	})
}

// GetFriendApplyList 获取好友申请列表
func (c *userServiceClientImpl) GetFriendApplyList(ctx context.Context, req *userpb.GetFriendApplyListRequest) (*userpb.GetFriendApplyListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetFriendApplyList", func() (*userpb.GetFriendApplyListResponse, error) {
		return c.friendClient.GetFriendApplyList(ctx, req)
	})
}

// GetSentApplyList 获取发出的申请列表
func (c *userServiceClientImpl) GetSentApplyList(ctx context.Context, req *userpb.GetSentApplyListRequest) (*userpb.GetSentApplyListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetSentApplyList", func() (*userpb.GetSentApplyListResponse, error) {
		return c.friendClient.GetSentApplyList(ctx, req)
	})
}

// HandleFriendApply 处理好友申请
func (c *userServiceClientImpl) HandleFriendApply(ctx context.Context, req *userpb.HandleFriendApplyRequest) (*userpb.HandleFriendApplyResponse, error) {
	return ExecuteWithBreaker(c.breaker, "HandleFriendApply", func() (*userpb.HandleFriendApplyResponse, error) {
		return c.friendClient.HandleFriendApply(ctx, req)
	})
}

// GetUnreadApplyCount 获取未读申请数量
func (c *userServiceClientImpl) GetUnreadApplyCount(ctx context.Context, req *userpb.GetUnreadApplyCountRequest) (*userpb.GetUnreadApplyCountResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetUnreadApplyCount", func() (*userpb.GetUnreadApplyCountResponse, error) {
		return c.friendClient.GetUnreadApplyCount(ctx, req)
	})
}

// MarkApplyAsRead 标记申请已读
func (c *userServiceClientImpl) MarkApplyAsRead(ctx context.Context, req *userpb.MarkApplyAsReadRequest) (*userpb.MarkApplyAsReadResponse, error) {
	return ExecuteWithBreaker(c.breaker, "MarkApplyAsRead", func() (*userpb.MarkApplyAsReadResponse, error) {
		return c.friendClient.MarkApplyAsRead(ctx, req)
	})
}

// GetFriendList 获取好友列表
func (c *userServiceClientImpl) GetFriendList(ctx context.Context, req *userpb.GetFriendListRequest) (*userpb.GetFriendListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetFriendList", func() (*userpb.GetFriendListResponse, error) {
		return c.friendClient.GetFriendList(ctx, req)
	})
}

// SyncFriendList 好友增量同步
func (c *userServiceClientImpl) SyncFriendList(ctx context.Context, req *userpb.SyncFriendListRequest) (*userpb.SyncFriendListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "SyncFriendList", func() (*userpb.SyncFriendListResponse, error) {
		return c.friendClient.SyncFriendList(ctx, req)
	})
}

// DeleteFriend 删除好友
func (c *userServiceClientImpl) DeleteFriend(ctx context.Context, req *userpb.DeleteFriendRequest) (*userpb.DeleteFriendResponse, error) {
	return ExecuteWithBreaker(c.breaker, "DeleteFriend", func() (*userpb.DeleteFriendResponse, error) {
		return c.friendClient.DeleteFriend(ctx, req)
	})
}

// SetFriendRemark 设置好友备注
func (c *userServiceClientImpl) SetFriendRemark(ctx context.Context, req *userpb.SetFriendRemarkRequest) (*userpb.SetFriendRemarkResponse, error) {
	return ExecuteWithBreaker(c.breaker, "SetFriendRemark", func() (*userpb.SetFriendRemarkResponse, error) {
		return c.friendClient.SetFriendRemark(ctx, req)
	})
}

// SetFriendTag 设置好友标签
func (c *userServiceClientImpl) SetFriendTag(ctx context.Context, req *userpb.SetFriendTagRequest) (*userpb.SetFriendTagResponse, error) {
	return ExecuteWithBreaker(c.breaker, "SetFriendTag", func() (*userpb.SetFriendTagResponse, error) {
		return c.friendClient.SetFriendTag(ctx, req)
	})
}

// GetTagList 获取标签列表
func (c *userServiceClientImpl) GetTagList(ctx context.Context, req *userpb.GetTagListRequest) (*userpb.GetTagListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetTagList", func() (*userpb.GetTagListResponse, error) {
		return c.friendClient.GetTagList(ctx, req)
	})
}

// CheckIsFriend 判断是否好友
func (c *userServiceClientImpl) CheckIsFriend(ctx context.Context, req *userpb.CheckIsFriendRequest) (*userpb.CheckIsFriendResponse, error) {
	return ExecuteWithBreaker(c.breaker, "CheckIsFriend", func() (*userpb.CheckIsFriendResponse, error) {
		return c.friendClient.CheckIsFriend(ctx, req)
	})
}

// GetRelationStatus 获取关系状态
func (c *userServiceClientImpl) GetRelationStatus(ctx context.Context, req *userpb.GetRelationStatusRequest) (*userpb.GetRelationStatusResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetRelationStatus", func() (*userpb.GetRelationStatusResponse, error) {
		return c.friendClient.GetRelationStatus(ctx, req)
	})
}

// ==================== 黑名单服务方法实现 ====================

// AddBlacklist 拉黑用户
func (c *userServiceClientImpl) AddBlacklist(ctx context.Context, req *userpb.AddBlacklistRequest) (*userpb.AddBlacklistResponse, error) {
	return ExecuteWithBreaker(c.breaker, "AddBlacklist", func() (*userpb.AddBlacklistResponse, error) {
		return c.blacklistClient.AddBlacklist(ctx, req)
	})
}

// RemoveBlacklist 取消拉黑
func (c *userServiceClientImpl) RemoveBlacklist(ctx context.Context, req *userpb.RemoveBlacklistRequest) (*userpb.RemoveBlacklistResponse, error) {
	return ExecuteWithBreaker(c.breaker, "RemoveBlacklist", func() (*userpb.RemoveBlacklistResponse, error) {
		return c.blacklistClient.RemoveBlacklist(ctx, req)
	})
}

// GetBlacklistList 获取黑名单列表
func (c *userServiceClientImpl) GetBlacklistList(ctx context.Context, req *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetBlacklistList", func() (*userpb.GetBlacklistListResponse, error) {
		return c.blacklistClient.GetBlacklistList(ctx, req)
	})
}

// CheckIsBlacklist 判断是否拉黑
func (c *userServiceClientImpl) CheckIsBlacklist(ctx context.Context, req *userpb.CheckIsBlacklistRequest) (*userpb.CheckIsBlacklistResponse, error) {
	return ExecuteWithBreaker(c.breaker, "CheckIsBlacklist", func() (*userpb.CheckIsBlacklistResponse, error) {
		return c.blacklistClient.CheckIsBlacklist(ctx, req)
	})
}

// ==================== 设备会话服务方法实现 ====================

// GetDeviceList 获取设备列表
func (c *userServiceClientImpl) GetDeviceList(ctx context.Context, req *userpb.GetDeviceListRequest) (*userpb.GetDeviceListResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetDeviceList", func() (*userpb.GetDeviceListResponse, error) {
		return c.deviceClient.GetDeviceList(ctx, req)
	})
}

// KickDevice 踢出设备
func (c *userServiceClientImpl) KickDevice(ctx context.Context, req *userpb.KickDeviceRequest) (*userpb.KickDeviceResponse, error) {
	return ExecuteWithBreaker(c.breaker, "KickDevice", func() (*userpb.KickDeviceResponse, error) {
		return c.deviceClient.KickDevice(ctx, req)
	})
}

// GetOnlineStatus 获取用户在线状态
func (c *userServiceClientImpl) GetOnlineStatus(ctx context.Context, req *userpb.GetOnlineStatusRequest) (*userpb.GetOnlineStatusResponse, error) {
	return ExecuteWithBreaker(c.breaker, "GetOnlineStatus", func() (*userpb.GetOnlineStatusResponse, error) {
		return c.deviceClient.GetOnlineStatus(ctx, req)
	})
}

// BatchGetOnlineStatus 批量获取在线状态
func (c *userServiceClientImpl) BatchGetOnlineStatus(ctx context.Context, req *userpb.BatchGetOnlineStatusRequest) (*userpb.BatchGetOnlineStatusResponse, error) {
	return ExecuteWithBreaker(c.breaker, "BatchGetOnlineStatus", func() (*userpb.BatchGetOnlineStatusResponse, error) {
		return c.deviceClient.BatchGetOnlineStatus(ctx, req)
	})
}

// ==================== 通用工具函数 ====================

// ExecuteWithBreaker 是一个独立的通用函数，不再挂载在 userServiceClientImpl 下
// breaker: 传入熔断器实例
// method: 方法名
// fn: 具体的业务逻辑闭包
func ExecuteWithBreaker[T any](breaker *gobreaker.CircuitBreaker, method string, fn func() (T, error)) (T, error) {
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
    middleware.RecordGRPCRequest("user.Service", method, duration, err)

    if err != nil {
        var zero T // 高效返回零值
        return zero, err
    }

    return resp, nil
}

// ==================== gRPC 连接和熔断器初始化工具函数 ====================

// gRPC 服务配置，定义重试策略
const retryPolicy = `{
	"methodConfig": [{
		"name": [{"service": "user.AuthService"}],
		"waitForReady": true,
		"timeout": "2s",
		"retryPolicy": {
			"maxAttempts": 5,
			"initialBackoff": "0.1s",
			"maxBackoff": "1s",
			"backoffMultiplier": 2,
			"retryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED", "UNKNOWN"]
		}
	}]
}`

// CreateAuthServiceConnection 创建认证服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateAuthServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, "user.AuthService", breaker)
}

// CreateUserServiceConnection 创建用户服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateUserServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, "user.UserService", breaker)
}

// CreateFriendServiceConnection 创建好友服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateFriendServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, "user.FriendService", breaker)
}

// CreateBlacklistServiceConnection 创建黑名单服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateBlacklistServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, "user.BlacklistService", breaker)
}

// CreateDeviceServiceConnection 创建设备服务 gRPC 连接
// addr: 用户服务地址，格式为 "host:port"
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateDeviceServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return CreateConnection(addr, "user.DeviceService", breaker)
}

// CreateConnection 通用的 gRPC 连接创建函数
// addr: 服务地址，格式为 "host:port"
// serviceName: 服务名称（用于重试策略配置）
// breaker: 熔断器实例
// 返回: gRPC 连接和错误
func CreateConnection(addr string, serviceName string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(retryPolicy), // 应用重试策略
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(4*1024*1024), // 4MB接收大小
		),
		// 注入熔断拦截器
		grpc.WithChainUnaryInterceptor(
			middleware.CircuitBreakerInterceptor(breaker),
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
