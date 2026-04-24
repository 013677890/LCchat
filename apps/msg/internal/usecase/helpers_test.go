package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPeerUuid_FirstIsself(t *testing.T) {
	peer := extractPeerUuid("p2p-alice-bob", "alice")
	assert.Equal(t, "bob", peer)
}

func TestExtractPeerUuid_SecondIsSelf(t *testing.T) {
	peer := extractPeerUuid("p2p-alice-bob", "bob")
	assert.Equal(t, "alice", peer)
}

func TestExtractPeerUuid_InvalidFormat(t *testing.T) {
	peer := extractPeerUuid("invalid", "x")
	assert.Equal(t, "", peer)
}

func TestExtractPeerUuid_NoPrefix(t *testing.T) {
	peer := extractPeerUuid("group-uuid", "x")
	assert.Equal(t, "group", peer)
}
