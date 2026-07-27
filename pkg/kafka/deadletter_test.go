package kafka

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermanentErrorPreservesCauseAndIsIdempotent(t *testing.T) {
	cause := errors.New("schema mismatch")
	permanent := Permanent(cause)
	require.Error(t, permanent)
	assert.True(t, IsPermanent(permanent))
	assert.ErrorIs(t, permanent, cause)
	assert.Same(t, permanent, Permanent(permanent))
	assert.Nil(t, Permanent(nil))
}
