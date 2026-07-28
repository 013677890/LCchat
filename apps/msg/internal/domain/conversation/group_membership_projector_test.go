package conversation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/outbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newGroupMembershipProjectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:group-projector-%d?mode=memory&cache=shared", conversationTestDBSequence.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Conversation{},
		&model.GroupConversation{},
		&outbox.IdempotentRecord{},
	))
	return db
}

func groupProjectionSnapshot(groupUUID string, status int8, updatedAt time.Time) *groupevent.GroupSnapshot {
	return &groupevent.GroupSnapshot{
		GroupID:         1,
		GroupUUID:       groupUUID,
		Name:            "测试群",
		OwnerUUID:       "owner-1",
		MemberCount:     2,
		AddMode:         0,
		Status:          int32(status),
		UpdatedAtUnixMs: updatedAt.UnixMilli(),
	}
}

func groupProjectionMember(userUUID string, joinedAt time.Time) groupevent.GroupMemberSnapshot {
	role := int32(0)
	if userUUID == "owner-1" {
		role = 2
	}
	return groupevent.GroupMemberSnapshot{
		UserUUID:       userUUID,
		Role:           role,
		JoinedAtUnixMs: joinedAt.UnixMilli(),
	}
}

func TestGroupMembershipProjectorProjectsLifecycleWithoutOverwritingPersonalState(t *testing.T) {
	db := newGroupMembershipProjectorTestDB(t)
	repo := NewGroupMembershipProjectorRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	created := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-created",
		Action:            groupevent.ActionGroupCreated,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
		Members: []groupevent.GroupMemberSnapshot{
			groupProjectionMember("owner-1", now),
			groupProjectionMember("member-1", now),
		},
		UserUUIDs: []string{"owner-1", "member-1"},
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, created))

	var member model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND target_uuid = ?", "member-1", "group-1").First(&member).Error)
	assert.Equal(t, int8(2), member.Type)
	assert.Equal(t, model.ConversationMembershipActive, member.MembershipStatus)
	assert.Equal(t, int64(1), member.MembershipVersion)
	require.NotNil(t, member.MembershipJoinedAt)
	assert.Nil(t, member.MembershipLeftAt)

	// 模拟用户已经产生个人状态。后续 group 事件只能更新 Membership*，这些字段必须保留。
	require.NoError(t, db.Model(&model.Conversation{}).
		Where("owner_uuid = ? AND target_uuid = ?", "member-1", "group-1").
		Updates(map[string]interface{}{
			"read_seq": 37,
			"mute":     true,
			"pin":      true,
			"status":   1,
		}).Error)

	leftAt := now.Add(time.Minute)
	removed := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 2,
		EventID:           "event-removed",
		Action:            groupevent.ActionMemberRemoved,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, leftAt),
		UserUUID:          "member-1",
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, removed))

	member = model.Conversation{}
	require.NoError(t, db.Where("owner_uuid = ? AND target_uuid = ?", "member-1", "group-1").First(&member).Error)
	assert.Equal(t, model.ConversationMembershipLeft, member.MembershipStatus)
	assert.Equal(t, int64(2), member.MembershipVersion)
	require.NotNil(t, member.MembershipLeftAt)
	assert.Equal(t, int64(37), member.ReadSeq)
	assert.True(t, member.Mute)
	assert.True(t, member.Pin)
	assert.Equal(t, int8(1), member.Status)

	rejoinedAt := now.Add(2 * time.Minute)
	added := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 3,
		EventID:           "event-rejoined",
		Action:            groupevent.ActionMemberAdded,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, rejoinedAt),
		Members:           []groupevent.GroupMemberSnapshot{groupProjectionMember("member-1", rejoinedAt)},
		UserUUIDs:         []string{"member-1"},
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, added))

	member = model.Conversation{}
	require.NoError(t, db.Where("owner_uuid = ? AND target_uuid = ?", "member-1", "group-1").First(&member).Error)
	assert.Equal(t, model.ConversationMembershipActive, member.MembershipStatus)
	assert.Equal(t, int64(3), member.MembershipVersion)
	assert.Nil(t, member.MembershipLeftAt)
	assert.Equal(t, int64(37), member.ReadSeq)
	assert.True(t, member.Mute)
	assert.True(t, member.Pin)
	assert.Equal(t, int8(1), member.Status, "重新入群不得擅自覆盖用户主动隐藏状态")

	var groupState model.GroupConversation
	require.NoError(t, db.Where("group_uuid = ?", "group-1").First(&groupState).Error)
	assert.Equal(t, int64(3), groupState.ProjectionVersion)
	assert.Equal(t, model.GroupConversationStatusNormal, groupState.GroupStatus)
}

