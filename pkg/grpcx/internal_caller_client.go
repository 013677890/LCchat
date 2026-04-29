package grpcx

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// WithInternalCaller 返回一个 gRPC 客户端一元拦截器，
// 在每次出站 RPC 调用中自动注入 x-internal-caller metadata。
//
// 使用示例（在 providers.go 中创建连接时注册）:
//
//	conn, _ := grpc.Dial(addr,
//	    grpc.WithUnaryInterceptor(grpcx.WithInternalCaller("gateway")),
//	)
func WithInternalCaller(serviceName string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, MetadataInternalCaller, serviceName)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
