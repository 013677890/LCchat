package repository

import (
	"context"
	"testing"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newProjectorTestRepository 创建带 miniredis 的 projector 仓储实例。
func newProjectorTestRepository(t *testing.T) (*groupRepositoryImpl, *goredis.Client) {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	return &groupRepositoryImpl{redisClient: client}, client
}

// seedUserGroupsCache 预建 user_groups key，便于验证 patch-if-exists 语义。
func seedUserGroupsCache(t *testing.T, client *goredis.Client, userUUID string) {
	t.Helper()
	require.NoError(t, client.ZAdd(context.Background(), rediskey.UserGroupListKey(userUUID), goredis.Z{
		Score:  0,
		Member: userGroupsEmptyValue,
	}).Err())
}

// seedGroupMembersCache 预建成员 Hash，便于验证增量 patch 行为。
func seedGroupMembersCache(t *testing.T, repo *groupRepositoryImpl, groupUUID string, members ...*model.GroupMember) {
	t.Helper()
	require.NoError(t, repo.rebuildGroupMembersCache(context.Background(), groupUUID, members))
}

// mustReadGroupInfoEntry 读取并解析 group:info 主缓存。
func mustReadGroupInfoEntry(t *testing.T, client *goredis.Client, groupUUID string) *groupInfoCacheEntry {
	t.Helper()
	raw, err := client.Get(context.Background(), rediskey.GroupInfoKey(groupUUID)).Result()
	require.NoError(t, err)
	entry, err := decodeGroupInfoCacheValue(raw)
	require.NoError(t, err)
	return entry
}

// mustReadGroupMemberFields 读取群成员 Hash。
func mustReadGroupMemberFields(t *testing.T, client *goredis.Client, groupUUID string) map[string]string {
	t.Helper()
	fields, err := client.HGetAll(context.Background(), rediskey.GroupMembersKey(groupUUID)).Result()
	require.NoError(t, err)
	return fields
}

// mustReadUserGroups 读取用户群列表缓存。
func mustReadUserGroups(t *testing.T, client *goredis.Client, userUUID string) []string {
	t.Helper()
	values, err := client.ZRevRange(context.Background(), rediskey.UserGroupListKey(userUUID), 0, -1).Result()
	require.NoError(t, err)
	return values
}

// buildProjectorGroupSnapshot 构造测试用群快照。
func buildProjectorGroupSnapshot(groupUUID string, memberCount int32, status int32, updatedAt time.Time) *groupevent.GroupSnapshot {
	return &groupevent.GroupSnapshot{
		GroupUUID:     groupUUID,
		Name:          "测试群",
		Avatar:        "avatar.png",
		OwnerUUID:     "owner-1",
		MemberCount:   memberCount,
		Status:        status,
		UpdatedAtUnix: updatedAt.Unix(),
	}
}

// buildProjectorMemberSnapshot 构造测试用成员快照。
func buildProjectorMemberSnapshot(userUUID string, role int32, joinedAt time.Time) groupevent.GroupMemberSnapshot {
	return groupevent.GroupMemberSnapshot{
		UserUUID:       userUUID,
		Role:           role,
		JoinedAtUnixMs: joinedAt.UnixMilli(),
	}
}

// TestApplyGroupCacheEventGroupCreated 构造建群后的首次缓存投影闭环。
func TestApplyGroupCacheEventGroupCreated(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	updatedAt := time.Unix(1710000000, 0)
	joinedAt := updatedAt.Add(-time.Minute)

	seedUserGroupsCache(t, client, "owner-1")
	seedUserGroupsCache(t, client, "member-1")

	err := repo.ApplyGroupCacheEvent(ctx, groupevent.GroupCacheEventPayload{
		EventID:   "evt-created",
		Action:    groupevent.ActionGroupCreated,
		GroupUUID: "group-1",
		Group:     buildProjectorGroupSnapshot("group-1", 2, int32(groupStatusNormal), updatedAt),
		Members: []groupevent.GroupMemberSnapshot{
			buildProjectorMemberSnapshot("owner-1", int32(memberRoleOwner), joinedAt),
			buildProjectorMemberSnapshot("member-1", int32(memberRoleMember), joinedAt.Add(time.Second)),
		},
		UserUUIDs: []string{"owner-1", "member-1"},
	})
	require.NoError(t, err)

	info := mustReadGroupInfoEntry(t, client, "group-1")
	assert.Equal(t, "group-1", info.GroupUUID)
	assert.Equal(t, 2, info.MemberCount)
	assert.Equal(t, groupStatusNormal, info.Status)

	fields := mustReadGroupMemberFields(t, client, "group-1")
	require.Len(t, fields, 2)
	ownerEntry, err := decodeGroupMemberCacheValue(fields["owner-1"])
	require.NoError(t, err)
	assert.Equal(t, memberRoleOwner, ownerEntry.Role)

	assert.Equal(t, []string{"group-1"}, mustReadUserGroups(t, client, "owner-1"))
	assert.Equal(t, []string{"group-1"}, mustReadUserGroups(t, client, "member-1"))
}

// TestApplyGroupCacheEventMemberAddedOnlyPatchesExistingKeys 验证增量事件不会反向建缓存。
func TestApplyGroupCacheEventMemberAddedOnlyPatchesExistingKeys(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	updatedAt := time.Unix(1710001000, 0)

	require.NoError(t, repo.setGroupInfoCache(ctx, &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "旧群名",
		Avatar:    "old.png",
		OwnerUuid: "owner-1",
		MemberCnt: 1,
		Status:    groupStatusNormal,
		UpdatedAt: updatedAt.Add(-time.Minute),
	}))
	seedGroupMembersCache(t, repo, "group-1", &model.GroupMember{
		GroupUuid: "group-1",
		UserUuid:  "owner-1",
		Role:      memberRoleOwner,
		JoinedAt:  updatedAt.Add(-2 * time.Minute),
	})
	seedUserGroupsCache(t, client, "member-1")

	err := repo.ApplyGroupCacheEvent(ctx, groupevent.GroupCacheEventPayload{
		EventID:   "evt-member-added",
		Action:    groupevent.ActionMemberAdded,
		GroupUUID: "group-1",
		Group:     buildProjectorGroupSnapshot("group-1", 3, int32(groupStatusNormal), updatedAt),
		Members: []groupevent.GroupMemberSnapshot{
			buildProjectorMemberSnapshot("member-1", int32(memberRoleMember), updatedAt.Add(-time.Second)),
			buildProjectorMemberSnapshot("member-2", int32(memberRoleMember), updatedAt),
		},
		UserUUIDs: []string{"member-1", "member-2"},
	})
	require.NoError(t, err)

	info := mustReadGroupInfoEntry(t, client, "group-1")
	assert.Equal(t, 3, info.MemberCount)

	fields := mustReadGroupMemberFields(t, client, "group-1")
	assert.Contains(t, fields, "member-1")
	assert.Contains(t, fields, "member-2")
	assert.Equal(t, []string{"group-1"}, mustReadUserGroups(t, client, "member-1"))

	exists, err := client.Exists(ctx, rediskey.UserGroupListKey("member-2")).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
}

