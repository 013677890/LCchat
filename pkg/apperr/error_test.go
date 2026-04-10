package apperr

import (
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
