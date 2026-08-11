package grpcx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func TestTimeoutUnaryInterceptorUsesMethodOverride(t *testing.T) {
	logger.ReplaceGlobal(zap.NewNop())
	interceptor := TimeoutUnaryInterceptor(TimeoutConfig{
		DefaultTimeout: 2 * time.Second,
		MethodTimeouts: map[string]time.Duration{
			"/user.UserService/SearchUser": 2400 * time.Millisecond,
		},
	})

	var remaining time.Duration
	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/user.UserService/SearchUser"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining = time.Until(deadline)
			return "ok", nil
		},
	)

	require.NoError(t, err)
	assert.Greater(t, remaining, time.Duration(0))
	assert.Greater(t, remaining, 2*time.Second)
	assert.LessOrEqual(t, remaining, 2400*time.Millisecond)
}

func TestTimeoutUnaryInterceptorKeepsShorterParentDeadline(t *testing.T) {
	logger.ReplaceGlobal(zap.NewNop())
	parent, parentCancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer parentCancel()

	interceptor := TimeoutUnaryInterceptor(TimeoutConfig{DefaultTimeout: 2 * time.Second})

	var deadline time.Time
	_, err := interceptor(
		parent,
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			var ok bool
			deadline, ok = ctx.Deadline()
			require.True(t, ok)
			return "ok", nil
		},
	)

	require.NoError(t, err)
	parentDeadline, ok := parent.Deadline()
	require.True(t, ok)
	assert.WithinDuration(t, parentDeadline, deadline, 5*time.Millisecond)
}

func TestTimeoutUnaryInterceptorMapsDeadlineExceeded(t *testing.T) {
	logger.ReplaceGlobal(zap.NewNop())
	interceptor := TimeoutUnaryInterceptor(TimeoutConfig{DefaultTimeout: 50 * time.Millisecond})

	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/msg.MsgService/SendMessage"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	require.Error(t, err)
	appErr := apperr.FromStatus(err)
	require.NotNil(t, appErr)
	assert.Equal(t, consts.CodeTimeoutError, apperr.Code(appErr))
	assert.Equal(t, consts.GetMessage(consts.CodeTimeoutError), apperr.Message(appErr))
}

func TestIsDeadlineExceeded(t *testing.T) {
	assert.True(t, isDeadlineExceeded(context.DeadlineExceeded))
	assert.False(t, isDeadlineExceeded(errors.New("other")))
}
