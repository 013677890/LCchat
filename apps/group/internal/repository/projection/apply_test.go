package projection

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/event"
	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newProjectorTestRepository(t *testing.T) (*Repository, *goredis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Repository{redisClient: client}, client
}

func newProjectorReconcileTestRepository(t *testing.T) (*Repository, *goredis.Client, *gorm.DB) {
	t.Helper()
	repo, client := newProjectorTestRepository(t)
	dsn := fmt.Sprintf("file:group_cache_reconcile_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.GroupInfo{},
		&model.GroupMember{},
		&model.GroupJoinRequest{},
	))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo.db = db
	return repo, client, db
}

func projectorGroupSnapshot(groupUUID, name string, memberCount int, status int8, updatedAt time.Time) *event.GroupSnapshot {
	return &event.GroupSnapshot{
		GroupID:         1,
		GroupUUID:       groupUUID,
		Name:            name,
		Avatar:          "avatar.png",
		Notice:          "notice",
		OwnerUUID:       "owner-1",
		MemberCount:     int32(memberCount),
		AddMode:         0,
		Status:          int32(status),
		UpdatedAtUnixMs: updatedAt.UnixMilli(),
	}
}

func projectorMemberSnapshot(userUUID string, role int8, joinedAt time.Time) event.GroupMemberSnapshot {
	return event.GroupMemberSnapshot{
		UserUUID:       userUUID,
		Role:           int32(role),
		JoinedAtUnixMs: joinedAt.UnixMilli(),
	}
}

func projectorPayload(version int64, action string, group *event.GroupSnapshot) event.GroupCacheEventPayload {
	return event.GroupCacheEventPayload{
		SchemaVersion:     event.GroupCacheSchemaVersion,
		ProjectionVersion: version,
		EventID:           fmt.Sprintf("event-%d-%s", version, action),
		Action:            action,
		GroupUUID:         group.GroupUUID,
		Group:             group,
	}
}

func readGroupInfoProjection(
	t *testing.T,
	client *goredis.Client,
	groupUUID string,
) (*repository.GroupInfoCacheEntry, int64) {
	t.Helper()
	raw, err := client.Get(context.Background(), rediskey.GroupInfoKey(groupUUID)).Result()
	require.NoError(t, err)
	entry, version, empty, err := repository.DecodeGroupInfoCacheValue(raw)
	require.NoError(t, err)
	require.False(t, empty)
	return entry, version
}

func requireHashVersion(t *testing.T, client *goredis.Client, key string, version int64) map[string]string {
	t.Helper()
	values, err := client.HGetAll(context.Background(), key).Result()
	require.NoError(t, err)
	require.Equal(t, repository.GroupCacheSchemaVersion, values[repository.GroupProjectionSchemaField])
	require.Equal(t, strconv.FormatInt(version, 10), values[repository.GroupProjectionVersionField])
	require.Equal(t, "1", values[repository.GroupProjectionCompleteField])
	return values
}

func TestApplyGroupCreatedBuildsStrictVersionedProjection(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	now := time.UnixMilli(1710000000123)
	payload := projectorPayload(
		10,
		event.ActionGroupCreated,
		projectorGroupSnapshot("group-1", "v10", 2, repository.GroupStatusNormal, now),
	)
	payload.Members = []event.GroupMemberSnapshot{
		projectorMemberSnapshot("owner-1", repository.MemberRoleOwner, now),
		projectorMemberSnapshot("member-1", repository.MemberRoleMember, now),
	}
	payload.UserUUIDs = []string{"owner-1", "member-1"}

	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), payload))

	info, version := readGroupInfoProjection(t, client, "group-1")
	assert.Equal(t, int64(10), version)
	assert.Equal(t, "v10", info.Name)

	memberFields := requireHashVersion(t, client, rediskey.GroupMembersKey("group-1"), 10)
	assert.Contains(t, memberFields, "owner-1")
	assert.Contains(t, memberFields, "member-1")

	for _, userUUID := range payload.UserUUIDs {
		groups, err := client.ZRange(context.Background(), rediskey.UserGroupListKey(userUUID), 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{"group-1"}, groups)
		versions, err := client.HGetAll(context.Background(), rediskey.UserGroupVersionKey(userUUID)).Result()
		require.NoError(t, err)
		assert.Equal(t, "10", versions["group-1"])
		assert.Empty(t, versions[repository.UserGroupsReadyField], "单条事件不能把局部反向索引标成完整列表")
	}
}

