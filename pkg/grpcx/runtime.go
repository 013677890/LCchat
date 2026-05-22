package grpcx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ServerOptions 定义 gRPC Server 的启动参数。
type ServerOptions struct {
	// Address 监听地址，例如 ":9090"。
	Address string
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
	Server  *grpc.Server
	Metrics *Metrics
}

// NewServer 仅负责构建 gRPC Server，不负责监听和阻塞运行。
// 这样 provider 可以安全复用它，而不会在依赖注入阶段意外启动网络服务。
func NewServer(opts ServerOptions, register RegistrationFunc) (*BuiltServer, error) {
	if register == nil {
		return nil, errors.New("grpc register func is nil")
	}

	metricsCfg := DefaultMetricsConfig()
	metricsCfg.Namespace = opts.Namespace
	if opts.MetricsConfig != nil {
		metricsCfg = *opts.MetricsConfig
		if metricsCfg.Namespace == "" {
			metricsCfg.Namespace = opts.Namespace
		}
	}
	metrics := NewMetrics(metricsCfg)

	var rateLimitCfg RateLimitConfig
	if opts.RateLimit != nil {
		rateLimitCfg = *opts.RateLimit
	} else {
		rateLimitCfg = DefaultRateLimitConfig()
	}

	var loggingCfg LoggingConfig
	if opts.Logging != nil {
		loggingCfg = *opts.Logging
	} else {
		loggingCfg = DefaultLoggingConfig()
	}

	var timeoutCfg TimeoutConfig
	if opts.Timeout != nil {
		timeoutCfg = *opts.Timeout
	}

	unaryInters := []grpc.UnaryServerInterceptor{
		RecoveryUnaryInterceptor(),
		MetadataUnaryInterceptor(),
		// timeout 要尽早生效，这样后续 validate / rate-limit / metrics / 业务处理
		// 看到的都是同一个被收紧后的请求级 deadline。
		TimeoutUnaryInterceptor(timeoutCfg),
		ValidateUnaryInterceptor(),
		RateLimitUnaryInterceptor(rateLimitCfg),
		metrics.UnaryInterceptor(),
		ErrorNormalizeUnaryInterceptor(),
		LoggingUnaryInterceptor(loggingCfg),
	}
	unaryInters = append(unaryInters, opts.ExtraUnaryInterceptors...)

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

// NewListener 创建 gRPC TCP 监听器。
func NewListener(address string) (net.Listener, error) {
	if address == "" {
		return nil, errors.New("grpc listen address is empty")
	}
	return net.Listen("tcp", address)
}

// Run 使用已构建的 gRPC Server 在指定监听器上阻塞运行。
// 它同时监听 ctx.Done()，把“服务停止时机”从旧版 Start 中解耦出来，
// 交由上层 App 或 main 统一控制。
func Run(ctx context.Context, server *grpc.Server, listener net.Listener) error {
	if server == nil {
		return errors.New("grpc server is nil")
	}
	if listener == nil {
		return errors.New("grpc listener is nil")
	}

	stopCtx, stopCancel := context.WithCancel(context.Background())
	defer stopCancel()

	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := GracefulStop(shutdownCtx, server, 10*time.Second); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				logger.Warn(ctx, "gRPC 服务停止时发生异常", logger.ErrorField("error", err))
			}
		case <-stopCtx.Done():
		}
	}()

	logger.Info(ctx, "gRPC 服务启动", logger.String("addr", listener.Addr().String()))
	if err := server.Serve(listener); err != nil {
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("grpc serve failed: %w", err)
	}
	return nil
}

// GracefulStop 负责优雅停止 gRPC Server。
// 超时后会退化为强制 Stop，避免关闭流程被永久卡住。
func GracefulStop(ctx context.Context, server *grpc.Server, timeout time.Duration) error {
	if server == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	stopDone := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		logger.Info(ctx, "gRPC 服务已优雅停止")
		return nil
	case <-time.After(timeout):
		logger.Warn(ctx, "gRPC 优雅停机超时，执行强制停止", logger.Duration("timeout", timeout))
		server.Stop()
		return context.DeadlineExceeded
	}
}

func newHealthServer() healthgrpc.HealthServer {
	hs := health.NewServer()
	hs.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	return hs
}
