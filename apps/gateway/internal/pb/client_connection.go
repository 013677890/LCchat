package pb

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
)

const gatewayMaxCallRecvMsgSize = 4 * 1024 * 1024

// newGatewayConnection 把 gateway 出站 gRPC 连接的公共装配统一收口到一个地方。
// 统一内容包括：
//   - metadata 透传
//   - 方法级 timeout
//   - 精确 full method 级 retry 白名单（默认不重试）
//   - method-scoped internal-caller
//   - gateway 特有的熔断保护
//   - 下游 gRPC 指标观测
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

// gateway 下游重试白名单：仅只读或天然幂等查询。
// 写方法（Register/Login/CreateGroup/SendFriendApply/SendMessage 等）一律不配置式重试，
// 避免“服务端已提交、响应丢失”时被自动重放产生重复副作用。
// SendMessage 虽有 client_msg_id，本次仍排除；若要恢复须另加证明无副作用的专项测试。

var gatewayAuthRetryMethods = []string{
	"/auth.DeviceService/GetDeviceList",
	"/auth.DeviceService/GetOnlineStatus",
	"/auth.DeviceService/BatchGetOnlineStatus",
	"/auth.InternalAuthService/FindAccountByEmail",
}

var gatewayUserRetryMethods = []string{
	"/user.UserService/GetProfile",
	"/user.UserService/GetOtherProfile",
	"/user.UserService/SearchUser",
	"/user.UserService/GetQRCode",
	"/user.UserService/ParseQRCode",
	"/user.UserService/BatchGetProfile",
}

var gatewayRelationRetryMethods = []string{
	"/relation.FriendService/GetFriendApplyList",
	"/relation.FriendService/GetSentApplyList",
	"/relation.FriendService/GetUnreadApplyCount",
	"/relation.FriendService/GetFriendList",
	"/relation.FriendService/SyncFriendList",
	"/relation.FriendService/CheckIsFriend",
	"/relation.FriendService/BatchCheckIsFriend",
	"/relation.FriendService/GetRelationStatus",
	"/relation.BlacklistService/GetBlacklistList",
	"/relation.BlacklistService/CheckIsBlacklist",
}

var gatewayGroupRetryMethods = []string{
	"/group.GroupService/GetGroupInfo",
	"/group.GroupService/GetMyJoinGroupApplication",
	"/group.GroupService/ListMyJoinGroupApplications",
	"/group.GroupService/ListJoinRequests",
	"/group.GroupService/ListReviewedJoinRequests",
	"/group.GroupService/GetMemberList",
	"/group.GroupService/SearchGroupMembers",
	"/group.GroupService/GetGroupList",
	"/group.GroupService/SearchGroups",
	"/group.GroupService/GetGroupMemberIds",
	"/group.GroupService/CheckGroupMember",
	"/group.GroupService/CheckGroupSendPermission",
	"/group.GroupService/GetJoinRequestPendingCount",
}

var gatewayMsgRetryMethods = []string{
	"/msg.MsgService/PullMessages",
	"/msg.MsgService/BatchSyncMessages",
	"/msg.MsgService/GetMessagesByIds",
	"/msg.MsgService/GetConversations",
}

// CreateAuthServiceConnection 创建认证服务 gRPC 连接。
func CreateAuthServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// auth 连接同时承载公开接口与内部接口，
	// 因此 internal-caller 只对白名单内部方法注入，避免污染公开调用。
	// 配置式重试仅覆盖设备查询与内部账号查询，写接口（注册/登录/验证码等）不重试。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(gatewayAuthRetryMethods...), []string{
		"/auth.InternalAuthService/FindAccountByEmail",
	}, breaker)
}

// CreateUserServiceConnection 创建用户服务 gRPC 连接。
func CreateUserServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// 仅资料/搜索/批量读取可重试；UpdateProfile/UploadAvatar 等写接口不重试。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(gatewayUserRetryMethods...), nil, breaker)
}

// CreateRelationServiceConnection 创建 relation-service gRPC 连接。
func CreateRelationServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// 列表、同步、关系/黑名单检查可重试；SendFriendApply/HandleFriendApply 等写接口不重试。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(gatewayRelationRetryMethods...), nil, breaker)
}

// CreateMsgServiceConnection 创建消息服务 gRPC 连接。
func CreateMsgServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// 仅 Pull/BatchSync/Get 类读取可重试；SendMessage/RecallMessage/MarkRead 等写接口不重试。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(gatewayMsgRetryMethods...), nil, breaker)
}

// CreateGroupServiceConnection 创建群组服务 gRPC 连接。
func CreateGroupServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// Get/List/Search/Check 类读取可重试；CreateGroup/AddMember/ReviewJoinGroup 等写接口不重试。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(gatewayGroupRetryMethods...), nil, breaker)
}
