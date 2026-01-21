package middleware

import (
	"context"
	"time"

	"ChatServer/pkg/logger"

	"google.golang.org/grpc"
)

// GRPCLoggerInterceptor 创建一个 gRPC 客户端一元拦截器，用于记录请求日志
func GRPCLoggerInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		
		// 记录请求开始
		logger.Info(ctx, "gRPC请求开始",
			logger.String("method", method),
			logger.String("service", cc.Target()),
		)

		// 执行 RPC 调用
		err := invoker(ctx, method, req, reply, cc, opts...)

		// 计算耗时
		duration := time.Since(start)

		// 记录请求结果
		if err != nil {
			logger.Error(ctx, "gRPC请求失败",
				logger.String("method", method),
				logger.String("service", cc.Target()),
				logger.Duration("duration", duration),
				logger.ErrorField("error", err),
			)
		} else {
			// 只记录慢请求（>1s），正常请求不记录以避免日志过多
			if duration > 1*time.Second {
				logger.Warn(ctx, "gRPC慢请求",
					logger.String("method", method),
					logger.String("service", cc.Target()),
					logger.Duration("duration", duration),
				)
			}
		}

		return err
	}
}