// TestApplyGroupCacheEventMemberRemovedRemovesFieldAndReverseIndex 验证删成员后的缓存投影。
func TestApplyGroupCacheEventMemberRemovedRemovesFieldAndReverseIndex(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	updatedAt := time.Unix(1710002000, 0)

	require.NoError(t, repo.setGroupInfoCache(ctx, &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "测试群",
		Avatar:    "avatar.png",
		OwnerUuid: "owner-1",
		MemberCnt: 1,
		Status:    groupStatusNormal,
		UpdatedAt: updatedAt.Add(-time.Minute),
	}))
	seedGroupMembersCache(t, repo, "group-1", &model.GroupMember{
		GroupUuid: "group-1",
		UserUuid:  "member-1",
		Role:      memberRoleMember,
		JoinedAt:  updatedAt.Add(-time.Minute),
	})
	seedUserGroupsCache(t, client, "member-1")
	require.NoError(t, client.ZAdd(ctx, rediskey.UserGroupListKey("member-1"), goredis.Z{
		Score:  float64(updatedAt.Unix()),
		Member: "group-1",
	}).Err())

	err := repo.ApplyGroupCacheEvent(ctx, groupevent.GroupCacheEventPayload{
		EventID:   "evt-member-removed",
		Action:    groupevent.ActionMemberRemoved,
		GroupUUID: "group-1",
		UserUUID:  "member-1",
		Group:     buildProjectorGroupSnapshot("group-1", 0, int32(groupStatusNormal), updatedAt),
	})
	require.NoError(t, err)

	info := mustReadGroupInfoEntry(t, client, "group-1")
	assert.Equal(t, 0, info.MemberCount)

	fields := mustReadGroupMemberFields(t, client, "group-1")
	assert.Equal(t, map[string]string{groupMembersEmptyField: groupMembersEmptyValue}, fields)
	assert.Equal(t, []string{userGroupsEmptyValue}, mustReadUserGroups(t, client, "member-1"))
}

