package grpcx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestOrderClientUnaryInterceptors(t *testing.T) {
	var calls []string
	set := clientUnaryInterceptorSet{
		metadata:       recordClientInterceptor("metadata", &calls),
		internalCaller: recordClientInterceptor("internal-caller", &calls),
		timeout:        recordClientInterceptor("timeout", &calls),
		logging:        recordClientInterceptor("logging", &calls),
		observers: []grpc.UnaryClientInterceptor{
			recordClientInterceptor("observer-1", &calls),
			recordClientInterceptor("observer-2", &calls),
		},
		extra: []grpc.UnaryClientInterceptor{
			recordClientInterceptor("extra-1", &calls),
			recordClientInterceptor("extra-2", &calls),
		},
	}

	interceptors := orderClientUnaryInterceptors(set)
	err := invokeClientInterceptorChain(interceptors, func(context.Context) {
		calls = append(calls, "invoker")
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"metadata",
		"internal-caller",
		"timeout",
		"logging",
		"observer-1",
		"observer-2",
		"extra-1",
		"extra-2",
		"invoker",
	}, calls)
}

func recordClientInterceptor(label string, calls *[]string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		*calls = append(*calls, label)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func invokeClientInterceptorChain(
	interceptors []grpc.UnaryClientInterceptor,
	onInvoke func(context.Context),
) error {
	invoker := func(ctx context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		onInvoke(ctx)
		return nil
	}
	for index := len(interceptors) - 1; index >= 0; index-- {
		interceptor := interceptors[index]
		next := invoker
		invoker = func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return interceptor(ctx, method, req, reply, cc, next, opts...)
		}
	}
	return invoker(context.Background(), "/test.Service/Call", nil, nil, nil)
}