func TestGroupMembershipProjectorRejectsVersionGapAndIgnoresStaleVersion(t *testing.T) {
	db := newGroupMembershipProjectorTestDB(t)
	repo := NewGroupMembershipProjectorRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	created := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-created",
		Action:            groupevent.ActionGroupCreated,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
		Members:           []groupevent.GroupMemberSnapshot{groupProjectionMember("owner-1", now)},
		UserUUIDs:         []string{"owner-1"},
	}
	created.Group.MemberCount = 1
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, created))

	gap := created
	gap.EventID = "event-gap"
	gap.ProjectionVersion = 3
	gap.Action = groupevent.ActionMemberAdded
	gap.Members = []groupevent.GroupMemberSnapshot{groupProjectionMember("member-2", now)}
	gap.UserUUIDs = []string{"member-2"}
	err := repo.ApplyGroupCacheEvent(ctx, gap)
	require.ErrorIs(t, err, ErrGroupProjectionVersionGap)

	next := gap
	next.EventID = "event-next"
	next.ProjectionVersion = 2
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, next))

	staleRemove := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-stale-remove",
		Action:            groupevent.ActionMemberRemoved,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
		UserUUID:          "member-2",
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, staleRemove))

	var member model.Conversation
	require.NoError(t, db.Where("owner_uuid = ? AND target_uuid = ?", "member-2", "group-1").First(&member).Error)
	assert.Equal(t, model.ConversationMembershipActive, member.MembershipStatus)
	assert.Equal(t, int64(2), member.MembershipVersion)
}

func TestGroupMembershipProjectorRejectsInvalidLifecycleTransitions(t *testing.T) {
	t.Run("首条事件不是建群", func(t *testing.T) {
		db := newGroupMembershipProjectorTestDB(t)
		repo := NewGroupMembershipProjectorRepository(db)
		now := time.Now().Truncate(time.Millisecond)

		firstAdd := groupevent.GroupCacheEventPayload{
			SchemaVersion:     groupevent.GroupCacheSchemaVersion,
			ProjectionVersion: 1,
			EventID:           "event-first-add",
			Action:            groupevent.ActionMemberAdded,
			GroupUUID:         "group-1",
			Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
			Members:           []groupevent.GroupMemberSnapshot{groupProjectionMember("member-1", now)},
			UserUUIDs:         []string{"member-1"},
		}
		err := repo.ApplyGroupCacheEvent(context.Background(), firstAdd)
		require.ErrorIs(t, err, ErrInvalidGroupProjectionEvent)

		var count int64
		require.NoError(t, db.Model(&model.GroupConversation{}).
			Where("group_uuid = ?", "group-1").
			Count(&count).Error)
		assert.Zero(t, count, "非法首事件必须连同事务内的占位群状态一起回滚")
	})

	t.Run("已建立的群再次建群", func(t *testing.T) {
		db := newGroupMembershipProjectorTestDB(t)
		repo := NewGroupMembershipProjectorRepository(db)
		now := time.Now().Truncate(time.Millisecond)
		created := groupevent.GroupCacheEventPayload{
			SchemaVersion:     groupevent.GroupCacheSchemaVersion,
			ProjectionVersion: 1,
			EventID:           "event-created",
			Action:            groupevent.ActionGroupCreated,
			GroupUUID:         "group-1",
			Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
			Members: []groupevent.GroupMemberSnapshot{
				groupProjectionMember("owner-1", now),
				groupProjectionMember("member-1", now),
			},
			UserUUIDs: []string{"owner-1", "member-1"},
		}
		require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), created))

		createdAgain := created
		createdAgain.EventID = "event-created-again"
		createdAgain.ProjectionVersion = 2
		err := repo.ApplyGroupCacheEvent(context.Background(), createdAgain)
		require.ErrorIs(t, err, ErrInvalidGroupProjectionEvent)

		var state model.GroupConversation
		require.NoError(t, db.Where("group_uuid = ?", "group-1").First(&state).Error)
		assert.Equal(t, int64(1), state.ProjectionVersion)
	})
}

