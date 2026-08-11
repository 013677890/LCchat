package grpcx

import (
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientOptions 定义统一的 gRPC 客户端构造参数。
//
// 设计目标：
//   - 让 metadata / timeout / logging / internal-caller / retry 的装配顺序固定；
//   - 让各服务只保留“这条连接连谁、需要哪些策略”的最小差异；
//   - 业务方若有额外需求（如 gateway 的熔断器），通过 ExtraUnaryInterceptors 注入。
type ClientOptions struct {
	// Address 是 gRPC target，例如 "auth-service:9090"。
	Address string

	// Credentials 允许调用方覆盖传输凭证；未显式指定时默认走 insecure，
	// 与当前仓库内部服务直连的现状保持一致。
	Credentials credentials.TransportCredentials

	// Timeout 为 nil 时不额外收紧调用 deadline，仍服从调用方 context。
	Timeout *ClientTimeoutConfig
	// Logging 为 nil 时启用仓库统一的默认出站日志配置。
	Logging *ClientLoggingConfig
	// InternalCaller 控制 x-internal-caller 的注入范围；nil 时完全不注入。
	InternalCaller *InternalCallerClientConfig
	// Retry 只对明确列出的 full method 生效；nil 不代表关闭负载均衡配置。
	Retry *ClientRetryConfig
	// Observers 在一次 RPC 返回后接收耗时、状态码等汇总结果。
	Observers []ClientCallObserver

	// LoadBalancingPolicy 写入 gRPC service config。
	// 默认使用 round_robin，让 Docker Compose DNS 返回多个后端地址时能分摊请求。
	LoadBalancingPolicy string

	// ExtraUnaryInterceptors 位于内置拦截器和 Observers 之后，适合注入熔断等
	// 只属于特定调用方的策略。
	ExtraUnaryInterceptors []grpc.UnaryClientInterceptor

	// MaxRecvMsgSize / MaxSendMsgSize 作用于默认 CallOptions。
	MaxRecvMsgSize int
	MaxSendMsgSize int
}

// NewClient 用统一装配顺序创建 gRPC ClientConn。
// grpc.NewClient 只创建逻辑连接，不在这里阻塞等待网络连通；首次 RPC 时才会触发连接。
func NewClient(opts ClientOptions) (*grpc.ClientConn, error) {
	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return nil, fmt.Errorf("grpc client address is empty")
	}

	transportCredentials := opts.Credentials
	if transportCredentials == nil {
		transportCredentials = insecure.NewCredentials()
	}

	// 拦截器与 DialOption 分开构建，避免连接参数和请求治理策略交织在一起。
	interceptors := buildClientUnaryInterceptors(opts)
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(transportCredentials)}
	if serviceConfigJSON, err := buildClientServiceConfig(opts.Retry, opts.LoadBalancingPolicy); err != nil {
		return nil, err
	} else if serviceConfigJSON != "" {
		dialOptions = append(dialOptions, grpc.WithDefaultServiceConfig(serviceConfigJSON))
	}

	callOptions := buildClientCallOptions(opts)
	if len(callOptions) > 0 {
		dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(callOptions...))
	}
	if len(interceptors) > 0 {
		dialOptions = append(dialOptions, grpc.WithChainUnaryInterceptor(interceptors...))
	}

	return grpc.NewClient(address, dialOptions...)
}

func buildClientCallOptions(opts ClientOptions) []grpc.CallOption {
	callOptions := make([]grpc.CallOption, 0, 2)
	if opts.MaxRecvMsgSize > 0 {
		callOptions = append(callOptions, grpc.MaxCallRecvMsgSize(opts.MaxRecvMsgSize))
	}
	if opts.MaxSendMsgSize > 0 {
		callOptions = append(callOptions, grpc.MaxCallSendMsgSize(opts.MaxSendMsgSize))
	}
	return callOptions
}
