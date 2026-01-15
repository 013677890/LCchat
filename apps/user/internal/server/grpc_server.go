package server

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ChatServer/pkg/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Options 定义 gRPC Server 的常用启动参数。
type Options struct {
	Address            string                         // 监听地址，例 :9090
	UnaryInterceptors  []grpc.UnaryServerInterceptor  // 自定义 Unary 拦截器
	StreamInterceptors []grpc.StreamServerInterceptor // 自定义 Stream 拦截器
	MaxRecvMsgSize     int                            // 最大接收包，默认不限制
	MaxSendMsgSize     int                            // 最大发送包，默认不限制
	EnableHealth       bool                           // 是否注册健康检查
	EnableReflection   bool                           // 是否开启反射（建议仅在开发环境）
}

// Start 启动 gRPC Server，负责创建监听、注册服务、处理优雅停机。
// register 由业务方传入，在此回调中完成各服务的 Register。
func Start(ctx context.Context, opts Options, register func(s *grpc.Server, health healthgrpc.HealthServer)) error {
	if opts.Address == "" {
		return status.Error(codes.InvalidArgument, "grpc address is empty")
	}

	grpcOpts := buildServerOptions(opts)
	s := grpc.NewServer(grpcOpts...)

	// 健康检查
	var healthServer healthgrpc.HealthServer
	if opts.EnableHealth {
		healthServer = NewHealthServer()
		healthgrpc.RegisterHealthServer(s, healthServer)
	}

	// 业务注册
	register(s, healthServer)

	// 反射（仅建议开发/测试开启）
	if opts.EnableReflection {
		reflection.Register(s)
	}

	lis, err := net.Listen("tcp", opts.Address)
	if err != nil {
		return err
	}

	// 优雅停机：捕获系统信号或 ctx 取消
	go gracefulStop(ctx, s)

	logger.Info(ctx, "gRPC server start", logger.String("addr", opts.Address))
	if err := s.Serve(lis); err != nil {
		return err
	}
	return nil
}

// buildServerOptions 构建 grpc.ServerOption。
func buildServerOptions(opts Options) []grpc.ServerOption {
	var serverOpts []grpc.ServerOption

	// 消息大小限制
	if opts.MaxRecvMsgSize > 0 {
		serverOpts = append(serverOpts, grpc.MaxRecvMsgSize(opts.MaxRecvMsgSize))
	}
	if opts.MaxSendMsgSize > 0 {
		serverOpts = append(serverOpts, grpc.MaxSendMsgSize(opts.MaxSendMsgSize))
	}

	// 默认拦截器（Recovery + Logging）
	unaryInters := []grpc.UnaryServerInterceptor{
		recoveryUnaryInterceptor(),
		loggingUnaryInterceptor(),
	}
	unaryInters = append(unaryInters, opts.UnaryInterceptors...)
	serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(unaryInters...))

	if len(opts.StreamInterceptors) > 0 {
		serverOpts = append(serverOpts, grpc.ChainStreamInterceptor(opts.StreamInterceptors...))
	}

	return serverOpts
}

// gracefulStop 监听信号或 ctx 取消，执行优雅停机。
func gracefulStop(ctx context.Context, s *grpc.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Warn(ctx, "received signal, graceful stop", logger.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Warn(ctx, "context canceled, graceful stop", logger.Any("err", ctx.Err()))
	}

	// 给正在处理的请求留出时间（GracefulStop 会等待中断）
	stopDone := make(chan struct{})
	go func() {
		s.GracefulStop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		logger.Info(ctx, "grpc server stopped gracefully")
	case <-time.After(10 * time.Second):
		logger.Warn(ctx, "graceful stop timeout, force stop")
		s.Stop()
	}
}

// recoveryUnaryInterceptor 捕获 panic，避免进程崩溃。
func recoveryUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(ctx, "panic recovered in grpc handler",
					logger.Any("panic", r),
					logger.String("method", info.FullMethod),
				)
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// loggingUnaryInterceptor 记录基础日志（方法、耗时、错误码、trace_id）。
func loggingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		start := time.Now()
		resp, err = handler(ctx, req)
		code := status.Code(err)

		traceID := ""
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("trace_id"); len(vals) > 0 {
				traceID = vals[0]
			}
		}

		fields := []interface{}{
			logger.String("method", info.FullMethod),
			logger.Duration("cost", time.Since(start)),
			logger.String("code", code.String()),
		}
		if traceID != "" {
			fields = append(fields, logger.String("trace_id", traceID))
		}
		if err != nil {
			fields = append(fields, logger.ErrorField("error", err))
			logger.Warn(ctx, "grpc unary request")
		} else {
			logger.Info(ctx, "grpc unary request")
		}

		return resp, err
	}
}

// NewHealthServer 创建健康检查服务，初始状态为 SERVING。
// 业务可在注册服务后自行设置状态。
func NewHealthServer() healthgrpc.HealthServer {
	hs := health.NewServer()
	hs.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	return hs
}