func TestGroupMembershipProjectorRejectsRemovingUnknownMember(t *testing.T) {
	db := newGroupMembershipProjectorTestDB(t)
	repo := NewGroupMembershipProjectorRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	created := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-created",
		Action:            groupevent.ActionGroupCreated,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
		Members: []groupevent.GroupMemberSnapshot{
			groupProjectionMember("owner-1", now),
			groupProjectionMember("member-1", now),
		},
		UserUUIDs: []string{"owner-1", "member-1"},
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, created))

	removed := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 2,
		EventID:           "event-remove-unknown",
		Action:            groupevent.ActionMemberRemoved,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now.Add(time.Minute)),
		UserUUID:          "unknown-member",
	}
	err := repo.ApplyGroupCacheEvent(ctx, removed)
	require.ErrorIs(t, err, ErrInvalidGroupProjectionEvent)

	var state model.GroupConversation
	require.NoError(t, db.Where("group_uuid = ?", "group-1").First(&state).Error)
	assert.Equal(t, int64(1), state.ProjectionVersion, "成员不变量错误必须回滚单群版本推进")
}

func TestGroupMembershipProjectorDismissalUsesSingleSharedGroupState(t *testing.T) {
	db := newGroupMembershipProjectorTestDB(t)
	repo := NewGroupMembershipProjectorRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	created := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-created",
		Action:            groupevent.ActionGroupCreated,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, now),
		Members: []groupevent.GroupMemberSnapshot{
			groupProjectionMember("owner-1", now),
			groupProjectionMember("member-1", now),
		},
		UserUUIDs: []string{"owner-1", "member-1"},
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, created))

	dismissedAt := now.Add(time.Minute)
	dismissed := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 2,
		EventID:           "event-dismissed",
		Action:            groupevent.ActionGroupDismissed,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusDismissed, dismissedAt),
		UserUUIDs:         []string{"owner-1", "member-1"},
	}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, dismissed))

	var groupState model.GroupConversation
	require.NoError(t, db.Where("group_uuid = ?", "group-1").First(&groupState).Error)
	assert.Equal(t, model.GroupConversationStatusDismissed, groupState.GroupStatus)
	assert.Equal(t, int64(2), groupState.ProjectionVersion)

	// 解散只更新一条共享群状态，不做 N 行成员 UPDATE；列表和读取都通过共享状态拒绝。
	var activeRows int64
	require.NoError(t, db.Model(&model.Conversation{}).
		Where("target_uuid = ? AND membership_status = ?", "group-1", model.ConversationMembershipActive).
		Count(&activeRows).Error)
	assert.Equal(t, int64(2), activeRows)
}

func TestGroupMembershipProjectorRejectsLegacyOrIncompleteMembershipPayload(t *testing.T) {
	db := newGroupMembershipProjectorTestDB(t)
	repo := NewGroupMembershipProjectorRepository(db)

	payload := groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-invalid",
		Action:            groupevent.ActionMemberAdded,
		GroupUUID:         "group-1",
		Group:             groupProjectionSnapshot("group-1", model.GroupConversationStatusNormal, time.Now()),
		Members:           []groupevent.GroupMemberSnapshot{groupProjectionMember("member-1", time.Now())},
		// 当前 v2 契约要求 user_uuids 与 members 显式表达同一集合；禁止从 members 推导。
		UserUUIDs: nil,
	}
	err := repo.ApplyGroupCacheEvent(context.Background(), payload)
	require.ErrorIs(t, err, ErrInvalidGroupProjectionEvent)
}
