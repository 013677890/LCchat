package grpcx

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
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

// Start 创建并启动 gRPC Server。
// register 回调中完成业务服务的注册。
// 返回 ServerResult 供调用方获取 Metrics Handler 等组件。
// 此函数会阻塞直到服务停止。
func Start(ctx context.Context, opts ServerOptions, register func(s *grpc.Server, health healthgrpc.HealthServer)) (*ServerResult, error) {
	// 构建 Metrics
	metricsCfg := DefaultMetricsConfig()
	metricsCfg.Namespace = opts.Namespace
	if opts.MetricsConfig != nil {
		metricsCfg = *opts.MetricsConfig
		if metricsCfg.Namespace == "" {
			metricsCfg.Namespace = opts.Namespace
		}
	}
	metrics := NewMetrics(metricsCfg)

	result := &ServerResult{Metrics: metrics}

	// 构建拦截器链
	// 执行顺序：Recovery(最外层) → Metadata → RateLimit → Metrics → ErrorNormalize → Logging(最内层)
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

	// 顺序：Recovery -> Metadata -> RateLimit -> Metrics -> ErrorNormalize -> Logging。
	unaryInters := []grpc.UnaryServerInterceptor{
		RecoveryUnaryInterceptor(),
		MetadataUnaryInterceptor(),
		RateLimitUnaryInterceptor(rateLimitCfg),
		metrics.UnaryInterceptor(),
		ErrorNormalizeUnaryInterceptor(),
		LoggingUnaryInterceptor(loggingCfg),
	}
	unaryInters = append(unaryInters, opts.ExtraUnaryInterceptors...)

	// 创建 gRPC Server 选项
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
	// 创建健康检查服务

	var healthServer healthgrpc.HealthServer
	// 注册健康检查服务
	if opts.EnableHealth {
		healthServer = newHealthServer()
		healthgrpc.RegisterHealthServer(s, healthServer)
	}

	// 注册服务
	register(s, healthServer)
	// 注册反射服务
	if opts.EnableReflection {
		reflection.Register(s)
	}

	// 创建监听器
	lis, err := net.Listen("tcp", opts.Address)
	if err != nil {
		return result, err
	}
	// 启动优雅停机监听
	go gracefulStop(ctx, s)
	logger.Info(ctx, "gRPC 服务启动", logger.String("addr", opts.Address))
	if err := s.Serve(lis); err != nil {
		return result, err
	}
	return result, nil
}

// gracefulStop 监听 SIGINT/SIGTERM 或 ctx 取消，执行优雅停机。
func gracefulStop(ctx context.Context, s *grpc.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Warn(ctx, "收到停止信号，开始优雅停机", logger.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Warn(ctx, "上下文已取消，开始优雅停机", logger.Any("err", ctx.Err()))
	}

	stopDone := make(chan struct{})
	go func() {
		s.GracefulStop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		logger.Info(ctx, "gRPC 服务已优雅停止")
	case <-time.After(10 * time.Second):
		logger.Warn(ctx, "优雅停机超时，执行强制停止")
		s.Stop()
	}
}

// newHealthServer 创建健康检查服务，初始状态为 SERVING。
func newHealthServer() healthgrpc.HealthServer {
	hs := health.NewServer()
	hs.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	return hs
}
