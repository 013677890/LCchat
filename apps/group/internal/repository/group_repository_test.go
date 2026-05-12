package repository

import (
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestCanRemoveGroupMemberRoleMatrix(t *testing.T) {
	cases := []struct {
		name         string
		operatorRole int8
		targetRole   int8
		want         bool
	}{
		{name: "群主可移除管理员", operatorRole: memberRoleOwner, targetRole: memberRoleAdmin, want: true},
		{name: "群主可移除普通成员", operatorRole: memberRoleOwner, targetRole: memberRoleMember, want: true},
		{name: "群主不能移除群主", operatorRole: memberRoleOwner, targetRole: memberRoleOwner, want: false},
		{name: "管理员可移除普通成员", operatorRole: memberRoleAdmin, targetRole: memberRoleMember, want: true},
		{name: "管理员不能移除管理员", operatorRole: memberRoleAdmin, targetRole: memberRoleAdmin, want: false},
		{name: "管理员不能移除群主", operatorRole: memberRoleAdmin, targetRole: memberRoleOwner, want: false},
		{name: "普通成员不能移除普通成员", operatorRole: memberRoleMember, targetRole: memberRoleMember, want: false},
		{name: "未知角色无权限", operatorRole: -1, targetRole: memberRoleMember, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canRemoveGroupMember(tc.operatorRole, tc.targetRole))
		})
	}
}

func TestIsActiveGroupMember(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		member *model.GroupMember
		want   bool
	}{
		{name: "空成员", member: nil, want: false},
		{name: "正常成员", member: &model.GroupMember{Status: memberStatusNormal}, want: true},
		{name: "已退出", member: &model.GroupMember{Status: memberStatusQuit}, want: false},
		{name: "已踢出", member: &model.GroupMember{Status: memberStatusKicked}, want: false},
		{name: "软删成员", member: &model.GroupMember{Status: memberStatusNormal, DeletedAt: deletedAt(now)}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isActiveGroupMember(tc.member))
		})
	}
}

func TestBuildGroupSnapshotRoundTrip(t *testing.T) {
	updatedAt := time.Unix(1710000000, 0)
	group := &model.GroupInfo{
		Uuid:      "group-1",
		Name:      "测试群",
		Avatar:    "avatar.png",
		OwnerUuid: "owner-1",
		MemberCnt: 3,
		Status:    groupStatusNormal,
		UpdatedAt: updatedAt,
	}

	// 事件快照需要完整承接缓存投影所需字段，回放后应能还原出一致的群资料视图。
	snapshot := buildGroupSnapshot(group)
	restored := buildGroupInfoFromSnapshot(snapshot)

	if assert.NotNil(t, snapshot) && assert.NotNil(t, restored) {
		assert.Equal(t, group.Uuid, snapshot.GroupUUID)
		assert.Equal(t, group.MemberCnt, int(snapshot.MemberCount))
		assert.Equal(t, group.UpdatedAt.Unix(), snapshot.UpdatedAtUnix)

		assert.Equal(t, group.Uuid, restored.Uuid)
		assert.Equal(t, group.Name, restored.Name)
		assert.Equal(t, group.Avatar, restored.Avatar)
		assert.Equal(t, group.OwnerUuid, restored.OwnerUuid)
		assert.Equal(t, group.MemberCnt, restored.MemberCnt)
		assert.Equal(t, group.Status, restored.Status)
		assert.True(t, group.UpdatedAt.Equal(restored.UpdatedAt))
	}
}

func TestBuildGroupMemberSnapshotsDeduplicate(t *testing.T) {
	joinedAt := time.UnixMilli(1710000000123)
	members := []*model.GroupMember{
		{GroupUuid: "group-1", UserUuid: "user-1", Role: memberRoleAdmin, JoinedAt: joinedAt},
		nil,
		{GroupUuid: "group-1", UserUuid: "user-1", Role: memberRoleMember, JoinedAt: joinedAt.Add(time.Minute)},
		{GroupUuid: "group-1", UserUuid: "user-2", Role: memberRoleMember, JoinedAt: joinedAt.Add(2 * time.Minute)},
	}

	// 写链路合并“新增成员 + 恢复成员”时可能出现重复 UUID，mapper 必须保证先到的有效快照只保留一次。
	snapshots := buildGroupMemberSnapshots(members)
	userUUIDs := collectGroupMemberSnapshotUUIDs(members)

	if assert.Len(t, snapshots, 2) {
		assert.Equal(t, "user-1", snapshots[0].UserUUID)
		assert.Equal(t, int32(memberRoleAdmin), snapshots[0].Role)
		assert.Equal(t, joinedAt.UnixMilli(), snapshots[0].JoinedAtUnixMs)

		assert.Equal(t, "user-2", snapshots[1].UserUUID)
		assert.Equal(t, int32(memberRoleMember), snapshots[1].Role)
	}
	assert.Equal(t, []string{"user-1", "user-2"}, userUUIDs)
}

func TestValidateGroupCacheEventPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload groupevent.GroupCacheEventPayload
		wantErr bool
	}{
		{
			name:    "缺少基础字段",
			payload: groupevent.GroupCacheEventPayload{},
			wantErr: true,
		},
		{
			name: "建群事件缺少群快照",
			payload: groupevent.GroupCacheEventPayload{
				EventID:   "evt-1",
				Action:    groupevent.ActionGroupCreated,
				GroupUUID: "group-1",
			},
			wantErr: true,
		},
		{
			name: "移除成员事件最小载荷合法",
			payload: groupevent.GroupCacheEventPayload{
				EventID:   "evt-2",
				Action:    groupevent.ActionMemberRemoved,
				GroupUUID: "group-1",
				UserUUID:  "user-1",
				Group: &groupevent.GroupSnapshot{
					GroupUUID:   "group-1",
					MemberCount: 2,
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGroupCacheEventPayload(tc.payload)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestCollectProjectedUserUUIDs(t *testing.T) {
	payloadWithExplicitUsers := groupevent.GroupCacheEventPayload{
		UserUUIDs: []string{"user-1", "user-2"},
		Members: []groupevent.GroupMemberSnapshot{
			{UserUUID: "ignored-user"},
		},
	}
	assert.Equal(t, []string{"user-1", "user-2"}, collectProjectedUserUUIDs(payloadWithExplicitUsers))

	payloadWithMembers := groupevent.GroupCacheEventPayload{
		Members: []groupevent.GroupMemberSnapshot{
			{UserUUID: "user-1"},
			{UserUUID: "user-1"},
			{UserUUID: "user-2"},
		},
	}

	// 当事件没有显式 user_uuids 时，projector 需要从成员快照中回退提取并去重，
	// 这样新增成员和恢复成员都能稳定更新 user_groups 反向索引。
	assert.Equal(t, []string{"user-1", "user-2"}, collectProjectedUserUUIDs(payloadWithMembers))
}

func deletedAt(t time.Time) gorm.DeletedAt {
	return gorm.DeletedAt{Time: t, Valid: true}
}
