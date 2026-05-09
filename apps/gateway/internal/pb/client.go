package pb

import (
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
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
