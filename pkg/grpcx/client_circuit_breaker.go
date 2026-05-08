package grpcx

import (
	"context"
	"fmt"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CircuitBreakerUnaryClientInterceptor 为需要熔断保护的调用方提供可选拦截器。
// 它不属于所有服务都必须启用的公共策略，因此通过 ClientOptions.ExtraUnaryInterceptors 注入。
func CircuitBreakerUnaryClientInterceptor(cb *gobreaker.CircuitBreaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if cb == nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		_, err := cb.Execute(func() (interface{}, error) {
			return nil, invoker(ctx, method, req, reply, cc, opts...)
		})
		if err == nil {
			return nil
		}
		if err == gobreaker.ErrOpenState {
			return status.Error(codes.Unavailable, fmt.Sprintf("circuit breaker [%s] is open", cb.Name()))
		}
		return err
	}
}
