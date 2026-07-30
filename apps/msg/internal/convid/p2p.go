package convid

import (
	"fmt"
	"strings"
)

const p2pPrefix = "p2p-"

// PeerUUID returns the other participant from the current P2P conversation ID.
// The accepted format is exactly "p2p-{participantA}-{participantB}", and
// selfUUID must be one of those two participants.
func PeerUUID(conversationID, selfUUID string) (string, error) {
	if !strings.HasPrefix(conversationID, p2pPrefix) {
		return "", fmt.Errorf("P2P conversation ID %q must start with %q", conversationID, p2pPrefix)
	}

	first, second, found := strings.Cut(strings.TrimPrefix(conversationID, p2pPrefix), "-")
	if !found || first == "" || second == "" || strings.Contains(second, "-") {
		return "", fmt.Errorf("P2P conversation ID %q must contain exactly two participants", conversationID)
	}

	switch selfUUID {
	case first:
		return second, nil
	case second:
		return first, nil
	default:
		return "", fmt.Errorf("user %q is not a participant of P2P conversation %q", selfUUID, conversationID)
	}
}