func TestProjectionRejectsOutOfOrderGroupInfo(t *testing.T) {
	repo, _ := newProjectorTestRepository(t)
	now := time.UnixMilli(1710001000123)

	created := projectorPayload(10, event.ActionGroupCreated, projectorGroupSnapshot("group-1", "v10", 1, 0, now))
	created.Members = []event.GroupMemberSnapshot{projectorMemberSnapshot("owner-1", repository.MemberRoleOwner, now)}
	created.UserUUIDs = []string{"owner-1"}
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), created))

	newer := projectorPayload(12, event.ActionGroupInfoUpdated, projectorGroupSnapshot("group-1", "v12", 1, 0, now.Add(time.Second)))
	older := projectorPayload(11, event.ActionGroupInfoUpdated, projectorGroupSnapshot("group-1", "v11", 1, 0, now.Add(2*time.Second)))
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), newer))
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), older))

	info, version := readGroupInfoProjection(t, repo.redisClient, "group-1")
	assert.Equal(t, int64(12), version)
	assert.Equal(t, "v12", info.Name)
}

func TestOwnerTransferUpdatesTwoMembersInOneVersionedLua(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	now := time.UnixMilli(1710002000123)
	created := projectorPayload(1, event.ActionGroupCreated, projectorGroupSnapshot("group-1", "group", 2, 0, now))
	created.Members = []event.GroupMemberSnapshot{
		projectorMemberSnapshot("owner-1", repository.MemberRoleOwner, now),
		projectorMemberSnapshot("member-1", repository.MemberRoleMember, now),
	}
	created.UserUUIDs = []string{"owner-1", "member-1"}
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), created))

	transfer := projectorPayload(2, event.ActionOwnerTransferred, projectorGroupSnapshot("group-1", "group", 2, 0, now.Add(time.Second)))
	transfer.Group.OwnerUUID = "member-1"
	transfer.Members = []event.GroupMemberSnapshot{
		projectorMemberSnapshot("owner-1", repository.MemberRoleMember, now),
		projectorMemberSnapshot("member-1", repository.MemberRoleOwner, now),
	}
	transfer.UserUUIDs = []string{"owner-1", "member-1"}
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), transfer))

	fields := requireHashVersion(t, client, rediskey.GroupMembersKey("group-1"), 2)
	oldOwner, err := repository.DecodeGroupMemberCacheValue(fields["owner-1"])
	require.NoError(t, err)
	newOwner, err := repository.DecodeGroupMemberCacheValue(fields["member-1"])
	require.NoError(t, err)
	assert.Equal(t, int8(repository.MemberRoleMember), oldOwner.Role)
	assert.Equal(t, int8(repository.MemberRoleOwner), newOwner.Role)
}

func TestRemovalTombstoneRejectsLateAdd(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	now := time.UnixMilli(1710003000123)
	created := projectorPayload(5, event.ActionGroupCreated, projectorGroupSnapshot("group-1", "group", 2, 0, now))
	created.Members = []event.GroupMemberSnapshot{
		projectorMemberSnapshot("owner-1", repository.MemberRoleOwner, now),
		projectorMemberSnapshot("member-1", repository.MemberRoleMember, now),
	}
	created.UserUUIDs = []string{"owner-1", "member-1"}
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), created))

	removed := projectorPayload(7, event.ActionMemberRemoved, projectorGroupSnapshot("group-1", "group", 1, 0, now.Add(2*time.Second)))
	removed.UserUUID = "member-1"
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), removed))

	lateAdd := projectorPayload(6, event.ActionMemberAdded, projectorGroupSnapshot("group-1", "group", 2, 0, now.Add(time.Second)))
	lateAdd.Members = []event.GroupMemberSnapshot{projectorMemberSnapshot("member-1", repository.MemberRoleMember, now)}
	lateAdd.UserUUIDs = []string{"member-1"}
	require.NoError(t, repo.ApplyGroupCacheEvent(context.Background(), lateAdd))

	fields := requireHashVersion(t, client, rediskey.GroupMembersKey("group-1"), 7)
	assert.NotContains(t, fields, "member-1")
	groups, err := client.ZRange(context.Background(), rediskey.UserGroupListKey("member-1"), 0, -1).Result()
	require.NoError(t, err)
	assert.Equal(t, []string{repository.UserGroupsEmptyValue}, groups)
	assert.Equal(
		t,
		"7",
		client.HGet(context.Background(), rediskey.UserGroupVersionKey("member-1"), "group-1").Val(),
	)
}