// TestApplyGroupCacheEventGroupDismissedDeletesMembersAndMarksInfo 验证群解散后的强失效行为。
func TestApplyGroupCacheEventGroupDismissedDeletesMembersAndMarksInfo(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	updatedAt := time.Unix(1710003000, 0)

	require.NoError(t, repo.setGroupInfoCache(ctx, &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "测试群",
		Avatar:    "avatar.png",
		OwnerUuid: "owner-1",
		MemberCnt: 2,
		Status:    groupStatusNormal,
		UpdatedAt: updatedAt.Add(-time.Minute),
	}))
	seedGroupMembersCache(t, repo, "group-1",
		&model.GroupMember{GroupUuid: "group-1", UserUuid: "owner-1", Role: memberRoleOwner, JoinedAt: updatedAt.Add(-2 * time.Minute)},
		&model.GroupMember{GroupUuid: "group-1", UserUuid: "member-1", Role: memberRoleMember, JoinedAt: updatedAt.Add(-time.Minute)},
	)
	seedUserGroupsCache(t, client, "owner-1")
	seedUserGroupsCache(t, client, "member-1")
	require.NoError(t, client.ZAdd(ctx, rediskey.UserGroupListKey("owner-1"), goredis.Z{Score: float64(updatedAt.Unix()), Member: "group-1"}).Err())
	require.NoError(t, client.ZAdd(ctx, rediskey.UserGroupListKey("member-1"), goredis.Z{Score: float64(updatedAt.Unix()), Member: "group-1"}).Err())

	err := repo.ApplyGroupCacheEvent(ctx, groupevent.GroupCacheEventPayload{
		EventID:   "evt-dismissed",
		Action:    groupevent.ActionGroupDismissed,
		GroupUUID: "group-1",
		Group:     buildProjectorGroupSnapshot("group-1", 2, int32(groupStatusDismissed), updatedAt),
		UserUUIDs: []string{"owner-1", "member-1"},
	})
	require.NoError(t, err)

	info := mustReadGroupInfoEntry(t, client, "group-1")
	assert.Equal(t, groupStatusDismissed, info.Status)

	exists, err := client.Exists(ctx, rediskey.GroupMembersKey("group-1")).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
	assert.Equal(t, []string{userGroupsEmptyValue}, mustReadUserGroups(t, client, "owner-1"))
	assert.Equal(t, []string{userGroupsEmptyValue}, mustReadUserGroups(t, client, "member-1"))
}

// TestApplyGroupCacheEventGroupInfoUpdatedOnlyTouchesInfo 验证资料更新不会误改成员缓存。
func TestApplyGroupCacheEventGroupInfoUpdatedOnlyTouchesInfo(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	updatedAt := time.Unix(1710004000, 0)

	require.NoError(t, repo.setGroupInfoCache(ctx, &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "旧群名",
		Avatar:    "old.png",
		OwnerUuid: "owner-1",
		MemberCnt: 2,
		Status:    groupStatusNormal,
		UpdatedAt: updatedAt.Add(-time.Minute),
	}))
	seedGroupMembersCache(t, repo, "group-1",
		&model.GroupMember{GroupUuid: "group-1", UserUuid: "owner-1", Role: memberRoleOwner, JoinedAt: updatedAt.Add(-2 * time.Minute)},
		&model.GroupMember{GroupUuid: "group-1", UserUuid: "member-1", Role: memberRoleMember, JoinedAt: updatedAt.Add(-time.Minute)},
	)

	err := repo.ApplyGroupCacheEvent(ctx, groupevent.GroupCacheEventPayload{
		EventID:   "evt-info-updated",
		Action:    groupevent.ActionGroupInfoUpdated,
		GroupUUID: "group-1",
		Group: &groupevent.GroupSnapshot{
			GroupUUID:     "group-1",
			Name:          "新群名",
			Avatar:        "new.png",
			OwnerUUID:     "owner-1",
			MemberCount:   2,
			Status:        int32(groupStatusNormal),
			UpdatedAtUnix: updatedAt.Unix(),
		},
	})
	require.NoError(t, err)

	info := mustReadGroupInfoEntry(t, client, "group-1")
	assert.Equal(t, "新群名", info.Name)
	assert.Equal(t, "new.png", info.Avatar)

	fields := mustReadGroupMemberFields(t, client, "group-1")
	assert.Len(t, fields, 2)
	assert.Contains(t, fields, "owner-1")
	assert.Contains(t, fields, "member-1")
}

// TestApplyGroupCacheEventWrongTypeDeletesDirtyKey 验证投影遇到脏 key 时会清理并降级成功。
func TestApplyGroupCacheEventWrongTypeDeletesDirtyKey(t *testing.T) {
	repo, client := newProjectorTestRepository(t)
	ctx := context.Background()
	updatedAt := time.Unix(1710005000, 0)

	require.NoError(t, repo.setGroupInfoCache(ctx, &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "测试群",
		Avatar:    "avatar.png",
		OwnerUuid: "owner-1",
		MemberCnt: 1,
		Status:    groupStatusNormal,
		UpdatedAt: updatedAt.Add(-time.Minute),
	}))
	require.NoError(t, client.RPush(ctx, rediskey.GroupMembersKey("group-1"), "bad-field").Err())

	err := repo.ApplyGroupCacheEvent(ctx, groupevent.GroupCacheEventPayload{
		EventID:   "evt-member-removed-wrongtype",
		Action:    groupevent.ActionMemberRemoved,
		GroupUUID: "group-1",
		UserUUID:  "member-1",
		Group:     buildProjectorGroupSnapshot("group-1", 0, int32(groupStatusNormal), updatedAt),
	})
	require.NoError(t, err)

	exists, err := client.Exists(ctx, rediskey.GroupMembersKey("group-1")).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
}
