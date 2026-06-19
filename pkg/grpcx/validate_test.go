package grpcx

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type validReq struct{}

func (v *validReq) Validate() error { return nil }

type invalidReq struct{}

func (v *invalidReq) Validate() error {
	return errors.New("from_uuid: value length must be at least 1")
}

type noValidateReq struct{}

func TestValidateInterceptor_PassesValid(t *testing.T) {
	interceptor := ValidateUnaryInterceptor()
	resp, err := interceptor(
		context.Background(),
		&validReq{},
		&grpc.UnaryServerInfo{},
		func(_ context.Context, req interface{}) (interface{}, error) {
			return "ok", nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestValidateInterceptor_RejectsInvalid(t *testing.T) {
	interceptor := ValidateUnaryInterceptor()
	_, err := interceptor(
		context.Background(),
		&invalidReq{},
		&grpc.UnaryServerInfo{},
		func(_ context.Context, req interface{}) (interface{}, error) {
			t.Fatal("handler should not be called")
			return nil, nil
		},
	)
	require.Error(t, err)
	appErr := apperr.FromStatus(err)
	require.NotNil(t, appErr)
	assert.Equal(t, consts.CodeParamError, apperr.Code(appErr))
}

// TestValidateInterceptor_FrameworkMapsToInvalidArgument 复现并守护审查发现 #3：
// Validate 拦截器位于 ErrorNormalize 之外，其返回的裸 *apperr.Error 会直达 gRPC 框架。
// 自 apperr 实现 GRPCStatus() 后，框架的 status.FromError 应映射为 InvalidArgument，
// 而非旧行为里的 codes.Unknown（后者会触发客户端重试并被熔断器误判为基础设施失败）。
func TestValidateInterceptor_FrameworkMapsToInvalidArgument(t *testing.T) {
	interceptor := ValidateUnaryInterceptor()
	_, err := interceptor(
		context.Background(),
		&invalidReq{},
		&grpc.UnaryServerInfo{},
		func(_ context.Context, req interface{}) (interface{}, error) {
			return nil, nil
		},
	)
	require.Error(t, err)

	// 模拟 gRPC 框架把 handler/拦截器错误转成传输层 status。
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.NotEqual(t, codes.Unknown, st.Code())
	// 脱敏：不应把 PGV 校验原文（含字段名）透传给客户端。
	assert.NotContains(t, st.Message(), "from_uuid")
}

func TestValidateInterceptor_SkipsNoValidator(t *testing.T) {
	interceptor := ValidateUnaryInterceptor()
	resp, err := interceptor(
		context.Background(),
		&noValidateReq{},
		&grpc.UnaryServerInfo{},
		func(_ context.Context, req interface{}) (interface{}, error) {
			return "passthrough", nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "passthrough", resp)
}