func TestStaleFullRebuildCannotOverwriteNewProjection(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	now := time.UnixMilli(1710004000123)
	newMembers := []*model.GroupMember{{GroupUuid: "group-1", UserUuid: "new-user", JoinedAt: now}}
	oldMembers := []*model.GroupMember{{GroupUuid: "group-1", UserUuid: "old-user", JoinedAt: now}}

	require.NoError(t, repo.replaceGroupMembersProjection(context.Background(), "group-1", newMembers, 20, false))
	require.NoError(t, repo.replaceGroupMembersProjection(context.Background(), "group-1", oldMembers, 19, false))

	fields := requireHashVersion(t, client, rediskey.GroupMembersKey("group-1"), 20)
	assert.Contains(t, fields, "new-user")
	assert.NotContains(t, fields, "old-user")
}

func TestOldCacheFormatIsDeletedInsteadOfAdapted(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710005000123)

	require.NoError(t, client.Set(ctx, rediskey.GroupInfoKey("group-1"), `{"group_uuid":"group-1"}`, time.Hour).Err())
	update := projectorPayload(2, event.ActionGroupInfoUpdated, projectorGroupSnapshot("group-1", "new", 1, 0, now))
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, update))
	assert.Equal(t, int64(0), client.Exists(ctx, rediskey.GroupInfoKey("group-1")).Val())

	require.NoError(t, client.HSet(ctx, rediskey.GroupMembersKey("group-1"), "member-1", `{}`).Err())
	roleUpdate := projectorPayload(3, event.ActionMemberRoleUpdated, projectorGroupSnapshot("group-1", "new", 1, 0, now))
	roleUpdate.Members = []event.GroupMemberSnapshot{projectorMemberSnapshot("member-1", repository.MemberRoleAdmin, now)}
	roleUpdate.UserUUIDs = []string{"member-1"}
	require.NoError(t, repo.ApplyGroupCacheEvent(ctx, roleUpdate))
	assert.Equal(t, int64(0), client.Exists(ctx, rediskey.GroupMembersKey("group-1")).Val())
}

