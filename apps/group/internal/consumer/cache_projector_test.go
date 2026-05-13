package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeProjectorRepoForConsumer 用于验证 consumer 的分支行为。
type fakeProjectorRepoForConsumer struct {
	applyFn     func(context.Context, groupevent.GroupCacheEventPayload) error
	applyCalls  int
	lastPayload groupevent.GroupCacheEventPayload
}

// ApplyGroupCacheEvent 实现 IGroupCacheProjectorRepository。
func (f *fakeProjectorRepoForConsumer) ApplyGroupCacheEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error {
	f.applyCalls++
	f.lastPayload = payload
	if f.applyFn == nil {
		return nil
	}
	return f.applyFn(ctx, payload)
}

// newConsumerTestDB 创建仅用于幂等记录测试的 SQLite 内存库。
func newConsumerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:group_cache_projector_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&outbox.IdempotentRecord{}))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db
}

// buildConsumerTestMessage 构造一条可被成功解析的 group.cache 事件。
func buildConsumerTestMessage(t *testing.T, eventID string) []byte {
	t.Helper()

	encoded, err := groupevent.Encode(groupevent.GroupCacheEventPayload{
		EventID:   eventID,
		Action:    groupevent.ActionGroupInfoUpdated,
		GroupUUID: "group-1",
		Group: &groupevent.GroupSnapshot{
			GroupUUID:     "group-1",
			Name:          "测试群",
			Status:        0,
			UpdatedAtUnix: 1710000000,
		},
	})
	require.NoError(t, err)
	return []byte(encoded)
}

// hasConsumerIdempotentRecord 判断是否已经写入幂等记录。
func hasConsumerIdempotentRecord(t *testing.T, db *gorm.DB, eventID string) bool {
	t.Helper()

	processed, err := outbox.CheckIdempotent(db, groupCacheProjectorIdempotentEventType, eventID)
	require.NoError(t, err)
	return processed
}

// TestHandleSkipsDecodeError 验证坏消息会被跳过且不触发投影。
func TestHandleSkipsDecodeError(t *testing.T) {
	repo := &fakeProjectorRepoForConsumer{}
	projector := &CacheProjector{projectorRepo: repo, db: newConsumerTestDB(t)}

	err := projector.handle(context.Background(), []byte("not-json"))
	require.NoError(t, err)
	assert.Zero(t, repo.applyCalls)
}

// TestHandleSkipsProcessedEvent 验证已处理事件不会重复投影。
func TestHandleSkipsProcessedEvent(t *testing.T) {
	db := newConsumerTestDB(t)
	require.NoError(t, outbox.MarkIdempotent(db, groupCacheProjectorIdempotentEventType, "evt-processed"))

	repo := &fakeProjectorRepoForConsumer{}
	projector := &CacheProjector{projectorRepo: repo, db: db}

	err := projector.handle(context.Background(), buildConsumerTestMessage(t, "evt-processed"))
	require.NoError(t, err)
	assert.Zero(t, repo.applyCalls)
}

// TestHandleSkipsInvalidPayloadError 验证无效载荷按不可重试消息跳过。
func TestHandleSkipsInvalidPayloadError(t *testing.T) {
	db := newConsumerTestDB(t)
	repo := &fakeProjectorRepoForConsumer{
		applyFn: func(context.Context, groupevent.GroupCacheEventPayload) error {
			return repository.ErrInvalidProjectorPayload
		},
	}
	projector := &CacheProjector{projectorRepo: repo, db: db}

	err := projector.handle(context.Background(), buildConsumerTestMessage(t, "evt-invalid"))
	require.NoError(t, err)
	assert.Equal(t, 1, repo.applyCalls)
	assert.False(t, hasConsumerIdempotentRecord(t, db, "evt-invalid"))
}

// TestHandleReturnsProjectorError 验证 Redis 类可重试错误会向上返回。
func TestHandleReturnsProjectorError(t *testing.T) {
	db := newConsumerTestDB(t)
	repo := &fakeProjectorRepoForConsumer{
		applyFn: func(context.Context, groupevent.GroupCacheEventPayload) error {
			return errors.New("redis down")
		},
	}
	projector := &CacheProjector{projectorRepo: repo, db: db}

	err := projector.handle(context.Background(), buildConsumerTestMessage(t, "evt-retry"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "投影 group.cache 事件失败")
	assert.Equal(t, 1, repo.applyCalls)
	assert.False(t, hasConsumerIdempotentRecord(t, db, "evt-retry"))
}

// TestHandleMarksIdempotentAfterSuccess 验证成功投影后会补写幂等记录。
func TestHandleMarksIdempotentAfterSuccess(t *testing.T) {
	db := newConsumerTestDB(t)
	repo := &fakeProjectorRepoForConsumer{}
	projector := &CacheProjector{projectorRepo: repo, db: db}

	err := projector.handle(context.Background(), buildConsumerTestMessage(t, "evt-success"))
	require.NoError(t, err)
	assert.Equal(t, 1, repo.applyCalls)
	assert.Equal(t, "evt-success", repo.lastPayload.EventID)
	assert.True(t, hasConsumerIdempotentRecord(t, db, "evt-success"))
}
