package utils

import (
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
)

func TestExtractErrorCodeFromAppError(t *testing.T) {
	err := apperr.New(consts.CodeFriendRequestSent)
	assert.Equal(t, consts.CodeFriendRequestSent, ExtractErrorCode(err))
}

func TestExtractErrorCodeFromStatusDetails(t *testing.T) {
	err := apperr.ToStatus(apperr.New(consts.CodeNoPermission))
	assert.Equal(t, consts.CodeNoPermission, ExtractErrorCode(err))
}