func TestAtomicReadScriptsRejectIncompatibleCacheWithoutCompatibilityFallback(t *testing.T) {
	_, client := newProjectorTestRepository(t)
	ctx := context.Background()
	ttlSeconds := int(cachex.JitterTTL(rediskey.GroupMembersTTL).Seconds())

	memberKey := rediskey.GroupMembersKey("group-1")
	require.NoError(t, client.HSet(ctx, memberKey, map[string]string{
		repository.GroupProjectionSchemaField:  "1",
		repository.GroupProjectionVersionField: "8",
		"user-1":                               `{"role":0,"joined_at_unix_ms":1710000000000}`,
	}).Err())
	_, _, cacheHit, err := repository.ReadVersionedHashField(ctx, client, memberKey, "user-1", ttlSeconds, false)
	require.NoError(t, err)
	assert.False(t, cacheHit)
	assert.Equal(t, int64(0), client.Exists(ctx, memberKey).Val())

	// 只有 schema/version 也不是当前格式；缺少 complete 凭证必须直接失效。
	require.NoError(t, client.HSet(ctx, memberKey, map[string]string{
		repository.GroupProjectionSchemaField:  repository.GroupCacheSchemaVersion,
		repository.GroupProjectionVersionField: "8",
	}).Err())
	_, _, cacheHit, err = repository.ReadVersionedHashField(ctx, client, memberKey, "user-1", ttlSeconds, false)
	require.NoError(t, err)
	assert.False(t, cacheHit)
	assert.Equal(t, int64(0), client.Exists(ctx, memberKey).Val())

	listKey := rediskey.UserGroupListKey("user-1")
	versionKey := rediskey.UserGroupVersionKey("user-1")
	require.NoError(t, client.ZAdd(ctx, listKey, goredis.Z{Score: 1, Member: "group-1"}).Err())
	require.NoError(t, client.HSet(ctx, versionKey, map[string]string{
		repository.GroupProjectionSchemaField: "1",
		repository.UserGroupsReadyField:       "1",
		"group-1":                             "8",
	}).Err())
	references, hit, err := repository.ReadVersionedUserGroups(ctx, client, "user-1", false)
	require.NoError(t, err)
	assert.False(t, hit)
	assert.Nil(t, references)
	assert.Equal(t, int64(0), client.Exists(ctx, listKey, versionKey).Val())
}

func TestIncrementalHashScriptsCannotPromoteMetadataOnlyCacheToCompleteState(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710005500123)
	memberKey := rediskey.GroupMembersKey("group-1")

	writeMetadataOnly := func() {
		t.Helper()
		require.NoError(t, client.HSet(ctx, memberKey, map[string]string{
			repository.GroupProjectionSchemaField:   repository.GroupCacheSchemaVersion,
			repository.GroupProjectionVersionField:  "8",
			repository.GroupProjectionCompleteField: "1",
		}).Err())
	}

	writeMetadataOnly()
	require.NoError(t, repo.upsertGroupMembersProjectionIfExists(
		ctx,
		"group-1",
		[]*model.GroupMember{{
			GroupUuid: "group-1",
			UserUuid:  "user-1",
			Role:      repository.MemberRoleMember,
			JoinedAt:  now,
		}},
		9,
	))
	assert.Equal(t, int64(0), client.Exists(ctx, memberKey).Val(),
		"局部 upsert 必须清理不完整 Hash，不能把它伪装成只有一个成员的完整缓存")

	writeMetadataOnly()
	require.NoError(t, repo.removeGroupMemberProjectionIfExists(ctx, "group-1", "user-1", 9))
	assert.Equal(t, int64(0), client.Exists(ctx, memberKey).Val(),
		"局部 remove 必须清理不完整 Hash，不能把它伪装成权威空集合")
}

