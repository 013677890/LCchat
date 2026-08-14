package conversation

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var conversationTestDBSequence atomic.Uint64

func newConversationRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:conversation-repo-%d?mode=memory&cache=shared", conversationTestDBSequence.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Conversation{}, &model.GroupConversation{}, &outbox.Event{}))
	return db
}

func TestConversationSchemaRejectsImplicitGroupMembershipState(t *testing.T) {
	db := newConversationRepoTestDB(t)

	err := db.Create(&model.Conversation{
		ConvId:           "group-invalid",
		Type:             2,
		OwnerUuid:        "user-1",
		TargetUuid:       "group-invalid",
		MembershipStatus: model.ConversationMembershipNotApplicable,
	}).Error
	require.Error(t, err, "GROUP 行的 membership_status=0 必须由表约束拒绝")
}

func TestConversationSchemaRejectsNegativeMembershipVersion(t *testing.T) {
	db := newConversationRepoTestDB(t)

	err := db.Create(&model.Conversation{
		ConvId:            "group-invalid-version",
		Type:              2,
		OwnerUuid:         "user-1",
		TargetUuid:        "group-invalid-version",
		MembershipStatus:  model.ConversationMembershipActive,
		MembershipVersion: -1,
	}).Error
	require.Error(t, err, "负数 membership_version 必须由表约束拒绝")
}

func TestRepositoryUpsertGroupReadCreatesExplicitActiveMembership(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)

	conv, err := repo.UpsertGroupReadSeqWithOutbox(
		context.Background(),
		"user-1",
		"group-1",
		7,
		[]OutboxEvent{{
			EventType: "msg.push",
			EntityID:  "group-1",
			Payload:   `{}`,
		}},
	)
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, model.ConversationMembershipActive, conv.MembershipStatus)
	assert.Equal(t, int64(0), conv.MembershipVersion)
	assert.Equal(t, int64(7), conv.ReadSeq)
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

func TestRepositoryUpdateReadSeqIsMonotonic(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	require.NoError(t, db.Create(&model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "a",
		TargetUuid:  "b",
		MaxSeq:      10,
		ReadSeq:     5,
		UnreadCount: 5,
	}).Error)

	require.NoError(t, repo.UpdateReadSeq(context.Background(), "a", "p2p-a-b", 7))
	require.NoError(t, repo.UpdateReadSeq(context.Background(), "a", "p2p-a-b", 6))

	var got model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "a", "p2p-a-b").First(&got).Error)
	assert.Equal(t, int64(7), got.ReadSeq)
	assert.Equal(t, 3, got.UnreadCount)
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

func TestRepositoryRepairForGroupMessageDoesNotCrossEqualClearSeq(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	now := time.Now()

	require.NoError(t, db.Create(&model.Conversation{
		ConvId:            "group-1",
		Type:              2,
		OwnerUuid:         "sender-1",
		TargetUuid:        "group-1",
		MaxSeq:            0, // 群个人行不随消息写扩散，可能显著落后于共享群 max_seq。
		ReadSeq:           9,
		ClearSeq:          9,
		Status:            1,
		MembershipStatus:  model.ConversationMembershipActive,
		MembershipVersion: 1,
	}).Error)

	repair := &model.Conversation{
		ConvId:            "group-1",
		Type:              2,
		OwnerUuid:         "sender-1",
		TargetUuid:        "group-1",
		LastMsgId:         "msg-9",
		LastMsgAt:         &now,
		LastMsgPrev:       `{"sender_uuid":"sender-1","preview":"m9"}`,
		MaxSeq:            9,
		ReadSeq:           9,
		Status:            0,
		MembershipStatus:  model.ConversationMembershipActive,
		MembershipVersion: 0,
	}
	require.NoError(t, repo.RepairForMessage(context.Background(), repair, true))

	var got model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "sender-1", "group-1").First(&got).Error)
	assert.Equal(t, int64(9), got.MaxSeq)
	assert.Equal(t, int64(9), got.ClearSeq)
	assert.Equal(t, int8(1), got.Status, "seq 等于 clear_seq 的幂等重试不能恢复已删除会话")
}

