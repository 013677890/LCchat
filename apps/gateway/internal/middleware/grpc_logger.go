package middleware

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// GRPCLoggerInterceptor 创建 gRPC 客户端一元拦截器，输出单条聚合日志。
func GRPCLoggerInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		fields := []zap.Field{
			logger.String("method", method),
			logger.String("service", cc.Target()),
			logger.Duration("cost", duration),
			logger.String("grpc_code", status.Code(err).String()),
		}
		if err != nil {
			appErr := apperr.FromStatus(err)
			fields = append(fields, logger.Int("business_code", apperr.Code(appErr)))
			if apperr.Code(appErr) >= 30000 {
				logger.Error(ctx, "Gateway gRPC 请求错误", fields...)
			} else {
				logger.Info(ctx, "Gateway gRPC 请求成功", fields...)
			}
			return err
		}
		if duration > time.Second {
			logger.Warn(ctx, "Gateway gRPC 慢请求", fields...)
		} else {
			logger.Info(ctx, "Gateway gRPC 请求成功", fields...)
		}
		return nil
	}
}
