package grpcx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestObserveUnaryClientInterceptorCollectsCallResult(t *testing.T) {
	var observed ClientCallResult
	interceptor := ObserveUnaryClientInterceptor(func(_ context.Context, result ClientCallResult) {
		observed = result
	})

	conn, err := grpc.NewClient("passthrough:///observer-target", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	boom := errors.New("boom")
	err = interceptor(
		context.Background(),
		"/user.UserService/GetProfile",
		nil,
		nil,
		conn,
		func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			time.Sleep(10 * time.Millisecond)
			return boom
		},
	)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, "/user.UserService/GetProfile", observed.FullMethod)
	assert.Equal(t, "user.UserService", observed.Service)
	assert.Equal(t, "GetProfile", observed.Method)
	assert.Equal(t, "passthrough:///observer-target", observed.Target)
	assert.ErrorIs(t, observed.Err, boom)
	assert.GreaterOrEqual(t, observed.Cost, 10*time.Millisecond)
}

func TestObserveUnaryClientInterceptorSkipsNilObserver(t *testing.T) {
	interceptor := ObserveUnaryClientInterceptor(nil)

	err := interceptor(
		context.Background(),
		"/user.UserService/GetProfile",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		},
	)

	require.NoError(t, err)
}