func TestRepositoryListGroupFiltersMembershipAndSharedGroupStatus(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	now := time.Now().Truncate(time.Millisecond)

	for _, conv := range []*model.Conversation{
		{
			ConvId:            "group-active",
			Type:              2,
			OwnerUuid:         "user-1",
			TargetUuid:        "group-active",
			MembershipStatus:  model.ConversationMembershipActive,
			MembershipVersion: 1,
			Status:            0,
		},
		{
			ConvId:            "group-left",
			Type:              2,
			OwnerUuid:         "user-1",
			TargetUuid:        "group-left",
			MembershipStatus:  model.ConversationMembershipLeft,
			MembershipVersion: 2,
			Status:            0,
		},
		{
			ConvId:            "group-dismissed",
			Type:              2,
			OwnerUuid:         "user-1",
			TargetUuid:        "group-dismissed",
			MembershipStatus:  model.ConversationMembershipActive,
			MembershipVersion: 1,
			Status:            0,
		},
	} {
		require.NoError(t, db.Create(conv).Error)
	}
	for _, group := range []*model.GroupConversation{
		{
			GroupUuid:   "group-active",
			MaxSeq:      8,
			LastMsgAt:   &now,
			GroupStatus: model.GroupConversationStatusNormal,
		},
		{
			GroupUuid:   "group-left",
			MaxSeq:      8,
			LastMsgAt:   &now,
			GroupStatus: model.GroupConversationStatusNormal,
		},
		{
			GroupUuid:   "group-dismissed",
			MaxSeq:      8,
			LastMsgAt:   &now,
			GroupStatus: model.GroupConversationStatusDismissed,
		},
	} {
		require.NoError(t, db.Create(group).Error)
	}

	got, err := repo.ListGroup(context.Background(), "user-1", 0, 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "group-active", got[0].ConvId)
	assert.Equal(t, int64(8), got[0].MaxSeq)
}

func TestRepositoryListGroupIncrementalObservesSharedGroupActivity(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	oldTime := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	newTime := time.Now().Truncate(time.Millisecond)

	require.NoError(t, db.Create(&model.Conversation{
		ConvId:            "group-1",
		Type:              2,
		OwnerUuid:         "user-1",
		TargetUuid:        "group-1",
		MembershipStatus:  model.ConversationMembershipActive,
		MembershipVersion: 1,
		Status:            0,
		CreatedAt:         oldTime,
		UpdatedAt:         oldTime,
	}).Error)
	require.NoError(t, db.Create(&model.GroupConversation{
		GroupUuid:   "group-1",
		MaxSeq:      9,
		LastMsgAt:   &newTime,
		GroupStatus: model.GroupConversationStatusNormal,
		UpdatedAt:   newTime,
	}).Error)

	since := oldTime.Add(time.Hour).UnixMilli()
	got, err := repo.ListGroup(context.Background(), "user-1", since, 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "group-1", got[0].ConvId)
	assert.Equal(t, newTime.UnixMilli(), got[0].UpdatedAt.UnixMilli())
}

func TestRepositoryUpsertAdvancesLastMsgOnlyWithHigherSeq(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	initialUpdatedAt := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	seq2Time := initialUpdatedAt.Add(time.Minute)
	require.NoError(t, db.Create(&model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "a",
		TargetUuid:  "b",
		LastMsgId:   "msg-2",
		LastMsgPrev: "preview-2",
		LastMsgAt:   &seq2Time,
		MaxSeq:      2,
		ReadSeq:     2,
		UnreadCount: 0,
		Status:      0,
		UpdatedAt:   initialUpdatedAt,
	}).Error)

	// 模拟 seq=1 的发送 workflow 比 seq=2 更晚执行到会话投影。
	seq1Time := seq2Time.Add(-time.Minute)
	require.NoError(t, repo.Upsert(context.Background(), &model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "a",
		TargetUuid:  "b",
		LastMsgId:   "msg-1",
		LastMsgPrev: "preview-1",
		LastMsgAt:   &seq1Time,
		MaxSeq:      1,
		ReadSeq:     1,
		Status:      0,
	}, true))

	var stored model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "a", "p2p-a-b").First(&stored).Error)
	assert.Equal(t, int64(2), stored.MaxSeq)
	assert.Equal(t, "msg-2", stored.LastMsgId)
	assert.Equal(t, "preview-2", stored.LastMsgPrev)
	require.NotNil(t, stored.LastMsgAt)
	assert.Equal(t, seq2Time.UnixMilli(), stored.LastMsgAt.UnixMilli())
	assert.Equal(t, int64(2), stored.ReadSeq)
	assert.Equal(t, initialUpdatedAt.UnixMilli(), stored.UpdatedAt.UnixMilli())

	// 接收方迟到的旧消息仍应 +1 未读，但不能把预览写回旧 seq。
	require.NoError(t, db.Model(&model.Conversation{}).
		Where("owner_uuid = ? AND conv_id = ?", "a", "p2p-a-b").
		Updates(map[string]interface{}{"unread_count": 0, "read_seq": 2}).Error)
	require.NoError(t, repo.Upsert(context.Background(), &model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "a",
		TargetUuid:  "b",
		LastMsgId:   "msg-1",
		LastMsgPrev: "preview-1",
		LastMsgAt:   &seq1Time,
		MaxSeq:      1,
		Status:      0,
	}, false))

	stored = model.Conversation{}
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "a", "p2p-a-b").First(&stored).Error)
	assert.Equal(t, int64(2), stored.MaxSeq)
	assert.Equal(t, "msg-2", stored.LastMsgId)
	assert.Equal(t, 1, stored.UnreadCount)

	seq3Time := time.Now().Truncate(time.Millisecond)
	require.NoError(t, repo.Upsert(context.Background(), &model.Conversation{
		ConvId:      "p2p-a-b",
		Type:        1,
		OwnerUuid:   "a",
		TargetUuid:  "b",
		LastMsgId:   "msg-3",
		LastMsgPrev: "preview-3",
		LastMsgAt:   &seq3Time,
		MaxSeq:      3,
		ReadSeq:     3,
		Status:      0,
	}, true))

	stored = model.Conversation{}
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "a", "p2p-a-b").First(&stored).Error)
	assert.Equal(t, int64(3), stored.MaxSeq)
	assert.Equal(t, "msg-3", stored.LastMsgId)
	assert.Equal(t, "preview-3", stored.LastMsgPrev)
	require.NotNil(t, stored.LastMsgAt)
	assert.Equal(t, seq3Time.UnixMilli(), stored.LastMsgAt.UnixMilli())
	assert.Equal(t, int64(3), stored.ReadSeq)
	assert.Greater(t, stored.UpdatedAt.UnixMilli(), initialUpdatedAt.UnixMilli())
}