func TestReconcileGroupCacheRepairsAllProjectionsFromDB(t *testing.T) {
	repo, client, db := newProjectorReconcileTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006000123)
	group := &model.GroupInfo{
		Uuid:         "group-1",
		Name:         "authoritative",
		OwnerUuid:    "owner-1",
		MemberCnt:    1,
		Status:       repository.GroupStatusNormal,
		CacheVersion: 9,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(group).Error)
	active := &model.GroupMember{
		GroupUuid: "group-1",
		UserUuid:  "owner-1",
		Role:      repository.MemberRoleOwner,
		Status:    repository.MemberStatusNormal,
		JoinedAt:  now.Add(-time.Hour),
	}
	removed := &model.GroupMember{
		GroupUuid: "group-1",
		UserUuid:  "removed-1",
		Role:      repository.MemberRoleMember,
		Status:    repository.MemberStatusKicked,
		JoinedAt:  now.Add(-time.Hour),
	}
	require.NoError(t, db.Create(active).Error)
	require.NoError(t, db.Create(removed).Error)
	require.NoError(t, db.Model(removed).Update("deleted_at", now).Error)
	require.NoError(t, db.Create(&model.GroupJoinRequest{
		GroupUuid:     "group-1",
		ApplicantUuid: "pending-1",
		Status:        repository.JoinRequestStatusPending,
		CreatedAt:     now,
	}).Error)
	require.NoError(t, db.Create(&model.GroupJoinRequest{
		GroupUuid:     "group-1",
		ApplicantUuid: "reviewed-1",
		Status:        repository.JoinRequestStatusApproved,
		CreatedAt:     now,
	}).Error)

	// 先放一份低版本反向索引，验证对账既补 active，也按历史软删关系修 remove。
	for _, userUUID := range []string{"owner-1", "removed-1"} {
		require.NoError(t, client.ZAdd(ctx, rediskey.UserGroupListKey(userUUID), goredis.Z{
			Score:  float64(now.UnixMilli()),
			Member: "group-1",
		}).Err())
		require.NoError(t, client.HSet(ctx, rediskey.UserGroupVersionKey(userUUID), map[string]string{
			repository.GroupProjectionSchemaField: repository.GroupCacheSchemaVersion,
			repository.UserGroupsReadyField:       "1",
			"group-1":                             "3",
		}).Err())
	}

	require.NoError(t, repo.ReconcileGroupCache(ctx, "group-1"))

	info, version := readGroupInfoProjection(t, client, "group-1")
	assert.Equal(t, int64(9), version)
	assert.Equal(t, "authoritative", info.Name)

	memberFields := requireHashVersion(t, client, rediskey.GroupMembersKey("group-1"), 9)
	assert.Contains(t, memberFields, "owner-1")
	assert.NotContains(t, memberFields, "removed-1")

	requestFields := requireHashVersion(t, client, rediskey.GroupJoinRequestPendingKey("group-1"), 9)
	assert.Len(t, requestFields, 4, "schema、version、complete 和唯一 pending 申请")

	assert.Equal(t, []string{"group-1"}, client.ZRange(ctx, rediskey.UserGroupListKey("owner-1"), 0, -1).Val())
	assert.Equal(t, []string{repository.UserGroupsEmptyValue}, client.ZRange(ctx, rediskey.UserGroupListKey("removed-1"), 0, -1).Val())
	assert.Equal(t, "9", client.HGet(ctx, rediskey.UserGroupVersionKey("removed-1"), "group-1").Val())
}

func TestReconcileSoftDeletedGroupPublishesUnavailableTombstones(t *testing.T) {
	repo, client, db := newProjectorReconcileTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006250123)
	group := &model.GroupInfo{
		Uuid:         "deleted-group",
		Name:         "must-not-resurrect",
		OwnerUuid:    "owner-1",
		MemberCnt:    1,
		Status:       repository.GroupStatusNormal,
		CacheVersion: 6,
		UpdatedAt:    now,
		DeletedAt:    gorm.DeletedAt{Time: now, Valid: true},
	}
	require.NoError(t, db.Unscoped().Create(group).Error)
	require.NoError(t, db.Create(&model.GroupMember{
		GroupUuid: group.Uuid,
		UserUuid:  "owner-1",
		Role:      repository.MemberRoleOwner,
		Status:    repository.MemberStatusNormal,
		JoinedAt:  now.Add(-time.Hour),
	}).Error)

	require.NoError(t, repo.ReconcileGroupCache(ctx, group.Uuid))

	info, version := readGroupInfoProjection(t, client, group.Uuid)
	assert.Equal(t, int64(6), version)
	assert.Equal(t, repository.GroupStatusDismissed, info.Status,
		"软删除行必须投影为正版本终态，不能因 DB status=0 复活成正常群")
	memberFields := requireHashVersion(t, client, rediskey.GroupMembersKey(group.Uuid), 6)
	assert.Equal(t, repository.GroupMembersEmptyValue, memberFields[repository.GroupMembersEmptyField])

	// 读路径约定：解散终态对 GetGroupInfo 映射为不存在，对 GetGroupMembers 映射为群已解散。
	// 这里只断言投影内容，避免 projection 测试 import cache 造成 import cycle。
	restored := repository.BuildGroupInfoFromCache(info, version)
	require.NotNil(t, restored)
	assert.Equal(t, repository.GroupStatusDismissed, restored.Status)
}

