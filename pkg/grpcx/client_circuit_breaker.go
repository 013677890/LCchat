package grpcx

import (
	"context"
	"errors"
	"fmt"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BreakerIsSuccessful 判断一次 gRPC 调用结果是否应被熔断器视为“成功”。
//
// 熔断器的职责是隔离“下游不可用 / 过慢 / 内部故障”这类基础设施问题，而不是对正常的
// 业务拒绝（参数错误、未授权、密码错误、未找到、已存在、限流、前置条件不满足等）做出反应。
// 若把业务错误也计入失败率，错误密码风暴、批量校验失败等正常流量就会拖垮失败率并误触熔断，
// 反而把健康的下游对所有调用方一起熔断（自伤）。
//
// 计为“成功”（不累计失败）的情形：
//   - nil；
//   - 客户端主动取消（context.Canceled / codes.Canceled）：与下游健康无关；
//   - 业务类错误码：InvalidArgument / Unauthenticated / PermissionDenied / NotFound /
//     AlreadyExists / FailedPrecondition / Unimplemented / ResourceExhausted(本项目为业务限流) 等。
//
// 计为“失败”（累计并可能触发熔断）的情形：
//   - Unavailable / DeadlineExceeded / Internal / Unknown / DataLoss；
//   - 非 gRPC status 错误：保守地计为失败。
func BreakerIsSuccessful(err error) bool {
	if err == nil {
		return true
	}

	// 客户端取消（如网关请求超时被取消、连接断开）不反映下游健康，不应计入失败。
	if errors.Is(err, context.Canceled) {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		// 不是 gRPC status 错误，无法归类，保守计为失败。
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Internal,
		codes.Unknown, codes.DataLoss:
		return false
	default:
		// 其余（含 Canceled 与各业务码）视为成功，不触发熔断。
		return true
	}
}

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
