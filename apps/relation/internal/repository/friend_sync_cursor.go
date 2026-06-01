package repository

import (
	"fmt"
	"strconv"
	"strings"
)

const friendSyncCursorPrefix = "v1"

// FriendSyncCursor is the stable pagination anchor for friend incremental sync.
//
// UpdatedAtUnixMilli matches the DATETIME(3) precision used by user_relations,
// and LastID reproduces the repository's updated_at ASC, id ASC tie-break.
type FriendSyncCursor struct {
	UpdatedAtUnixMilli int64
	LastID             int64
	Exact              bool
}

func FriendSyncCursorFromVersion(version int64) FriendSyncCursor {
	if version <= 0 {
		return FriendSyncCursor{}
	}
	return FriendSyncCursor{UpdatedAtUnixMilli: version}
}

func DecodeFriendSyncCursor(raw string) (FriendSyncCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return FriendSyncCursor{}, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != friendSyncCursorPrefix {
		return FriendSyncCursor{}, fmt.Errorf("invalid friend sync cursor")
	}
	updatedAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || updatedAt < 0 {
		return FriendSyncCursor{}, fmt.Errorf("invalid friend sync cursor")
	}
	lastID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || lastID < 0 {
		return FriendSyncCursor{}, fmt.Errorf("invalid friend sync cursor")
	}
	return FriendSyncCursor{
		UpdatedAtUnixMilli: updatedAt,
		LastID:             lastID,
		Exact:              true,
	}, nil
}

func EncodeFriendSyncCursor(cursor FriendSyncCursor) string {
	if cursor.UpdatedAtUnixMilli <= 0 {
		return ""
	}
	lastID := cursor.LastID
	if lastID < 0 {
		lastID = 0
	}
	return fmt.Sprintf("%s:%d:%d", friendSyncCursorPrefix, cursor.UpdatedAtUnixMilli, lastID)
}
