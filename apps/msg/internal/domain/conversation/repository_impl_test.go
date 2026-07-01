package conversation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newConversationRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Conversation{}))
	return db
}

func TestRepositoryRepairForMessageReceiverRecomputesUnreadIdempotently(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	now := time.Now()

	require.NoError(t, db.Create(&model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "b",
		TargetUuid:  "a",
		LastMsgId:   "m6",
		LastMsgAt:   &now,
		LastMsgPrev: `{"sender_uuid":"a","preview":"m6"}`,
		MaxSeq:      6,
		ReadSeq:     4,
		UnreadCount: 1, // 模拟 seq=5 的接收方 Upsert 漏写，后来 seq=6 又只 +1。
		Status:      0,
	}).Error)

	repair := &model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "b",
		TargetUuid:  "a",
		LastMsgId:   "m5",
		LastMsgAt:   &now,
		LastMsgPrev: `{"sender_uuid":"a","preview":"m5"}`,
		MaxSeq:      5,
		UnreadCount: 5,
		Status:      0,
	}
	require.NoError(t, repo.RepairForMessage(context.Background(), repair, false))
	require.NoError(t, repo.RepairForMessage(context.Background(), repair, false))

	var got model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "b", "p2p-a-b").First(&got).Error)
	assert.Equal(t, int64(6), got.MaxSeq)
	assert.Equal(t, "m6", got.LastMsgId)
	assert.Equal(t, 2, got.UnreadCount)
	assert.Equal(t, int8(0), got.Status)
}

func TestRepositoryRepairForMessageDoesNotReactivateDeletedConversationForOldMessage(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	now := time.Now()

	require.NoError(t, db.Create(&model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "b",
		TargetUuid:  "a",
		LastMsgId:   "m5",
		LastMsgAt:   &now,
		LastMsgPrev: `{"sender_uuid":"a","preview":"m5"}`,
		MaxSeq:      5,
		ReadSeq:     5,
		ClearSeq:    5,
		UnreadCount: 0,
		Status:      1,
	}).Error)

	repair := &model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "b",
		TargetUuid:  "a",
		LastMsgId:   "m5",
		LastMsgAt:   &now,
		LastMsgPrev: `{"sender_uuid":"a","preview":"m5"}`,
		MaxSeq:      5,
		UnreadCount: 5,
		Status:      0,
	}
	require.NoError(t, repo.RepairForMessage(context.Background(), repair, false))

	var got model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "b", "p2p-a-b").First(&got).Error)
	assert.Equal(t, int8(1), got.Status)
	assert.Equal(t, 0, got.UnreadCount)
}
