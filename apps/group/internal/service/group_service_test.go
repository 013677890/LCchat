package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGroupRepoForService struct {
	createGroupFn     func(context.Context, *model.GroupInfo, []*model.GroupMember) error
	addMembersFn      func(context.Context, string, string, []*model.GroupMember) error
	removeMemberFn    func(context.Context, string, string, string) error
	dismissGroupFn    func(context.Context, string, string) error
	updateGroupInfoFn func(context.Context, string, string, *string, *string) error
	getGroupInfoFn    func(context.Context, string) (*model.GroupInfo, error)
	getGroupMembersFn func(context.Context, string) ([]*model.GroupMember, error)
	checkMemberFn     func(context.Context, string, string) (bool, int8, error)
	listUserGroupsFn  func(context.Context, string) ([]*model.GroupInfo, error)
	getUserProfilesFn func(context.Context, []string) (map[string]*model.UserProfile, error)
}

func (f *fakeGroupRepoForService) CreateGroup(ctx context.Context, group *model.GroupInfo, members []*model.GroupMember) error {
	if f.createGroupFn == nil {
		return nil
	}
	return f.createGroupFn(ctx, group, members)
}

func (f *fakeGroupRepoForService) AddMembers(ctx context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error {
	if f.addMembersFn == nil {
		return nil
	}
	return f.addMembersFn(ctx, groupUUID, operatorUUID, members)
}

func (f *fakeGroupRepoForService) RemoveMember(ctx context.Context, groupUUID, operatorUUID, targetUUID string) error {
	if f.removeMemberFn == nil {
		return nil
	}
	return f.removeMemberFn(ctx, groupUUID, operatorUUID, targetUUID)
}

func (f *fakeGroupRepoForService) DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	if f.dismissGroupFn == nil {
		return nil
	}
	return f.dismissGroupFn(ctx, groupUUID, operatorUUID)
}

func (f *fakeGroupRepoForService) UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, name, avatar *string) error {
	if f.updateGroupInfoFn == nil {
		return nil
	}
	return f.updateGroupInfoFn(ctx, groupUUID, operatorUUID, name, avatar)
}

func (f *fakeGroupRepoForService) GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if f.getGroupInfoFn == nil {
		return nil, repository.ErrRecordNotFound
	}
	return f.getGroupInfoFn(ctx, groupUUID)
}

func (f *fakeGroupRepoForService) GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	if f.getGroupMembersFn == nil {
		return nil, nil
	}
	return f.getGroupMembersFn(ctx, groupUUID)
}

func (f *fakeGroupRepoForService) CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error) {
	if f.checkMemberFn == nil {
		return false, -1, nil
	}
	return f.checkMemberFn(ctx, groupUUID, userUUID)
}

func (f *fakeGroupRepoForService) ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	if f.listUserGroupsFn == nil {
		return nil, nil
	}
	return f.listUserGroupsFn(ctx, userUUID)
}

func (f *fakeGroupRepoForService) GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
	if f.getUserProfilesFn == nil {
		return map[string]*model.UserProfile{}, nil
	}
	return f.getUserProfilesFn(ctx, userUUIDs)
}

func groupServiceTestContext(userUUID string) context.Context {
	return ctxmeta.WithUserUUID(context.Background(), userUUID)
}

func requireGroupBizCode(t *testing.T, err error, wantCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantCode, apperr.Code(err))
}