func TestReconcileGroupCacheRepairsCorruptionAtEqualVersion(t *testing.T) {
	repo, client, db := newProjectorReconcileTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006500123)
	group := &model.GroupInfo{
		Uuid:         "group-equal",
		Name:         "authoritative",
		OwnerUuid:    "owner-equal",
		MemberCnt:    1,
		Status:       repository.GroupStatusNormal,
		CacheVersion: 9,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(group).Error)
	owner := &model.GroupMember{
		GroupUuid: "group-equal",
		UserUuid:  "owner-equal",
		Role:      repository.MemberRoleOwner,
		Status:    repository.MemberStatusNormal,
		JoinedAt:  now.Add(-time.Hour),
	}
	require.NoError(t, db.Create(owner).Error)
	request := &model.GroupJoinRequest{
		GroupUuid:     "group-equal",
		ApplicantUuid: "pending-equal",
		Status:        repository.JoinRequestStatusPending,
		CreatedAt:     now,
	}
	require.NoError(t, db.Create(request).Error)

	// 四类缓存都带着“看似最新”的 v9，但业务值故意写错。权威对账若只接受
	// incoming>current，就永远无法修复这种版本元数据未丢、内容已损坏的场景。
	corruptGroup := *group
	corruptGroup.Name = "corrupt"
	require.NoError(t, repo.setVersionedGroupInfoProjection(
		ctx,
		&corruptGroup,
		9,
		groupInfoCreateIfMissing,
	))
	corruptMember := *owner
	corruptMember.Role = repository.MemberRoleMember
	require.NoError(t, repo.replaceGroupMembersProjection(
		ctx,
		group.Uuid,
		[]*model.GroupMember{&corruptMember},
		9,
		false,
	))
	corruptRequest := &model.GroupJoinRequest{
		Id:            request.Id + 100,
		ApplicantUuid: "wrong-applicant",
		CreatedAt:     now,
	}
	require.NoError(t, repo.replaceGroupJoinRequestsProjection(
		ctx,
		group.Uuid,
		[]*model.GroupJoinRequest{corruptRequest},
		9,
		false,
	))
	require.NoError(t, repo.patchUserGroupProjection(ctx, owner.UserUuid, group, false, 9, false))

	require.NoError(t, repo.ReconcileGroupCache(ctx, group.Uuid))

	info, version := readGroupInfoProjection(t, client, group.Uuid)
	assert.Equal(t, int64(9), version)
	assert.Equal(t, group.Name, info.Name)

	memberFields := requireHashVersion(t, client, rediskey.GroupMembersKey(group.Uuid), 9)
	memberEntry, err := repository.DecodeGroupMemberCacheValue(memberFields[owner.UserUuid])
	require.NoError(t, err)
	assert.Equal(t, repository.MemberRoleOwner, memberEntry.Role)

	requestFields := requireHashVersion(t, client, rediskey.GroupJoinRequestPendingKey(group.Uuid), 9)
	assert.Contains(t, requestFields, strconv.FormatInt(request.Id, 10))
	assert.NotContains(t, requestFields, strconv.FormatInt(corruptRequest.Id, 10))
	assert.Equal(t, []string{group.Uuid}, client.ZRange(ctx, rediskey.UserGroupListKey(owner.UserUuid), 0, -1).Val())
}

func TestReconcileUserGroupsRemovesEqualVersionStrayGroup(t *testing.T) {
	repo, client, db := newProjectorReconcileTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006750123)
	group := &model.GroupInfo{
		Uuid:         "stray-group",
		Name:         "group",
		OwnerUuid:    "owner-1",
		Status:       repository.GroupStatusNormal,
		CacheVersion: 5,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(group).Error)

	// user-1 在 DB 中从未加入该群，但缓存错误地把它标为 active v5。
	require.NoError(t, client.ZAdd(ctx, rediskey.UserGroupListKey("user-1"), goredis.Z{
		Score:  float64(now.UnixMilli()),
		Member: group.Uuid,
	}).Err())
	require.NoError(t, client.HSet(ctx, rediskey.UserGroupVersionKey("user-1"), map[string]string{
		repository.GroupProjectionSchemaField: repository.GroupCacheSchemaVersion,
		repository.UserGroupsReadyField:       "1",
		group.Uuid:                            "5",
	}).Err())

	require.NoError(t, repo.ReconcileUserGroupsCache(ctx, "user-1"))

	assert.Equal(t, []string{repository.UserGroupsEmptyValue}, client.ZRange(
		ctx,
		rediskey.UserGroupListKey("user-1"),
		0,
		-1,
	).Val())
	versions := client.HGetAll(ctx, rediskey.UserGroupVersionKey("user-1")).Val()
	assert.Equal(t, "5", versions[group.Uuid])
	assert.Equal(t, "1", versions[repository.UserGroupsReadyField])
}

