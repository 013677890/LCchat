package grpcx

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// InternalCallerClientConfig 定义客户端何时注入 x-internal-caller。
//
// 约定：
//   - Caller 为空时不注入；
//   - Methods 为空时，对该连接上的所有 RPC 注入；
//   - Methods 非空时，仅对列出的 full method 注入。
//
// 这样既能覆盖“整条连接都只做内部调用”的场景，
// 也能覆盖 gateway 这种“同一连接同时承载公开接口 + 内部接口”的场景。
type InternalCallerClientConfig struct {
	Caller  string
	Methods []string
}

// InternalCallerUnaryClientInterceptor 按配置为出站 RPC 注入 x-internal-caller metadata。
func InternalCallerUnaryClientInterceptor(cfg InternalCallerClientConfig) grpc.UnaryClientInterceptor {
	caller := strings.TrimSpace(cfg.Caller)
	methodSet := make(map[string]struct{}, len(cfg.Methods))
	for _, method := range cfg.Methods {
		method = strings.TrimSpace(method)
		if method == "" {
			continue
		}
		methodSet[method] = struct{}{}
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if caller == "" {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		if len(methodSet) > 0 {
			if _, ok := methodSet[method]; !ok {
				return invoker(ctx, method, req, reply, cc, opts...)
			}
		}

		ctx = metadata.AppendToOutgoingContext(ctx, MetadataInternalCaller, caller)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// WithInternalCaller 返回一个 gRPC 客户端一元拦截器，
// 在每次出站 RPC 调用中自动注入 x-internal-caller metadata。
//
// 使用示例（在 providers.go 中创建连接时注册）:
//
//	conn, _ := grpc.Dial(addr,
//	    grpc.WithUnaryInterceptor(grpcx.WithInternalCaller("gateway")),
//	)
func WithInternalCaller(serviceName string) grpc.UnaryClientInterceptor {
	return InternalCallerUnaryClientInterceptor(InternalCallerClientConfig{Caller: serviceName})
}