func TestRepositoryUpsertGroupConvAdvancesLatestTupleOnlyWithHigherSeq(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	initialUpdatedAt := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	seq2Time := initialUpdatedAt.Add(time.Minute)
	require.NoError(t, db.Create(&model.GroupConversation{
		GroupUuid:   "group-1",
		MaxSeq:      2,
		LastMsgId:   "msg-2",
		LastMsgPrev: "preview-2",
		LastMsgAt:   &seq2Time,
		GroupStatus: model.GroupConversationStatusNormal,
		UpdatedAt:   initialUpdatedAt,
	}).Error)

	// 模拟 seq=1 的 workflow 比 seq=2 更晚执行到会话投影。旧 seq 必须整组跳过，
	// 不能只保住 max_seq 却把预览、排序时间或增量更新时间回退/刷新。
	seq1Time := seq2Time.Add(-time.Minute)
	require.NoError(t, repo.UpsertGroupConv(context.Background(), &model.GroupConversation{
		GroupUuid:   "group-1",
		MaxSeq:      1,
		LastMsgId:   "msg-1",
		LastMsgPrev: "preview-1",
		LastMsgAt:   &seq1Time,
	}))

	var stored model.GroupConversation
	require.NoError(t, db.Where("group_uuid = ?", "group-1").First(&stored).Error)
	assert.Equal(t, int64(2), stored.MaxSeq)
	assert.Equal(t, "msg-2", stored.LastMsgId)
	assert.Equal(t, "preview-2", stored.LastMsgPrev)
	require.NotNil(t, stored.LastMsgAt)
	assert.Equal(t, seq2Time.UnixMilli(), stored.LastMsgAt.UnixMilli())
	assert.Equal(t, initialUpdatedAt.UnixMilli(), stored.UpdatedAt.UnixMilli())

	seq3Time := time.Now().Truncate(time.Millisecond)
	require.NoError(t, repo.UpsertGroupConv(context.Background(), &model.GroupConversation{
		GroupUuid:   "group-1",
		MaxSeq:      3,
		LastMsgId:   "msg-3",
		LastMsgPrev: "preview-3",
		LastMsgAt:   &seq3Time,
	}))

	stored = model.GroupConversation{}
	require.NoError(t, db.Where("group_uuid = ?", "group-1").First(&stored).Error)
	assert.Equal(t, int64(3), stored.MaxSeq)
	assert.Equal(t, "msg-3", stored.LastMsgId)
	assert.Equal(t, "preview-3", stored.LastMsgPrev)
	require.NotNil(t, stored.LastMsgAt)
	assert.Equal(t, seq3Time.UnixMilli(), stored.LastMsgAt.UnixMilli())
	assert.Greater(t, stored.UpdatedAt.UnixMilli(), initialUpdatedAt.UnixMilli())
}

