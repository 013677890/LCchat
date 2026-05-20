package grpcx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestClientTimeoutUnaryInterceptorUsesMethodTimeout(t *testing.T) {
	interceptor := ClientTimeoutUnaryInterceptor(ClientTimeoutConfig{
		MethodTimeouts: map[string]time.Duration{
			"/user.UserService/SearchUser": 800 * time.Millisecond,
		},
	})

	var deadline time.Time
	err := interceptor(
		context.Background(),
		"/user.UserService/SearchUser",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			var ok bool
			deadline, ok = ctx.Deadline()
			require.True(t, ok)
			return nil
		},
	)

	require.NoError(t, err)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, time.Duration(0))
	assert.LessOrEqual(t, remaining, 800*time.Millisecond)
	assert.Greater(t, remaining, 700*time.Millisecond)
}

func TestClientTimeoutUnaryInterceptorKeepsShorterParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	interceptor := ClientTimeoutUnaryInterceptor(ClientTimeoutConfig{
		MethodTimeouts: map[string]time.Duration{
			"/msg.MsgService/SendMessage": time.Second,
		},
	})

	var deadline time.Time
	err := interceptor(
		parent,
		"/msg.MsgService/SendMessage",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			var ok bool
			deadline, ok = ctx.Deadline()
			require.True(t, ok)
			return nil
		},
	)

	require.NoError(t, err)
	parentDeadline, ok := parent.Deadline()
	require.True(t, ok)
	assert.WithinDuration(t, parentDeadline, deadline, 5*time.Millisecond)
}

func TestDefaultClientMethodTimeoutsReturnsClone(t *testing.T) {
	timeouts := DefaultClientMethodTimeouts()
	require.NotNil(t, timeouts)

	timeouts["/auth.AuthService/Login"] = 42 * time.Second

	assert.Equal(t, 1500*time.Millisecond, defaultClientMethodTimeouts["/auth.AuthService/Login"])
}
