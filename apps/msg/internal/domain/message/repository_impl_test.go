package message

import (
	"context"
	"fmt"
	"testing"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMessageRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Message{}))
	return db
}

func newMessageRepoTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestRepositoryUpdateStatusWithOutbox_OutboxFailureRollsBackMessage(t *testing.T) {
	db := newMessageRepoTestDB(t)

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
	err := repo.UpdateStatusWithOutbox(context.Background(), "conv-1", "msg-1", 1, `{"text":"recalled"}`, OutboxEvent{
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

func TestRepositoryGetBySeqRangeBackwardAnchorZeroStartsFromLatest(t *testing.T) {
	db := newMessageRepoTestDB(t)

	now := time.Now()
	for seq := int64(1); seq <= 5; seq++ {
		require.NoError(t, db.Create(&model.Message{
			ConvId:      "conv-1",
			Seq:         seq,
			MsgId:       fmt.Sprintf("msg-%d", seq),
			ClientMsgId: fmt.Sprintf("client-%d", seq),
			FromUuid:    "user-a",
			DeviceId:    "dev-1",
			MsgType:     int16(model.MsgTypeText),
			Content:     `{"text":"hello"}`,
			Status:      0,
			SendTime:    now.Add(time.Duration(seq) * time.Second),
		}).Error)
	}

	repo := &repositoryImpl{db: db}
	msgs, err := repo.GetBySeqRange(context.Background(), "conv-1", 0, DirectionBackward, 3, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, int64(5), msgs[0].Seq)
	assert.Equal(t, int64(4), msgs[1].Seq)
	assert.Equal(t, int64(3), msgs[2].Seq)
}

func TestRepositoryAllocSeqRecoversFromMissingRedisKey(t *testing.T) {
	db := newMessageRepoTestDB(t)
	repo := &repositoryImpl{db: db, redis: newMessageRepoTestRedis(t)}
	ctx := context.Background()
	require.NoError(t, db.Create(newTestMessage("conv-1", 100, "msg-100", "client-100")).Error)

	seq, err := repo.AllocSeq(ctx, "conv-1")
	require.NoError(t, err)
	assert.Equal(t, int64(101), seq)

	stored, err := repo.redis.Get(ctx, rediskey.MsgSeqKey("conv-1")).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(101), stored)
}

func TestRepositoryRepairSeqRaisesStaleRedisKey(t *testing.T) {
	db := newMessageRepoTestDB(t)
	repo := &repositoryImpl{db: db, redis: newMessageRepoTestRedis(t)}
	ctx := context.Background()
	require.NoError(t, db.Create(newTestMessage("conv-1", 100, "msg-100", "client-100")).Error)
	require.NoError(t, repo.redis.Set(ctx, rediskey.MsgSeqKey("conv-1"), 10, 0).Err())

	require.NoError(t, repo.RepairSeq(ctx, "conv-1"))
	seq, err := repo.AllocSeq(ctx, "conv-1")
	require.NoError(t, err)
	assert.Equal(t, int64(101), seq)
}

func TestRepositoryIdempotentLeaseCompleteRequiresMatchingToken(t *testing.T) {
	db := newMessageRepoTestDB(t)
	repo := &repositoryImpl{db: db, redis: newMessageRepoTestRedis(t)}
	ctx := context.Background()
	key := rediskey.MsgIdempotentKey("user-a", "dev-1", "client-1")

	acquired, err := repo.TryAcquireIdempotent(ctx, "user-a", "dev-1", "client-1")
	require.NoError(t, err)
	require.NotNil(t, acquired)
	require.NotEmpty(t, acquired.LeaseToken)

	stored, err := repo.redis.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, idempotentProcessingValue(acquired.LeaseToken), stored)

	msg := &model.Message{MsgId: "msg-1", Seq: 7, ConvId: "conv-1", SendTime: time.Now()}
	require.NoError(t, repo.CompleteIdempotentResult(ctx, "user-a", "dev-1", "client-1", "wrong-token", msg))
	stored, err = repo.redis.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, idempotentProcessingValue(acquired.LeaseToken), stored)

	require.NoError(t, repo.CompleteIdempotentResult(ctx, "user-a", "dev-1", "client-1", acquired.LeaseToken, msg))
	cached, err := repo.TryAcquireIdempotent(ctx, "user-a", "dev-1", "client-1")
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.NotNil(t, cached.CachedMsg)
	assert.Equal(t, "msg-1", cached.CachedMsg.MsgId)
	assert.Equal(t, int64(7), cached.CachedMsg.Seq)
}

func TestRepositoryReleaseIdempotentProcessingRequiresMatchingToken(t *testing.T) {
	db := newMessageRepoTestDB(t)
	repo := &repositoryImpl{db: db, redis: newMessageRepoTestRedis(t)}
	ctx := context.Background()
	key := rediskey.MsgIdempotentKey("user-a", "dev-1", "client-1")

	acquired, err := repo.TryAcquireIdempotent(ctx, "user-a", "dev-1", "client-1")
	require.NoError(t, err)
	require.NotNil(t, acquired)
	require.NotEmpty(t, acquired.LeaseToken)

	require.NoError(t, repo.ReleaseIdempotentProcessing(ctx, "user-a", "dev-1", "client-1", "wrong-token"))
	exists, err := repo.redis.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	require.NoError(t, repo.ReleaseIdempotentProcessing(ctx, "user-a", "dev-1", "client-1", acquired.LeaseToken))
	exists, err = repo.redis.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestRepositoryCreateMapsDuplicateSeq(t *testing.T) {
	db := newMessageRepoTestDB(t)
	repo := &repositoryImpl{db: db}
	require.NoError(t, db.Create(newTestMessage("conv-1", 1, "msg-1", "client-1")).Error)

	err := repo.Create(context.Background(), newTestMessage("conv-1", 1, "msg-2", "client-2"))
	assert.ErrorIs(t, err, ErrDuplicateMessageSeq)
}

func newTestMessage(convID string, seq int64, msgID string, clientMsgID string) *model.Message {
	return &model.Message{
		ConvId:      convID,
		Seq:         seq,
		MsgId:       msgID,
		ClientMsgId: clientMsgID,
		FromUuid:    "user-a",
		DeviceId:    "dev-1",
		MsgType:     int16(model.MsgTypeText),
		Content:     `{"text":"hello"}`,
		Status:      0,
		SendTime:    time.Now(),
	}
}
