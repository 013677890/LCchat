package grpcx

import (
	"context"
	"google.golang.org/grpc"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

// ServerOptions 定义 gRPC Server 的启动参数。
type ServerOptions struct {
	// Address 监听地址，例如 ":9090"。
	Address string
	// Namespace 服务名前缀，用于 Prometheus 指标命名空间。
	// 例如 "user" → user_grpc_request_total。
	Namespace string

	// RateLimit 限流参数，nil 时使用 DefaultRateLimitConfig()。
	RateLimit *RateLimitConfig
	// Logging 日志参数，nil 时使用 DefaultLoggingConfig()。
	Logging *LoggingConfig
	// MetricsConfig 指标参数，nil 时使用 DefaultMetricsConfig() + Namespace。
	MetricsConfig *MetricsConfig

	// ExtraUnaryInterceptors 业务方自定义的额外 Unary 拦截器，
	// 追加到内置拦截器链之后。
	ExtraUnaryInterceptors []grpc.UnaryServerInterceptor
	// ExtraStreamInterceptors 业务方自定义的额外 Stream 拦截器。
	ExtraStreamInterceptors []grpc.StreamServerInterceptor

	// MaxRecvMsgSize 最大接收包大小（字节），0 表示不限制。
	MaxRecvMsgSize int
	// MaxSendMsgSize 最大发送包大小（字节），0 表示不限制。
	MaxSendMsgSize int
	// EnableHealth 是否注册 gRPC 健康检查服务。
	EnableHealth bool
	// EnableReflection 是否开启 gRPC 反射（建议仅在开发环境开启）。
	EnableReflection bool
}

// ServerResult 包含 Start 后可供外部使用的组件。
type ServerResult struct {
	// Metrics 指标实例，可用于获取 HTTP handler（暴露 /metrics）。
	Metrics *Metrics
}

// Start 保留向后兼容入口，内部委托到可被 Wire 管理的构造与运行接口。
func Start(ctx context.Context, opts ServerOptions, register func(s *grpc.Server, health healthgrpc.HealthServer)) (*ServerResult, error) {
	built, err := NewServer(opts, register)
	if err != nil {
		return nil, err
	}

	lis, err := NewListener(opts.Address)
	if err != nil {
		return &ServerResult{Metrics: built.Metrics}, err
	}
	defer func() { _ = lis.Close() }()

	if err := Run(ctx, built.Server, lis); err != nil {
		return &ServerResult{Metrics: built.Metrics}, err
	}
	return &ServerResult{Metrics: built.Metrics}, nil
}