func TestRepositoryListGroupIncrementalReturnsMembershipAndGroupTombstones(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	oldTime := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	memberLeftAt := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	groupDismissedAt := time.Now().Truncate(time.Millisecond)

	for _, conv := range []*model.Conversation{
		{
			ConvId:            "group-left",
			Type:              2,
			OwnerUuid:         "user-1",
			TargetUuid:        "group-left",
			MembershipStatus:  model.ConversationMembershipLeft,
			MembershipVersion: 2,
			Status:            0,
			CreatedAt:         oldTime,
			UpdatedAt:         memberLeftAt,
		},
		{
			ConvId:            "group-dismissed",
			Type:              2,
			OwnerUuid:         "user-1",
			TargetUuid:        "group-dismissed",
			MembershipStatus:  model.ConversationMembershipActive,
			MembershipVersion: 1,
			Status:            0,
			CreatedAt:         oldTime,
			UpdatedAt:         oldTime,
		},
	} {
		require.NoError(t, db.Create(conv).Error)
	}
	for _, group := range []*model.GroupConversation{
		{
			GroupUuid:   "group-left",
			LastMsgAt:   &oldTime,
			GroupStatus: model.GroupConversationStatusNormal,
			UpdatedAt:   oldTime,
		},
		{
			GroupUuid:   "group-dismissed",
			LastMsgAt:   &oldTime,
			GroupStatus: model.GroupConversationStatusDismissed,
			UpdatedAt:   groupDismissedAt,
		},
	} {
		require.NoError(t, db.Create(group).Error)
	}

	got, err := repo.ListGroup(
		context.Background(),
		"user-1",
		oldTime.Add(time.Hour).UnixMilli(),
		0,
		0,
		20,
	)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "group-dismissed", got[0].ConvId)
	assert.Equal(t, int8(1), got[0].Status)
	assert.Equal(t, groupDismissedAt.UnixMilli(), got[0].UpdatedAt.UnixMilli())
	assert.Equal(t, "group-left", got[1].ConvId)
	assert.Equal(t, int8(1), got[1].Status)
	assert.Equal(t, memberLeftAt.UnixMilli(), got[1].UpdatedAt.UnixMilli())
}

func TestRepositoryDeleteGroupUsesSharedMaxSeqAndNewMessageReactivatesView(t *testing.T) {
	db := newConversationRepoTestDB(t)
	repo := NewRepository(db)
	oldTime := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	newTime := time.Now().Truncate(time.Millisecond)

	require.NoError(t, db.Create(&model.Conversation{
		ConvId:            "group-1",
		Type:              2,
		OwnerUuid:         "user-1",
		TargetUuid:        "group-1",
		MaxSeq:            0, // 群读扩散下，成员个人行不跟随每条消息写 max_seq。
		MembershipStatus:  model.ConversationMembershipActive,
		MembershipVersion: 1,
		Status:            0,
		CreatedAt:         oldTime,
		UpdatedAt:         oldTime,
	}).Error)
	require.NoError(t, db.Create(&model.GroupConversation{
		GroupUuid:   "group-1",
		MaxSeq:      8,
		LastMsgAt:   &oldTime,
		GroupStatus: model.GroupConversationStatusNormal,
		UpdatedAt:   oldTime,
	}).Error)

	require.NoError(t, repo.Delete(context.Background(), "user-1", "group-1"))

	var stored model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND conv_id = ?", "user-1", "group-1").First(&stored).Error)
	assert.Equal(t, int8(1), stored.Status)
	assert.Equal(t, int64(8), stored.ClearSeq)
	assert.Equal(t, int64(8), stored.ReadSeq)

	hidden, err := repo.ListGroup(context.Background(), "user-1", 0, 0, 0, 20)
	require.NoError(t, err)
	assert.Empty(t, hidden)

	require.NoError(t, db.Model(&model.GroupConversation{}).
		Where("group_uuid = ?", "group-1").
		Updates(map[string]interface{}{
			"max_seq":     9,
			"last_msg_at": newTime,
			"updated_at":  newTime,
		}).Error)

	visible, err := repo.ListGroup(context.Background(), "user-1", 0, 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	assert.Equal(t, int8(0), visible[0].Status)
	assert.Equal(t, int64(9), visible[0].MaxSeq)
	assert.Equal(t, int64(8), visible[0].ClearSeq)
}
