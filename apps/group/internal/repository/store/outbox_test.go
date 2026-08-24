package store

import (
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/event"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildGroupSnapshotRoundTrip(t *testing.T) {
	updatedAt := time.Unix(1710000000, 0)
	group := &model.GroupInfo{
		Id:        42,
		Uuid:      "group-1",
		Name:      "测试群",
		Avatar:    "avatar.png",
		Notice:    "欢迎加入",
		OwnerUuid: "owner-1",
		MemberCnt: 3,
		AddMode:   1,
		Status:    repository.GroupStatusNormal,
		UpdatedAt: updatedAt,
	}

	snapshot := buildGroupSnapshot(group)
	restored := repository.BuildGroupInfoFromSnapshot(snapshot)
	if assert.NotNil(t, snapshot) && assert.NotNil(t, restored) {
		assert.Equal(t, group.Uuid, snapshot.GroupUUID)
		assert.Equal(t, group.Id, snapshot.GroupID)
		assert.Equal(t, group.MemberCnt, int(snapshot.MemberCount))
		assert.Equal(t, group.UpdatedAt.UnixMilli(), snapshot.UpdatedAtUnixMs)
		assert.Equal(t, group.Id, restored.Id)
		assert.Equal(t, group.Uuid, restored.Uuid)
		assert.Equal(t, group.Name, restored.Name)
		assert.Equal(t, group.Avatar, restored.Avatar)
		assert.Equal(t, group.Notice, restored.Notice)
		assert.Equal(t, group.OwnerUuid, restored.OwnerUuid)
		assert.Equal(t, group.MemberCnt, restored.MemberCnt)
		assert.Equal(t, group.AddMode, restored.AddMode)
		assert.Equal(t, group.Status, restored.Status)
		assert.True(t, group.UpdatedAt.Equal(restored.UpdatedAt))
	}
}

func TestBuildGroupMemberSnapshotsDeduplicate(t *testing.T) {
	joinedAt := time.UnixMilli(1710000000123)
	members := []*model.GroupMember{
		{GroupUuid: "group-1", UserUuid: "user-1", Role: repository.MemberRoleAdmin, JoinedAt: joinedAt},
		nil,
		{GroupUuid: "group-1", UserUuid: "user-1", Role: repository.MemberRoleMember, JoinedAt: joinedAt.Add(time.Minute)},
		{GroupUuid: "group-1", UserUuid: "user-2", Role: repository.MemberRoleMember, JoinedAt: joinedAt.Add(2 * time.Minute)},
	}

	snapshots := buildGroupMemberSnapshots(members)
	userUUIDs := collectGroupMemberSnapshotUUIDs(members)
	if assert.Len(t, snapshots, 2) {
		assert.Equal(t, "user-1", snapshots[0].UserUUID)
		assert.Equal(t, int32(repository.MemberRoleAdmin), snapshots[0].Role)
		assert.Equal(t, joinedAt.UnixMilli(), snapshots[0].JoinedAtUnixMs)
		assert.Equal(t, "user-2", snapshots[1].UserUUID)
		assert.Equal(t, int32(repository.MemberRoleMember), snapshots[1].Role)
	}
	assert.Equal(t, []string{"user-1", "user-2"}, userUUIDs)
}

func TestBuildGroupJoinRequestSnapshotRoundTrip(t *testing.T) {
	createdAt := time.Unix(1710000000, 0)
	request := &model.GroupJoinRequest{
		Id:            7,
		ApplicantUuid: "user-1",
		Reason:        "申请加入",
		Status:        repository.JoinRequestStatusPending,
		CreatedAt:     createdAt,
	}

	snapshot := buildGroupJoinRequestSnapshot(request)
	restored := repository.BuildGroupJoinRequestFromSnapshot(snapshot)
	if assert.NotNil(t, snapshot) && assert.NotNil(t, restored) {
		assert.Equal(t, request.Id, snapshot.ApplyID)
		assert.Equal(t, request.ApplicantUuid, snapshot.ApplicantUUID)
		assert.Equal(t, request.Reason, snapshot.Reason)
		assert.Equal(t, request.CreatedAt.UnixMilli(), snapshot.CreatedAtUnixMs)
		assert.Equal(t, request.Id, restored.Id)
		assert.Equal(t, request.ApplicantUuid, restored.ApplicantUuid)
		assert.Equal(t, request.Reason, restored.Reason)
		assert.True(t, request.CreatedAt.Equal(restored.CreatedAt))
	}
}

func TestInsertGroupCacheEventAllocatesVersionPerEventInTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.GroupInfo{}, &outbox.Event{}))
	now := time.UnixMilli(1710009000123)
	group := &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "group",
		OwnerUuid: "owner-1",
		MemberCnt: 1,
		UpdatedAt: now,
	}
	require.NoError(t, db.Create(group).Error)
	mysqlStore := New(db, nil)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for index := int64(1); index <= 2; index++ {
			payload := event.GroupCacheEventPayload{
				EventID:   "event-" + time.Unix(index, 0).Format("150405"),
				Action:    event.ActionGroupInfoUpdated,
				GroupUUID: group.Uuid,
				Group:     buildGroupSnapshot(group),
			}
			if err := mysqlStore.insertGroupCacheEvent(tx, payload); err != nil {
				return err
			}
		}
		return nil
	}))

	var persisted model.GroupInfo
	require.NoError(t, db.Where("uuid = ?", group.Uuid).Take(&persisted).Error)
	assert.Equal(t, int64(2), persisted.CacheVersion)

	var events []outbox.Event
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	for index, outboxEvent := range events {
		payload, decodeErr := event.DecodeGroupCache([]byte(outboxEvent.Payload))
		require.NoError(t, decodeErr)
		assert.Equal(t, int64(index+1), payload.ProjectionVersion)
		assert.Equal(t, event.GroupCacheSchemaVersion, payload.SchemaVersion)
	}
}

func TestInsertGroupCacheEventRollsBackVersionWithOutbox(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.GroupInfo{}, &outbox.Event{}))
	group := &model.GroupInfo{
		Uuid:      "group-rollback",
		Name:      "group",
		OwnerUuid: "owner-1",
		MemberCnt: 1,
		UpdatedAt: time.UnixMilli(1710010000123),
	}
	require.NoError(t, db.Create(group).Error)
	mysqlStore := New(db, nil)
	rollbackErr := errors.New("force rollback")

	err = db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, mysqlStore.insertGroupCacheEvent(tx, event.GroupCacheEventPayload{
			EventID:   "event-rollback",
			Action:    event.ActionGroupInfoUpdated,
			GroupUUID: group.Uuid,
			Group:     buildGroupSnapshot(group),
		}))
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	var persisted model.GroupInfo
	require.NoError(t, db.Where("uuid = ?", group.Uuid).Take(&persisted).Error)
	assert.Zero(t, persisted.CacheVersion)
	var eventCount int64
	require.NoError(t, db.Model(&outbox.Event{}).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
}
