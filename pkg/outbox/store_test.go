package outbox

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newOutboxStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IdempotentRecord{}))
	return db
}

func TestMarkIdempotentTreatsDuplicateAsSuccess(t *testing.T) {
	db := newOutboxStoreTestDB(t)
	eventType := "user_created:user-service"
	eventID := "evt-duplicate"

	require.NoError(t, MarkIdempotent(db, eventType, eventID))
	require.NoError(t, MarkIdempotent(db, eventType, eventID))

	processed, err := CheckIdempotent(db, eventType, eventID)
	require.NoError(t, err)
	require.True(t, processed)

	var count int64
	require.NoError(t, db.Model(&IdempotentRecord{}).
		Where("event_type = ? AND event_id = ?", eventType, eventID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}
