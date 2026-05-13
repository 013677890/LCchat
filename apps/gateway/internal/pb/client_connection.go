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
//   - 精确 service 级 retry
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
// CreateAuthServiceConnection 创建认证服务 gRPC 连接。
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
// CreateUserServiceConnection 创建用户服务 gRPC 连接。
func CreateUserServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig("user.UserService"), nil, breaker)
}
// CreateRelationServiceConnection 创建 relation-service gRPC 连接。
func CreateRelationServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig(
		"relation.FriendService",
		"relation.BlacklistService",
	), nil, breaker)
}
// CreateMsgServiceConnection 创建消息服务 gRPC 连接。
func CreateMsgServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	// gateway 调用 msg-service 只会落到 MsgService，
	// 因此重试范围保持单一 service，底层装配则复用统一 grpcx builder。
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig("msg.MsgService"), nil, breaker)
}
// CreateGroupServiceConnection 创建群组服务 gRPC 连接。
func CreateGroupServiceConnection(addr string, breaker *gobreaker.CircuitBreaker) (*grpc.ClientConn, error) {
	return newGatewayConnection(addr, grpcx.DefaultClientRetryConfig("group.GroupService"), nil, breaker)
}
