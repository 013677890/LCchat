package grpcx

import (
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ServerOptions 定义 gRPC Server 的启动参数。
type ServerOptions struct {
	// Namespace 服务名前缀，用于 Prometheus 指标命名空间。
	Namespace string

	// RateLimit 限流参数，nil 时使用 DefaultRateLimitConfig()。
	RateLimit *RateLimitConfig
	// Logging 日志参数，nil 时使用 DefaultLoggingConfig()。
	Logging *LoggingConfig
	// Timeout 请求级兜底 deadline，nil 或 DefaultTimeout<=0 时不启用服务端兜底。
	Timeout *TimeoutConfig
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

// RegistrationFunc 负责向 gRPC Server 注册业务服务。
type RegistrationFunc func(s *grpc.Server)

// BuiltServer 表示已构建但尚未运行的 gRPC 运行时对象。
type BuiltServer struct {
	// Server 由上层 App 保存并交给 Serve、GracefulStop 管理。
	Server *grpc.Server
	// Metrics 与 Server 使用同一个采集实例，供独立的 metrics HTTP 端点暴露。
	Metrics *Metrics
}

// NewServer 仅负责构建 gRPC Server，不负责监听和阻塞运行。
// 这样 provider 可以安全复用它，而不会在依赖注入阶段意外启动网络服务。
func NewServer(opts ServerOptions, register RegistrationFunc) (*BuiltServer, error) {
	if register == nil {
		return nil, errors.New("grpc register func is nil")
	}

	// 每个服务创建独立 Registry；Namespace 默认跟随 ServerOptions，避免指标冲突。
	metricsCfg := DefaultMetricsConfig()
	metricsCfg.Namespace = opts.Namespace
	if opts.MetricsConfig != nil {
		metricsCfg = *opts.MetricsConfig
		if metricsCfg.Namespace == "" {
			metricsCfg.Namespace = opts.Namespace
		}
	}

	metrics := NewMetrics(metricsCfg)

	// Unary 治理链在 server_chain.go 集中装配，builder 只消费最终结果。
	unaryInters := buildServerUnaryInterceptors(opts, metrics)

	// 消息大小属于传输层能力；业务治理拦截器不应感知这些限制。
	var serverOpts []grpc.ServerOption
	if opts.MaxRecvMsgSize > 0 {
		serverOpts = append(serverOpts, grpc.MaxRecvMsgSize(opts.MaxRecvMsgSize))
	}
	if opts.MaxSendMsgSize > 0 {
		serverOpts = append(serverOpts, grpc.MaxSendMsgSize(opts.MaxSendMsgSize))
	}
	serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(unaryInters...))
	if len(opts.ExtraStreamInterceptors) > 0 {
		serverOpts = append(serverOpts, grpc.ChainStreamInterceptor(opts.ExtraStreamInterceptors...))
	}

	s := grpc.NewServer(serverOpts...)

	// 基础设施服务先注册，随后注册业务服务；反射最后开启，确保能看到完整描述。
	if opts.EnableHealth {
		healthgrpc.RegisterHealthServer(s, newHealthServer())
	}

	register(s)
	if opts.EnableReflection {
		reflection.Register(s)
	}

	return &BuiltServer{
		Server:  s,
		Metrics: metrics,
	}, nil
}

// newHealthServer 返回默认处于 SERVING 状态的标准 gRPC 健康检查实现。
func newHealthServer() healthgrpc.HealthServer {
	hs := health.NewServer()
	hs.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	return hs
}
