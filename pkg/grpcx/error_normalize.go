package grpcx

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"google.golang.org/grpc"
)

// ErrorNormalizeUnaryInterceptor 统一兜底服务端错误日志，并返回干净 gRPC 错误。
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
