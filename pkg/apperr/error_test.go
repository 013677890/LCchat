package apperr

import (
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWrapCapturesStackOnlyOnce(t *testing.T) {
	base := errors.New("db down")
	wrapped := Wrap(base, consts.CodeInternalError, "数据库异常")
	require.True(t, HasStack(wrapped))
	assert.Equal(t, "数据库异常: db down", wrapped.Error())

	frames1 := Frames(wrapped)
	require.NotEmpty(t, frames1)

	wrappedAgain := Wrap(wrapped, consts.CodeServiceUnavailable, "服务不可用")
	frames2 := Frames(wrappedAgain)
	require.NotEmpty(t, frames2)
	assert.Equal(t, frames1, frames2)
	assert.Equal(t, consts.CodeServiceUnavailable, Code(wrappedAgain))
	assert.Equal(t, "服务不可用: 数据库异常: db down", wrappedAgain.Error())
}

func TestSanitizeStripsStackAndKeepsCodeMessage(t *testing.T) {
	err := WithStack(Wrap(errors.New("boom"), consts.CodeInternalError, "内部错误"))
	require.True(t, HasStack(err))

	clean := Sanitize(err)
	require.NotNil(t, clean)
	assert.Equal(t, consts.CodeInternalError, Code(clean))
	assert.Equal(t, consts.GetMessage(consts.CodeInternalError), Message(clean))
	assert.Equal(t, consts.GetMessage(consts.CodeInternalError), clean.Error())
	assert.False(t, HasStack(clean))
}

func TestToStatusMapsBusinessErrorsToExpectedGRPCCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
		want codes.Code
	}{
		{name: "二维码格式错误", code: consts.CodeQRCodeFormatError, want: codes.InvalidArgument},
		{name: "二维码已过期", code: consts.CodeQRCodeExpired, want: codes.FailedPrecondition},
		{name: "邮箱格式无效", code: consts.CodeInvalidEmail, want: codes.InvalidArgument},
		{name: "消息类型不支持", code: consts.CodeMessageTypeNotSupport, want: codes.InvalidArgument},
		{name: "消息发送失败", code: consts.CodeMessageSendFail, want: codes.FailedPrecondition},
		{name: "不是群成员", code: consts.CodeNotGroupMember, want: codes.PermissionDenied},
		{name: "设备信息无效", code: consts.CodeDeviceInfoInvalid, want: codes.InvalidArgument},
		{name: "内部错误", code: consts.CodeInternalError, want: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ToStatus(New(tt.code))
			require.Error(t, err)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}
