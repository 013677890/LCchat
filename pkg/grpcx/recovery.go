package grpcx

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/grpc"
)

// RecoveryUnaryInterceptor 捕获 handler 内的 panic，避免单个请求的异常崩溃整个进程。
// 捕获后记录 Error 日志（含 method + panic 值），并返回 codes.Internal。
func RecoveryUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				panicErr := apperr.NewFromPanic(r)
				logger.Error(ctx, "gRPC 处理发生 panic",
					logger.Any("panic", r),
					logger.String("method", info.FullMethod),
					logger.String("top_frame", apperr.TopFrame(panicErr)),
					logger.StackFrames("stack", apperr.Frames(panicErr)),
					logger.ErrorField("error", panicErr),
				)
				apperr.MarkLogged(panicErr)
				err = apperr.ToStatus(apperr.Sanitize(panicErr))
			}
		}()
		return handler(ctx, req)
	}
}
