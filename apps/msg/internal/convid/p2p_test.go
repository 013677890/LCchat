package convid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerUUIDReturnsOtherParticipant(t *testing.T) {
	tests := []struct {
		name           string
		conversationID string
		selfUUID       string
		want           string
	}{
		{name: "first participant", conversationID: "p2p-alice-bob", selfUUID: "alice", want: "bob"},
		{name: "second participant", conversationID: "p2p-alice-bob", selfUUID: "bob", want: "alice"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			peerUUID, err := PeerUUID(test.conversationID, test.selfUUID)
			require.NoError(t, err)
			assert.Equal(t, test.want, peerUUID)
		})
	}
}

func TestPeerUUIDRejectsMalformedOrUnrelatedConversation(t *testing.T) {
	tests := []struct {
		name           string
		conversationID string
		selfUUID       string
	}{
		{name: "missing prefix", conversationID: "alice-bob", selfUUID: "alice"},
		{name: "missing separator", conversationID: "p2p-alice", selfUUID: "alice"},
		{name: "empty first participant", conversationID: "p2p--bob", selfUUID: "bob"},
		{name: "empty second participant", conversationID: "p2p-alice-", selfUUID: "alice"},
		{name: "extra separator", conversationID: "p2p-alice-bob-extra", selfUUID: "alice"},
		{name: "self not present", conversationID: "p2p-alice-bob", selfUUID: "carol"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			peerUUID, err := PeerUUID(test.conversationID, test.selfUUID)
			require.Error(t, err)
			assert.Empty(t, peerUUID)
		})
	}
}