func TestCreateGroupBuildsGroupAndInitialMembers(t *testing.T) {
	var gotGroup *model.GroupInfo
	var gotMembers []*model.GroupMember
	repo := &fakeGroupRepoForService{
		createGroupFn: func(_ context.Context, group *model.GroupInfo, members []*model.GroupMember) error {
			copied := *group
			gotGroup = &copied
			gotMembers = append([]*model.GroupMember(nil), members...)
			return nil
		},
	}
	svc := NewGroupService(repo)

	resp, err := svc.CreateGroup(groupServiceTestContext("owner-uuid"), &pb.CreateGroupRequest{
		Name:        "  群组名称  ",
		MemberUuids: []string{" member-a ", "owner-uuid", "member-a", "", "member-b"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, gotGroup)
	require.Len(t, gotMembers, 3)
	assert.NotEmpty(t, resp.GetGroupUuid())
	assert.Equal(t, resp.GetGroupUuid(), gotGroup.Uuid)
	assert.Equal(t, "群组名称", gotGroup.Name)
	assert.Equal(t, defaultGroupAvatarURL, gotGroup.Avatar)
	assert.Equal(t, "owner-uuid", gotGroup.OwnerUuid)
	assert.Equal(t, len(gotMembers), gotGroup.MemberCnt)
	assert.Equal(t, int8(0), gotGroup.Status)

	assert.Equal(t, "owner-uuid", gotMembers[0].UserUuid)
	assert.Equal(t, int8(2), gotMembers[0].Role)
	assert.Equal(t, gotGroup.Uuid, gotMembers[0].GroupUuid)
	assert.False(t, gotMembers[0].JoinedAt.IsZero())

	assert.Equal(t, "member-a", gotMembers[1].UserUuid)
	assert.Equal(t, "owner-uuid", gotMembers[1].Inviter)
	assert.Equal(t, int8(0), gotMembers[1].Role)
	assert.Equal(t, gotGroup.Uuid, gotMembers[1].GroupUuid)
	assert.Equal(t, "member-b", gotMembers[2].UserUuid)
}

func TestCreateGroupValidation(t *testing.T) {
	svc := NewGroupService(&fakeGroupRepoForService{})
	cases := []struct {
		name string
		ctx  context.Context
		req  *pb.CreateGroupRequest
		want int
	}{
		{name: "未登录", ctx: context.Background(), req: &pb.CreateGroupRequest{Name: "群"}, want: consts.CodeUnauthorized},
		{name: "空请求", ctx: groupServiceTestContext("owner"), req: nil, want: consts.CodeParamError},
		{name: "空名称", ctx: groupServiceTestContext("owner"), req: &pb.CreateGroupRequest{Name: "   "}, want: consts.CodeParamError},
		{name: "名称过长", ctx: groupServiceTestContext("owner"), req: &pb.CreateGroupRequest{Name: strings.Repeat("群", 65)}, want: consts.CodeGroupNameTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateGroup(tc.ctx, tc.req)
			requireGroupBizCode(t, err, tc.want)
		})
	}
}

func TestAddMemberNormalizesInputAndCallsRepository(t *testing.T) {
	var gotGroupUUID string
	var gotOperatorUUID string
	var gotMembers []*model.GroupMember
	repo := &fakeGroupRepoForService{
		addMembersFn: func(_ context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error {
			gotGroupUUID = groupUUID
			gotOperatorUUID = operatorUUID
			gotMembers = append([]*model.GroupMember(nil), members...)
			return nil
		},
	}
	svc := NewGroupService(repo)

	err := svc.AddMember(groupServiceTestContext("admin-uuid"), &pb.AddMemberRequest{
		GroupUuid: " group-uuid ",
		UserUuids: []string{" member-a ", "member-a", "", "member-b"},
	})

	require.NoError(t, err)
	assert.Equal(t, "group-uuid", gotGroupUUID)
	assert.Equal(t, "admin-uuid", gotOperatorUUID)
	require.Len(t, gotMembers, 2)
	assert.Equal(t, "member-a", gotMembers[0].UserUuid)
	assert.Equal(t, "group-uuid", gotMembers[0].GroupUuid)
	assert.Equal(t, "member-b", gotMembers[1].UserUuid)
}

func TestAddMemberSkipsRepositoryWhenNormalizedMembersEmpty(t *testing.T) {
	called := false
	repo := &fakeGroupRepoForService{
		addMembersFn: func(context.Context, string, string, []*model.GroupMember) error {
			called = true
			return nil
		},
	}
	svc := NewGroupService(repo)

	err := svc.AddMember(groupServiceTestContext("admin-uuid"), &pb.AddMemberRequest{
		GroupUuid: "group-uuid",
		UserUuids: []string{" ", ""},
	})

	require.NoError(t, err)
	assert.False(t, called)
}

func TestUpdateGroupInfoParsesFieldsAndSkipsNoop(t *testing.T) {
	var called int
	var gotName *string
	var gotAvatar *string
	repo := &fakeGroupRepoForService{
		updateGroupInfoFn: func(_ context.Context, _, _ string, name, avatar *string) error {
			called++
			gotName = name
			gotAvatar = avatar
			return nil
		},
	}
	svc := NewGroupService(repo)

	err := svc.UpdateGroupInfo(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupInfoRequest{
		GroupUuid: " group-uuid ",
		Name:      "  新群名  ",
		Avatar:    "  https://example.com/a.png  ",
	})

	require.NoError(t, err)
	require.NotNil(t, gotName)
	require.NotNil(t, gotAvatar)
	assert.Equal(t, 1, called)
	assert.Equal(t, "新群名", *gotName)
	assert.Equal(t, "https://example.com/a.png", *gotAvatar)

	err = svc.UpdateGroupInfo(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupInfoRequest{GroupUuid: "group-uuid"})
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestUpdateGroupInfoRejectsBlankName(t *testing.T) {
	svc := NewGroupService(&fakeGroupRepoForService{})

	err := svc.UpdateGroupInfo(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupInfoRequest{
		GroupUuid: "group-uuid",
		Name:      "   ",
	})

	requireGroupBizCode(t, err, consts.CodeParamError)
}

func TestRemoveAndDismissPassOperatorFromContext(t *testing.T) {
	var removeArgs []string
	var dismissArgs []string
	repo := &fakeGroupRepoForService{
		removeMemberFn: func(_ context.Context, groupUUID, operatorUUID, targetUUID string) error {
			removeArgs = []string{groupUUID, operatorUUID, targetUUID}
			return nil
		},
		dismissGroupFn: func(_ context.Context, groupUUID, operatorUUID string) error {
			dismissArgs = []string{groupUUID, operatorUUID}
			return nil
		},
	}
	svc := NewGroupService(repo)

	err := svc.RemoveMember(groupServiceTestContext("operator-uuid"), &pb.RemoveMemberRequest{GroupUuid: " group-uuid ", UserUuid: " target-uuid "})
	require.NoError(t, err)
	assert.Equal(t, []string{"group-uuid", "operator-uuid", "target-uuid"}, removeArgs)

	err = svc.DismissGroup(groupServiceTestContext("owner-uuid"), &pb.DismissGroupRequest{GroupUuid: " group-uuid "})
	require.NoError(t, err)
	assert.Equal(t, []string{"group-uuid", "owner-uuid"}, dismissArgs)
}

func TestGroupWriteErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "群已解散", err: repository.ErrGroupDismissed, want: consts.CodeGroupAlreadyDismiss},
		{name: "群不存在", err: repository.ErrRecordNotFound, want: consts.CodeGroupNotFound},
		{name: "无权限", err: repository.ErrNoPermission, want: consts.CodeNoPermission},
		{name: "不能踢群主", err: repository.ErrCannotKickOwner, want: consts.CodeCannotKickOwner},
		{name: "群主不能退群", err: repository.ErrCannotQuitAsOwner, want: consts.CodeCannotQuitAsOwner},
		{name: "未知错误", err: errors.New("db down"), want: consts.CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapGroupWriteError(tc.err, "写群失败")
			requireGroupBizCode(t, err, tc.want)
		})
	}
}

func TestCheckGroupMemberMapsDismissedAndNormalizesNonMemberRole(t *testing.T) {
	svc := NewGroupService(&fakeGroupRepoForService{
		checkMemberFn: func(context.Context, string, string) (bool, int8, error) {
			return false, 0, nil
		},
	})

	resp, err := svc.CheckGroupMember(context.Background(), &pb.CheckGroupMemberRequest{GroupUuid: " group-uuid ", UserUuid: " user-uuid "})
	require.NoError(t, err)
	assert.False(t, resp.GetIsMember())
	assert.Equal(t, int32(-1), resp.GetRole())

	svc = NewGroupService(&fakeGroupRepoForService{
		checkMemberFn: func(context.Context, string, string) (bool, int8, error) {
			return false, -1, repository.ErrGroupDismissed
		},
	})
	_, err = svc.CheckGroupMember(context.Background(), &pb.CheckGroupMemberRequest{GroupUuid: "group-uuid", UserUuid: "user-uuid"})
	requireGroupBizCode(t, err, consts.CodeGroupAlreadyDismiss)
}