func TestUserGroupCacheHitReconcileLeaseIsBoundedPerUser(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()

	claimed, err := repo.claimUserGroupsReconcileLease(ctx, "user-1")
	require.NoError(t, err)
	assert.True(t, claimed, "首次缓存命中必须能取得租约并触发语义对账")

	claimed, err = repo.claimUserGroupsReconcileLease(ctx, "user-1")
	require.NoError(t, err)
	assert.False(t, claimed, "租约窗口内的后续请求不能重复压向 MySQL")
	assert.Equal(t, "1", client.Get(ctx, rediskey.UserGroupReconcileLeaseKey("user-1")).Val())
	ttl := client.TTL(ctx, rediskey.UserGroupReconcileLeaseKey("user-1")).Val()
	assert.Positive(t, ttl)
	assert.LessOrEqual(t, ttl, rediskey.UserGroupReconcileLeaseTTL)
}

func TestListUserGroupsReadyHitMakesStrayGroupRepairReachable(t *testing.T) {
	repo, client, db := newProjectorReconcileTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006820123)
	group := &model.GroupInfo{
		Uuid:         "stray-on-hit",
		Name:         "group",
		OwnerUuid:    "owner-1",
		Status:       repository.GroupStatusNormal,
		CacheVersion: 5,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, repo.setVersionedGroupInfoProjection(
		ctx,
		group,
		group.CacheVersion,
		groupInfoCreateIfMissing,
	))
	// user-1 在 DB 从未有成员关系，但这份缓存的 schema、READY、版本和群资料都合法，
	// 所以结构校验不会把它降为 miss；命中路径本身必须提供用户级自愈入口。
	require.NoError(t, repo.reconcileUserGroupProjection(ctx, "user-1", []userGroupReconcileTarget{{
		group:  group,
		active: true,
	}}))

	references, hit, err := repository.ReadVersionedUserGroups(ctx, client, "user-1", false)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, references, 1)
	assert.Equal(t, group.Uuid, references[0].GroupUUID)
	repo.ScheduleUserGroupsCacheAuditAfterHit(ctx, "user-1")
	assert.Equal(t, "1", client.Get(
		ctx,
		rediskey.UserGroupReconcileLeaseKey("user-1"),
	).Val(), "READY 命中也必须取得租约并把权威对账调度为可达")
}

func TestLoadUserGroupReconcileTargetsLocksGroupsBeforeAuthoritativeMembershipRead(t *testing.T) {
	repo, _, db := newProjectorReconcileTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006900123)
	group := &model.GroupInfo{
		Uuid:         "locked-group",
		Name:         "group",
		OwnerUuid:    "user-1",
		Status:       repository.GroupStatusNormal,
		CacheVersion: 7,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&model.GroupMember{
		GroupUuid: group.Uuid,
		UserUuid:  "user-1",
		Role:      repository.MemberRoleOwner,
		Status:    repository.MemberStatusNormal,
		JoinedAt:  now,
	}).Error)

	// 候选发现查询只返回 []string，不参与顺序断言。这里精确记录真正装载
	// GroupInfo 与 GroupMember 快照的两个查询，防止未来重构重新引入
	// “先读旧 membership、后读新 cache_version”的跨事务拼接。
	queryOrder := make([]string, 0, 2)
	require.NoError(t, db.Callback().Query().
		Before("gorm:query").
		Register("test:user_group_reconcile_lock_order", func(tx *gorm.DB) {
			switch tx.Statement.Dest.(type) {
			case *[]*model.GroupInfo:
				queryOrder = append(queryOrder, "groups")
			case *[]*model.GroupMember:
				queryOrder = append(queryOrder, "memberships")
			}
		}))

	targets, err := repo.loadUserGroupReconcileTargets(ctx, "user-1", nil)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.True(t, targets[0].active)
	assert.Equal(t, int64(7), targets[0].group.CacheVersion)
	assert.Equal(t, []string{"groups", "memberships"}, queryOrder)
}

