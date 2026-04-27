package grpcx

import (
	"context"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"google.golang.org/grpc"
)

type validator interface {
	Validate() error
}

// ValidateUnaryInterceptor 在 handler 执行前调用 protoc-gen-validate 生成的
// Validate() 方法，校验失败立即返回 CodeParamError，不进入业务逻辑。
func ValidateUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if v, ok := req.(validator); ok {
			if err := v.Validate(); err != nil {
				return nil, apperr.Wrap(err, consts.CodeParamError, err.Error())
			}
		}
		return handler(ctx, req)
	}
}
