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
)

type validReq struct{}

func (v *validReq) Validate() error { return nil }

type invalidReq struct{}

func (v *invalidReq) Validate() error { return errors.New("from_uuid: value length must be at least 1") }

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
