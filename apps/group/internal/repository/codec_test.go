package repository

import (
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortGroupInfosMatchesDBOrder(t *testing.T) {
	base := time.UnixMilli(1710000000123)
	groups := []*model.GroupInfo{
		{Id: 1, Uuid: "group-a", UpdatedAt: base},
		{Id: 3, Uuid: "group-c", UpdatedAt: base},
		{Id: 2, Uuid: "group-b", UpdatedAt: base.Add(time.Millisecond)},
	}

	SortGroupInfos(groups)

	require.Len(t, groups, 3)
	assert.Equal(t, "group-b", groups[0].Uuid)
	assert.Equal(t, "group-c", groups[1].Uuid)
	assert.Equal(t, "group-a", groups[2].Uuid)
}

func TestBuildGroupJoinRequestFromCacheRoundTrip(t *testing.T) {
	createdAt := time.Unix(1710000100, 0)
	request := &model.GroupJoinRequest{
		Id:            9,
		ApplicantUuid: "user-9",
		Reason:        "想进群",
		Status:        JoinRequestStatusPending,
		CreatedAt:     createdAt,
	}

	encoded, err := EncodeGroupJoinRequestCacheValue(request)
	require.NoError(t, err)
	entry, err := DecodeGroupJoinRequestCacheValue(encoded)
	require.NoError(t, err)
	restored := BuildGroupJoinRequestFromCache(entry)
	require.NotNil(t, restored)
	assert.Equal(t, request.Id, restored.Id)
	assert.Equal(t, request.ApplicantUuid, restored.ApplicantUuid)
	assert.Equal(t, request.Reason, restored.Reason)
	assert.True(t, request.CreatedAt.Equal(restored.CreatedAt))
}

func TestEncodeGroupMemberCacheValueRejectsInvalid(t *testing.T) {
	_, err := EncodeGroupMemberCacheValue(nil)
	assert.Error(t, err)

	_, err = EncodeGroupMemberCacheValue(&model.GroupMember{
		UserUuid: "member-1",
		Role:     MemberRoleMember,
	})
	assert.Error(t, err)
}

func TestEncodeGroupJoinRequestCacheValueRejectsInvalid(t *testing.T) {
	_, err := EncodeGroupJoinRequestCacheValue(nil)
	assert.Error(t, err)

	_, err = EncodeGroupJoinRequestCacheValue(&model.GroupJoinRequest{
		Id:            1,
		ApplicantUuid: "",
		CreatedAt:     time.Unix(1710000000, 0),
	})
	assert.Error(t, err)
}

func TestEncodeGroupMemberCacheValueRoundTrip(t *testing.T) {
	joinedAt := time.UnixMilli(1710000000123)
	member := &model.GroupMember{
		UserUuid: "member-1",
		Role:     MemberRoleAdmin,
		Remark:   "管理员",
		JoinedAt: joinedAt,
	}

	encoded, err := EncodeGroupMemberCacheValue(member)
	require.NoError(t, err)
	entry, err := DecodeGroupMemberCacheValue(encoded)
	require.NoError(t, err)
	restored := BuildGroupMemberFromCache(member.UserUuid, entry)
	require.NotNil(t, restored)
	assert.Equal(t, member.UserUuid, restored.UserUuid)
	assert.Equal(t, member.Role, restored.Role)
	assert.Equal(t, member.Remark, restored.Remark)
	assert.True(t, member.JoinedAt.Equal(restored.JoinedAt))
	assert.Nil(t, restored.MuteUntil)
}

func TestSortGroupJoinRequests(t *testing.T) {
	older := time.Unix(1710000000, 0)
	newer := older.Add(time.Minute)
	items := []*model.GroupJoinRequest{
		{Id: 1, ApplicantUuid: "u1", CreatedAt: older},
		{Id: 3, ApplicantUuid: "u3", CreatedAt: newer},
		{Id: 2, ApplicantUuid: "u2", CreatedAt: newer},
	}

	SortGroupJoinRequests(items)
	require.Len(t, items, 3)
	assert.Equal(t, int64(3), items[0].Id)
	assert.Equal(t, int64(2), items[1].Id)
	assert.Equal(t, int64(1), items[2].Id)
}
