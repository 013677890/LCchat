package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFriendSyncCursorEncodeDecode(t *testing.T) {
	cursor := FriendSyncCursor{UpdatedAtUnixMilli: 1710000000123, LastID: 987654321, Exact: true}

	encoded := EncodeFriendSyncCursor(cursor)
	got, err := DecodeFriendSyncCursor(encoded)

	require.NoError(t, err)
	assert.Equal(t, "v1:1710000000123:987654321", encoded)
	assert.Equal(t, cursor, got)
}

func TestDecodeFriendSyncCursorRejectsInvalidInput(t *testing.T) {
	for _, raw := range []string{"v0:1:2", "v1:-1:2", "v1:1:-2", "v1:abc:2", "broken"} {
		_, err := DecodeFriendSyncCursor(raw)
		assert.Error(t, err, raw)
	}
}
