package utils

import (
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExtractErrorCodeFromAppError(t *testing.T) {
	err := apperr.New(consts.CodeFriendRequestSent)
	assert.Equal(t, consts.CodeFriendRequestSent, ExtractErrorCode(err))
}

func TestExtractErrorCodeFromStatusDetails(t *testing.T) {
	err := apperr.ToStatus(apperr.New(consts.CodeNoPermission))
	assert.Equal(t, consts.CodeNoPermission, ExtractErrorCode(err))
}

func TestExtractErrorCodeRejectsLegacyMessageNumericBizCode(t *testing.T) {
	// 旧协议：message 为纯数字业务码。
	legacy := status.Error(codes.Unknown, "12003")
	assert.Equal(t, consts.CodeInternalError, ExtractErrorCode(legacy))
	assert.NotEqual(t, 12003, ExtractErrorCode(legacy))
}

func TestExtractErrorCodeRejectsLegacyGRPCCodeAsBizCode(t *testing.T) {
	// 历史误用：把业务码塞进 gRPC codes.Code 数值位（>=10000）。
	// codes.Code 是 uint32，这里构造一个 code 值为业务码的 status。
	st := status.New(codes.Code(11001), "legacy")
	assert.Equal(t, consts.CodeInternalError, ExtractErrorCode(st.Err()))
	assert.NotEqual(t, 11001, ExtractErrorCode(st.Err()))
}

func TestExtractErrorCodeMapsTransportWithoutDetails(t *testing.T) {
	assert.Equal(t, consts.CodeServiceUnavailable, ExtractErrorCode(status.Error(codes.Unavailable, "down")))
	assert.Equal(t, consts.CodeTimeoutError, ExtractErrorCode(status.Error(codes.DeadlineExceeded, "slow")))
}
