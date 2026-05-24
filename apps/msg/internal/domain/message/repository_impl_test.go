package message

import (
	"context"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRepositoryUpdateStatusWithOutbox_OutboxFailureRollsBackMessage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Message{}))

	original := model.Message{
		ConvId:      "conv-1",
		Seq:         1,
		MsgId:       "msg-1",
		ClientMsgId: "client-1",
		FromUuid:    "user-a",
		DeviceId:    "dev-1",
		MsgType:     int16(model.MsgTypeText),
		Content:     `{"text":"hello"}`,
		Status:      0,
		SendTime:    time.Now(),
	}
	require.NoError(t, db.Create(&original).Error)

	repo := &repositoryImpl{db: db}
	err = repo.UpdateStatusWithOutbox(context.Background(), "conv-1", "msg-1", 1, `{"text":"recalled"}`, OutboxEvent{
		EventType: msgevent.EventTypeMsgPush,
		EntityID:  "conv-1",
		Payload:   `{"event_id":"evt-1"}`,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outbox insert failed")

	var got model.Message
	require.NoError(t, db.Where("conv_id = ? AND msg_id = ?", "conv-1", "msg-1").First(&got).Error)
	assert.Equal(t, int8(0), got.Status)
	assert.Equal(t, original.Content, got.Content)
}
