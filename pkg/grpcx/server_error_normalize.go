package grpcx

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"google.golang.org/grpc"
)

// ErrorNormalizeUnaryInterceptor 把 handler 返回的错误统一转换为脱敏后的 gRPC status。
// 日志由内层 LoggingUnaryInterceptor 记录，这里只负责传输边界，避免原始错误泄漏。
func ErrorNormalizeUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		appErr := apperr.FromStatus(err)
		return resp, apperr.ToStatus(apperr.Sanitize(appErr))
	}
}
