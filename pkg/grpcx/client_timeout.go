package grpcx

import (
	"context"
	"google.golang.org/grpc"
	"time"
)

// ClientTimeoutConfig 定义 gRPC 客户端按方法收紧 deadline 的策略。
// 这里先以内存 map 维护推荐值，后续可平滑演进到配置中心下发。
type ClientTimeoutConfig struct {
	// MethodTimeouts 按 full method 指定超时，例如 /user.AuthService/Login。
	MethodTimeouts map[string]time.Duration
}

// ClientTimeoutUnaryInterceptor 为上游调用方补齐下游方法级 deadline。
// 规则：实际生效 deadline = min(父 deadline, 方法推荐 timeout)。
func ClientTimeoutUnaryInterceptor(cfg ClientTimeoutConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timeout := resolveClientTimeout(method, cfg)
		if timeout <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		callCtx, cancel, _ := withEarlierDeadline(ctx, timeout)
		defer cancel()
		return invoker(callCtx, method, req, reply, cc, opts...)
	}
}

func resolveClientTimeout(fullMethod string, cfg ClientTimeoutConfig) time.Duration {
	if cfg.MethodTimeouts == nil {
		return 0
	}
	return cfg.MethodTimeouts[fullMethod]
}
