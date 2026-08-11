package grpcx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestOrderServerUnaryInterceptors(t *testing.T) {
	var calls []string
	set := serverUnaryInterceptorSet{
		recovery:       recordServerInterceptor("recovery", &calls),
		metadata:       recordServerInterceptor("metadata", &calls),
		timeout:        recordServerInterceptor("timeout", &calls),
		validate:       recordServerInterceptor("validate", &calls),
		rateLimit:      recordServerInterceptor("rate-limit", &calls),
		metrics:        recordServerInterceptor("metrics", &calls),
		errorNormalize: recordServerInterceptor("error-normalize", &calls),
		logging:        recordServerInterceptor("logging", &calls),
	}
	extra := []grpc.UnaryServerInterceptor{
		recordServerInterceptor("extra-1", &calls),
		recordServerInterceptor("extra-2", &calls),
	}

	interceptors := orderServerUnaryInterceptors(set, extra)
	_, err := invokeServerInterceptorChain(interceptors, func(context.Context) {
		calls = append(calls, "handler")
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"recovery",
		"metadata",
		"timeout",
		"validate",
		"rate-limit",
		"metrics",
		"error-normalize",
		"logging",
		"extra-1",
		"extra-2",
		"handler",
	}, calls)
}

func recordServerInterceptor(label string, calls *[]string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		*calls = append(*calls, label)
		return handler(ctx, req)
	}
}

func invokeServerInterceptorChain(
	interceptors []grpc.UnaryServerInterceptor,
	onHandle func(context.Context),
) (interface{}, error) {
	handler := func(ctx context.Context, _ interface{}) (interface{}, error) {
		onHandle(ctx)
		return nil, nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Call"}
	for index := len(interceptors) - 1; index >= 0; index-- {
		interceptor := interceptors[index]
		next := handler
		handler = func(ctx context.Context, req interface{}) (interface{}, error) {
			return interceptor(ctx, req, info, next)
		}
	}
	return handler(context.Background(), nil)
}