func TestUserReadReconcileCannotUndoNewerRemoval(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	now := time.UnixMilli(1710007000123)
	group := &model.GroupInfo{
		Uuid:         "group-1",
		UpdatedAt:    now,
		CacheVersion: 12,
	}
	require.NoError(t, repo.patchUserGroupProjection(ctx, "user-1", group, false, 12, false))

	stale := *group
	stale.CacheVersion = 11
	require.NoError(t, repo.reconcileUserGroupProjection(ctx, "user-1", []userGroupReconcileTarget{{
		group:  &stale,
		active: true,
	}}))

	assert.Equal(t, []string{repository.UserGroupsEmptyValue}, client.ZRange(ctx, rediskey.UserGroupListKey("user-1"), 0, -1).Val())
	versions := client.HGetAll(ctx, rediskey.UserGroupVersionKey("user-1")).Val()
	assert.Equal(t, "12", versions["group-1"])
	assert.Equal(t, "1", versions[repository.UserGroupsReadyField])
}

func TestValidateGroupCachePayloadRejectsCompatibilityFallbacks(t *testing.T) {
	now := time.UnixMilli(1710008000123)
	valid := projectorPayload(1, event.ActionMemberAdded, projectorGroupSnapshot("group-1", "group", 1, 0, now))
	valid.Members = []event.GroupMemberSnapshot{projectorMemberSnapshot("user-1", repository.MemberRoleMember, now)}
	valid.UserUUIDs = []string{"user-1"}
	require.NoError(t, validateGroupCacheEventPayload(valid))

	missingVersion := valid
	missingVersion.ProjectionVersion = 0
	assert.Error(t, validateGroupCacheEventPayload(missingVersion))

	// v1 曾允许从 members 推导 user_uuids；v2 必须显式提供且集合完全一致。
	missingExplicitUsers := valid
	missingExplicitUsers.UserUUIDs = nil
	assert.Error(t, validateGroupCacheEventPayload(missingExplicitUsers))

	mismatchedUsers := valid
	mismatchedUsers.UserUUIDs = []string{"other-user"}
	assert.Error(t, validateGroupCacheEventPayload(mismatchedUsers))

	dismissedWithNormalStatus := projectorPayload(
		2,
		event.ActionGroupDismissed,
		projectorGroupSnapshot("group-1", "group", 1, repository.GroupStatusNormal, now),
	)
	dismissedWithNormalStatus.UserUUIDs = []string{"user-1"}
	assert.Error(t, validateGroupCacheEventPayload(dismissedWithNormalStatus),
		"group_dismissed 不得携带仍为 normal 的矛盾终态")

	invalidTransfer := projectorPayload(
		3,
		event.ActionOwnerTransferred,
		projectorGroupSnapshot("group-1", "group", 2, repository.GroupStatusNormal, now),
	)
	invalidTransfer.Group.OwnerUUID = "new-owner"
	invalidTransfer.Members = []event.GroupMemberSnapshot{
		projectorMemberSnapshot("old-owner", repository.MemberRoleAdmin, now),
		projectorMemberSnapshot("new-owner", repository.MemberRoleOwner, now),
	}
	invalidTransfer.UserUUIDs = []string{"old-owner", "new-owner"}
	assert.Error(t, validateGroupCacheEventPayload(invalidTransfer),
		"旧群主必须按领域规则降为普通成员，不能接受任意角色快照")
}
