package grpcx

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func TestErrorNormalizeUnaryInterceptorSanitizesUnhandledError(t *testing.T) {
	logger.ReplaceGlobal(zap.NewNop())
	interceptor := ErrorNormalizeUnaryInterceptor()

	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/user.Auth/Login"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, apperr.Wrap(errors.New("db timeout"), consts.CodeInternalError, "数据库异常")
		},
	)

	require.Error(t, err)
	clean := apperr.FromStatus(err)
	require.NotNil(t, clean)
	assert.Equal(t, consts.CodeInternalError, apperr.Code(clean))
	assert.Equal(t, consts.GetMessage(consts.CodeInternalError), apperr.Message(clean))
	assert.False(t, apperr.HasStack(clean))
	assert.False(t, apperr.IsLogged(clean))
}
