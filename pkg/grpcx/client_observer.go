package grpcx

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// ClientCallResult 描述一次出站 gRPC 调用完成后的观测结果。
// 它只承载通用调用元信息，不绑定具体指标或日志实现，
// 方便业务方把它接到 Prometheus、审计或自定义埋点上。
type ClientCallResult struct {
	FullMethod string
	Service    string
	Method     string
	Target     string
	Cost       time.Duration
	Err        error
}

// ClientCallObserver 用于消费一次出站调用的观测结果。
type ClientCallObserver func(ctx context.Context, result ClientCallResult)

// ObserveUnaryClientInterceptor 为出站 RPC 提供一个轻量、可复用的观测 hook。
//
// 设计目的：
//   - 把“调用完成后做什么”从具体业务 facade 中剥离；
//   - 避免 grpcx 直接依赖某个业务模块的指标实现；
//   - 让 gateway 这类调用方可以在建连阶段统一挂接 metrics / tracing 扩展。
func ObserveUnaryClientInterceptor(observer ClientCallObserver) grpc.UnaryClientInterceptor {
	if observer == nil {
		return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)

		target := ""
		if cc != nil {
			target = cc.Target()
		}

		serviceName, methodName := splitFullMethod(method)
		observer(ctx, ClientCallResult{
			FullMethod: method,
			Service:    serviceName,
			Method:     methodName,
			Target:     target,
			Cost:       time.Since(start),
			Err:        err,
		})
		return err
	}
}

func splitFullMethod(fullMethod string) (serviceName string, methodName string) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(fullMethod, "/"))
	if trimmed == "" {
		return "", ""
	}

	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return trimmed, ""
	}
	return trimmed[:idx], trimmed[idx+1:]
}
