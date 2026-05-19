package service

import (
	"context"
	"errors"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/realtimepb"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
	"time"
)

type fakeGroupRepoForService struct {
	createGroupFn          func(context.Context, *model.GroupInfo, []*model.GroupMember) error
	addMembersFn           func(context.Context, string, string, []*model.GroupMember) error
	removeMemberFn         func(context.Context, string, string, string) error
	leaveGroupFn           func(context.Context, string, string) error
	dismissGroupFn         func(context.Context, string, string) error
	updateGroupInfoFn      func(context.Context, string, string, repository.GroupInfoUpdates) error
	updateNoticeFn         func(context.Context, string, string, string) error
	transferOwnerFn        func(context.Context, string, string, string) error
	updateMemberRoleFn     func(context.Context, string, string, string, int8) error
	searchMembersFn        func(context.Context, string, string, string, int, int) ([]*model.GroupMember, int64, error)
	searchGroupsFn         func(context.Context, string, int, int) ([]*model.GroupInfo, int64, error)
	updateNicknameFn       func(context.Context, string, string, string) error
	updateMemberNicknameFn func(context.Context, string, string, string, string) error
	muteMemberFn           func(context.Context, string, string, string, *time.Time) error
	updateMuteFn           func(context.Context, string, string, bool) error
	applyJoinGroupFn       func(context.Context, string, string, string) (repository.ApplyJoinGroupResult, error)
	cancelJoinApplyFn      func(context.Context, string, string) error
	getMyJoinApplyFn       func(context.Context, string, string) (*model.GroupJoinRequest, error)
	getJoinApplicantFn     func(context.Context, string, int64) (string, error)
	listMyJoinAppsFn       func(context.Context, string, *int8, int, int) ([]*model.GroupJoinRequest, int64, error)
	reviewJoinGroupFn      func(context.Context, string, string, int64, int8, string) error
	listJoinReqsFn         func(context.Context, string, string, int, int) ([]*model.GroupJoinRequest, int64, error)
	listReviewedFn         func(context.Context, string, string, *int8, int, int) ([]*model.GroupJoinRequest, int64, error)
	pendingCountFn         func(context.Context, string, string) (int64, error)
	getGroupInfoFn         func(context.Context, string) (*model.GroupInfo, error)
	getGroupsByUUIDsFn     func(context.Context, []string) (map[string]*model.GroupInfo, error)
	getGroupMembersFn      func(context.Context, string) ([]*model.GroupMember, error)
	checkMemberFn          func(context.Context, string, string) (bool, int8, error)
	checkSendFn            func(context.Context, string, string) (repository.CheckGroupSendPermissionResult, error)
	listUserGroupsFn       func(context.Context, string) ([]*model.GroupInfo, error)
	getUserProfilesFn      func(context.Context, []string) (map[string]*model.UserProfile, error)
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

func (f *fakeGroupRepoForService) LeaveGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	if f.leaveGroupFn == nil {
		return nil
	}
	return f.leaveGroupFn(ctx, groupUUID, operatorUUID)
}

func (f *fakeGroupRepoForService) DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	if f.dismissGroupFn == nil {
		return nil
	}
	return f.dismissGroupFn(ctx, groupUUID, operatorUUID)
}

func (f *fakeGroupRepoForService) UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, updates repository.GroupInfoUpdates) error {
	if f.updateGroupInfoFn == nil {
		return nil
	}
	return f.updateGroupInfoFn(ctx, groupUUID, operatorUUID, updates)
}

func (f *fakeGroupRepoForService) UpdateGroupNotice(ctx context.Context, groupUUID, operatorUUID, notice string) error {
	if f.updateNoticeFn == nil {
		return nil
	}
	return f.updateNoticeFn(ctx, groupUUID, operatorUUID, notice)
}

func (f *fakeGroupRepoForService) TransferGroupOwner(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string) error {
	if f.transferOwnerFn == nil {
		return nil
	}
	return f.transferOwnerFn(ctx, groupUUID, operatorUUID, targetUserUUID)
}

func (f *fakeGroupRepoForService) UpdateMemberRole(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, role int8) error {
	if f.updateMemberRoleFn == nil {
		return nil
	}
	return f.updateMemberRoleFn(ctx, groupUUID, operatorUUID, targetUserUUID, role)
}

func (f *fakeGroupRepoForService) SearchGroupMembers(ctx context.Context, groupUUID, operatorUUID, keyword string, page, pageSize int) ([]*model.GroupMember, int64, error) {
	if f.searchMembersFn == nil {
		return []*model.GroupMember{}, 0, nil
	}
	return f.searchMembersFn(ctx, groupUUID, operatorUUID, keyword, page, pageSize)
}

func (f *fakeGroupRepoForService) SearchGroups(ctx context.Context, keyword string, page, pageSize int) ([]*model.GroupInfo, int64, error) {
	if f.searchGroupsFn == nil {
		return []*model.GroupInfo{}, 0, nil
	}
	return f.searchGroupsFn(ctx, keyword, page, pageSize)
}

func (f *fakeGroupRepoForService) UpdateMyGroupNickname(ctx context.Context, groupUUID, userUUID, nickname string) error {
	if f.updateNicknameFn == nil {
		return nil
	}
	return f.updateNicknameFn(ctx, groupUUID, userUUID, nickname)
}

func (f *fakeGroupRepoForService) UpdateGroupMemberNickname(ctx context.Context, groupUUID, operatorUUID, targetUserUUID, nickname string) error {
	if f.updateMemberNicknameFn == nil {
		return nil
	}
	return f.updateMemberNicknameFn(ctx, groupUUID, operatorUUID, targetUserUUID, nickname)
}

func (f *fakeGroupRepoForService) MuteGroupMember(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, muteUntil *time.Time) error {
	if f.muteMemberFn == nil {
		return nil
	}
	return f.muteMemberFn(ctx, groupUUID, operatorUUID, targetUserUUID, muteUntil)
}

func (f *fakeGroupRepoForService) UpdateGroupMuteSetting(ctx context.Context, groupUUID, operatorUUID string, muteAll bool) error {
	if f.updateMuteFn == nil {
		return nil
	}
	return f.updateMuteFn(ctx, groupUUID, operatorUUID, muteAll)
}

func (f *fakeGroupRepoForService) ApplyJoinGroup(ctx context.Context, groupUUID, applicantUUID, reason string) (repository.ApplyJoinGroupResult, error) {
	if f.applyJoinGroupFn == nil {
		return repository.ApplyJoinGroupResult{}, nil
	}
	return f.applyJoinGroupFn(ctx, groupUUID, applicantUUID, reason)
}

func (f *fakeGroupRepoForService) CancelJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) error {
	if f.cancelJoinApplyFn == nil {
		return nil
	}
	return f.cancelJoinApplyFn(ctx, groupUUID, applicantUUID)
}

func (f *fakeGroupRepoForService) GetMyJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if f.getMyJoinApplyFn == nil {
		return nil, nil
	}
	return f.getMyJoinApplyFn(ctx, groupUUID, applicantUUID)
}

func (f *fakeGroupRepoForService) GetJoinRequestApplicant(ctx context.Context, groupUUID string, applyID int64) (string, error) {
	if f.getJoinApplicantFn == nil {
		return "", nil
	}
	return f.getJoinApplicantFn(ctx, groupUUID, applyID)
}

func (f *fakeGroupRepoForService) ListMyJoinGroupApplications(ctx context.Context, applicantUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if f.listMyJoinAppsFn == nil {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	return f.listMyJoinAppsFn(ctx, applicantUUID, status, page, pageSize)
}

func (f *fakeGroupRepoForService) ReviewJoinGroup(ctx context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error {
	if f.reviewJoinGroupFn == nil {
		return nil
	}
	return f.reviewJoinGroupFn(ctx, groupUUID, operatorUUID, applyID, action, remark)
}

func (f *fakeGroupRepoForService) ListJoinRequests(ctx context.Context, groupUUID, operatorUUID string, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if f.listJoinReqsFn == nil {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	return f.listJoinReqsFn(ctx, groupUUID, operatorUUID, page, pageSize)
}

func (f *fakeGroupRepoForService) ListReviewedJoinRequests(ctx context.Context, groupUUID, operatorUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	if f.listReviewedFn == nil {
		return []*model.GroupJoinRequest{}, 0, nil
	}
	return f.listReviewedFn(ctx, groupUUID, operatorUUID, status, page, pageSize)
}

func (f *fakeGroupRepoForService) GetJoinRequestPendingCount(ctx context.Context, groupUUID, operatorUUID string) (int64, error) {
	if f.pendingCountFn == nil {
		return 0, nil
	}
	return f.pendingCountFn(ctx, groupUUID, operatorUUID)
}

func (f *fakeGroupRepoForService) GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	if f.getGroupInfoFn == nil {
		return nil, repository.ErrRecordNotFound
	}
	return f.getGroupInfoFn(ctx, groupUUID)
}

func (f *fakeGroupRepoForService) GetGroupsByUUIDs(ctx context.Context, groupUUIDs []string) (map[string]*model.GroupInfo, error) {
	if f.getGroupsByUUIDsFn == nil {
		return map[string]*model.GroupInfo{}, nil
	}
	return f.getGroupsByUUIDsFn(ctx, groupUUIDs)
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

func (f *fakeGroupRepoForService) CheckGroupSendPermission(ctx context.Context, groupUUID, userUUID string) (repository.CheckGroupSendPermissionResult, error) {
	if f.checkSendFn == nil {
		return repository.CheckGroupSendPermissionResult{}, nil
	}
	return f.checkSendFn(ctx, groupUUID, userUUID)
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

type fakeRealtimePublisher struct {
	events []realtimepush.Event
	err    error
}

func (p *fakeRealtimePublisher) Publish(_ context.Context, event realtimepush.Event) error {
	if p.err != nil {
		return p.err
	}
	event.Normalize()
	p.events = append(p.events, event)
	return nil
}

func groupServiceTestContext(userUUID string) context.Context {
	return ctxmeta.WithUserUUID(context.Background(), userUUID)
}

func requireGroupBizCode(t *testing.T, err error, wantCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantCode, apperr.Code(err))
}

func stringPtr(value string) *string {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
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
	assert.Equal(t, "", gotGroup.Notice)
	assert.Equal(t, "owner-uuid", gotGroup.OwnerUuid)
	assert.Equal(t, len(gotMembers), gotGroup.MemberCnt)
	assert.Equal(t, int8(0), gotGroup.AddMode)
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
	var gotUpdates repository.GroupInfoUpdates
	repo := &fakeGroupRepoForService{
		updateGroupInfoFn: func(_ context.Context, _, _ string, updates repository.GroupInfoUpdates) error {
			called++
			gotUpdates = updates
			return nil
		},
	}
	svc := NewGroupService(repo)
	name := "  新群名  "
	avatar := "  https://example.com/a.png  "
	addMode := int32(1)
	err := svc.UpdateGroupInfo(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupInfoRequest{
		GroupUuid: " group-uuid ",
		Name:      &name,
		Avatar:    &avatar,
		AddMode:   &addMode,
	})
	require.NoError(t, err)
	require.NotNil(t, gotUpdates.Name)
	require.NotNil(t, gotUpdates.Avatar)
	require.NotNil(t, gotUpdates.AddMode)
	assert.Equal(t, 1, called)
	assert.Equal(t, "新群名", *gotUpdates.Name)
	assert.Equal(t, "https://example.com/a.png", *gotUpdates.Avatar)
	assert.Equal(t, int8(1), *gotUpdates.AddMode)
	err = svc.UpdateGroupInfo(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupInfoRequest{GroupUuid: "group-uuid"})
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestUpdateGroupNoticePassesOperatorAndNotice(t *testing.T) {
	var gotArgs []string
	repo := &fakeGroupRepoForService{
		updateNoticeFn: func(_ context.Context, groupUUID, operatorUUID, notice string) error {
			gotArgs = []string{groupUUID, operatorUUID, notice}
			return nil
		},
	}
	svc := NewGroupService(repo)
	err := svc.UpdateGroupNotice(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupNoticeRequest{
		GroupUuid: " group-uuid ",
		Notice:    "  新公告  ",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"group-uuid", "admin-uuid", "新公告"}, gotArgs)
}

func TestApplyJoinGroupReturnsRepositoryResult(t *testing.T) {
	repo := &fakeGroupRepoForService{
		applyJoinGroupFn: func(_ context.Context, groupUUID, applicantUUID, reason string) (repository.ApplyJoinGroupResult, error) {
			assert.Equal(t, "group-uuid", groupUUID)
			assert.Equal(t, "user-uuid", applicantUUID)
			assert.Equal(t, "申请理由", reason)
			return repository.ApplyJoinGroupResult{ApplyID: 123, JoinedDirectly: false}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.ApplyJoinGroup(groupServiceTestContext("user-uuid"), &pb.ApplyJoinGroupRequest{
		GroupUuid: " group-uuid ",
		Reason:    "  申请理由  ",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(123), resp.GetApplyId())
	assert.False(t, resp.GetJoinedDirectly())
}

func TestCancelJoinGroupApplicationPassesNormalizedArgs(t *testing.T) {
	var gotGroupUUID string
	var gotApplicantUUID string
	repo := &fakeGroupRepoForService{
		cancelJoinApplyFn: func(_ context.Context, groupUUID, applicantUUID string) error {
			gotGroupUUID = groupUUID
			gotApplicantUUID = applicantUUID
			return nil
		},
	}
	svc := NewGroupService(repo)
	err := svc.CancelJoinGroupApplication(groupServiceTestContext("user-uuid"), &pb.CancelJoinGroupApplicationRequest{GroupUuid: " group-uuid "})
	require.NoError(t, err)
	assert.Equal(t, "group-uuid", gotGroupUUID)
	assert.Equal(t, "user-uuid", gotApplicantUUID)
}

func TestGetMyJoinGroupApplicationBuildsResponse(t *testing.T) {
	reviewedAt := time.Unix(1710000100, 0)
	repo := &fakeGroupRepoForService{
		getMyJoinApplyFn: func(_ context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
			assert.Equal(t, "group-uuid", groupUUID)
			assert.Equal(t, "user-uuid", applicantUUID)
			return &model.GroupJoinRequest{
				Id:            18,
				ApplicantUuid: applicantUUID,
				Status:        2,
				Reason:        "申请原因",
				ReviewerUuid:  "admin-1",
				ReviewRemark:  "拒绝",
				CreatedAt:     time.Unix(1710000000, 0),
				ReviewedAt:    &reviewedAt,
			}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.GetMyJoinGroupApplication(groupServiceTestContext("user-uuid"), &pb.GetMyJoinGroupApplicationRequest{GroupUuid: " group-uuid "})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.GetHasApplication())
	require.NotNil(t, resp.GetApplication())
	assert.Equal(t, int64(18), resp.GetApplication().GetApplyId())
	assert.Equal(t, int32(2), resp.GetApplication().GetStatus())
	assert.Equal(t, "申请原因", resp.GetApplication().GetReason())
	assert.Equal(t, "admin-1", resp.GetApplication().GetReviewerUuid())
	assert.Equal(t, "拒绝", resp.GetApplication().GetReviewRemark())
	assert.Equal(t, reviewedAt.UnixMilli(), resp.GetApplication().GetReviewedAt())
}

func TestGetMyJoinGroupApplicationReturnsEmptyWhenMissing(t *testing.T) {
	svc := NewGroupService(&fakeGroupRepoForService{})
	resp, err := svc.GetMyJoinGroupApplication(groupServiceTestContext("user-uuid"), &pb.GetMyJoinGroupApplicationRequest{GroupUuid: "group-uuid"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.GetHasApplication())
	assert.Nil(t, resp.GetApplication())
}

func TestReviewJoinGroupPassesNormalizedArgs(t *testing.T) {
	var gotApplyID int64
	var gotAction int8
	var gotRemark string
	repo := &fakeGroupRepoForService{
		reviewJoinGroupFn: func(_ context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error {
			assert.Equal(t, "group-uuid", groupUUID)
			assert.Equal(t, "admin-uuid", operatorUUID)
			gotApplyID = applyID
			gotAction = action
			gotRemark = remark
			return nil
		},
	}
	svc := NewGroupService(repo)
	err := svc.ReviewJoinGroup(groupServiceTestContext("admin-uuid"), &pb.ReviewJoinGroupRequest{
		GroupUuid: " group-uuid ",
		ApplyId:   99,
		Action:    1,
		Remark:    "  同意  ",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(99), gotApplyID)
	assert.Equal(t, int8(1), gotAction)
	assert.Equal(t, "同意", gotRemark)
}

func TestReviewJoinGroupPublishesApplicantRealtimeNotice(t *testing.T) {
	publisher := &fakeRealtimePublisher{}
	repo := &fakeGroupRepoForService{
		getJoinApplicantFn: func(_ context.Context, groupUUID string, applyID int64) (string, error) {
			assert.Equal(t, "group-uuid", groupUUID)
			assert.Equal(t, int64(99), applyID)
			return "applicant-uuid", nil
		},
		reviewJoinGroupFn: func(_ context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error {
			assert.Equal(t, "group-uuid", groupUUID)
			assert.Equal(t, "admin-uuid", operatorUUID)
			assert.Equal(t, int64(99), applyID)
			assert.Equal(t, joinRequestActionReject, action)
			assert.Equal(t, "资料不完整", remark)
			return nil
		},
	}
	svc := NewGroupService(repo, publisher)

	err := svc.ReviewJoinGroup(groupServiceTestContext("admin-uuid"), &pb.ReviewJoinGroupRequest{
		GroupUuid: " group-uuid ",
		ApplyId:   99,
		Action:    int32(joinRequestActionReject),
		Remark:    "  资料不完整  ",
	})
	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	event := publisher.events[0]
	assert.Equal(t, realtimepush.TypeGroupJoinRequestReviewed, event.Type)
	assert.Equal(t, realtimepush.TargetKindUser, event.Target.Kind)
	assert.Equal(t, "applicant-uuid", event.Target.UserUUID)

	var payload realtimepb.GroupJoinRequestReviewedPayload
	require.NoError(t, proto.Unmarshal(event.Data, &payload))
	assert.Equal(t, "group-uuid", payload.GetGroupUuid())
	assert.Equal(t, "applicant-uuid", payload.GetApplicantUuid())
	assert.Equal(t, "admin-uuid", payload.GetReviewerUuid())
	assert.Equal(t, int64(99), payload.GetApplyId())
	assert.Equal(t, int32(joinRequestActionReject), payload.GetAction())
}

func TestListJoinRequestsBuildsApplicantProfiles(t *testing.T) {
	repo := &fakeGroupRepoForService{
		listJoinReqsFn: func(_ context.Context, groupUUID, operatorUUID string, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
			assert.Equal(t, "group-uuid", groupUUID)
			assert.Equal(t, "admin-uuid", operatorUUID)
			assert.Equal(t, 1, page)
			assert.Equal(t, 20, pageSize)
			return []*model.GroupJoinRequest{{
				Id:            7,
				ApplicantUuid: "user-a",
				Reason:        "申请入群",
				CreatedAt:     time.Unix(1710000000, 0),
			}}, 1, nil
		},
		getUserProfilesFn: func(_ context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
			assert.Equal(t, []string{"user-a"}, userUUIDs)
			return map[string]*model.UserProfile{
				"user-a": {Nickname: "张三", Avatar: "https://example.com/a.png"},
			}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.ListJoinRequests(groupServiceTestContext("admin-uuid"), &pb.ListJoinRequestsRequest{GroupUuid: "group-uuid"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.GetItems(), 1)
	assert.Equal(t, int64(1), resp.GetTotal())
	assert.Equal(t, "user-a", resp.GetItems()[0].GetApplicantUuid())
	assert.Equal(t, "张三", resp.GetItems()[0].GetNickname())
	assert.Equal(t, "https://example.com/a.png", resp.GetItems()[0].GetAvatar())
	assert.Equal(t, "申请入群", resp.GetItems()[0].GetReason())
}

func TestUpdateGroupInfoRejectsBlankName(t *testing.T) {
	svc := NewGroupService(&fakeGroupRepoForService{})
	name := "   "
	err := svc.UpdateGroupInfo(groupServiceTestContext("admin-uuid"), &pb.UpdateGroupInfoRequest{
		GroupUuid: "group-uuid",
		Name:      &name,
	})
	requireGroupBizCode(t, err, consts.CodeParamError)
}

func TestTransferGroupOwnerAndUpdateMemberRolePassOperatorFromContext(t *testing.T) {
	var transferArgs []string
	var roleArgs []any
	repo := &fakeGroupRepoForService{
		transferOwnerFn: func(_ context.Context, groupUUID, operatorUUID, targetUserUUID string) error {
			transferArgs = []string{groupUUID, operatorUUID, targetUserUUID}
			return nil
		},
		updateMemberRoleFn: func(_ context.Context, groupUUID, operatorUUID, targetUserUUID string, role int8) error {
			roleArgs = []any{groupUUID, operatorUUID, targetUserUUID, role}
			return nil
		},
	}
	svc := NewGroupService(repo)
	err := svc.TransferGroupOwner(groupServiceTestContext("owner-uuid"), &pb.TransferGroupOwnerRequest{
		GroupUuid:      " group-uuid ",
		TargetUserUuid: " member-1 ",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"group-uuid", "owner-uuid", "member-1"}, transferArgs)
	err = svc.UpdateMemberRole(groupServiceTestContext("owner-uuid"), &pb.UpdateMemberRoleRequest{
		GroupUuid: " group-uuid ",
		UserUuid:  " member-2 ",
		Role:      1,
	})
	require.NoError(t, err)
	assert.Equal(t, []any{"group-uuid", "owner-uuid", "member-2", int8(1)}, roleArgs)
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
		{name: "成员不存在", err: repository.ErrGroupMemberNotFound, want: consts.CodeGroupMemberNotFound},
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

func TestGetGroupListUsesContextUserAndMapsResponse(t *testing.T) {
	var gotUserUUID string
	repo := &fakeGroupRepoForService{
		listUserGroupsFn: func(_ context.Context, userUUID string) ([]*model.GroupInfo, error) {
			gotUserUUID = userUUID
			return []*model.GroupInfo{{
				Uuid:      "group-1",
				Name:      "测试群",
				Avatar:    "avatar.png",
				OwnerUuid: "owner-1",
				MemberCnt: 3,
			}}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.GetGroupList(groupServiceTestContext("user-1"), &pb.GetGroupListRequest{})
	require.NoError(t, err)
	assert.Equal(t, "user-1", gotUserUUID)
	require.Len(t, resp.GetGroups(), 1)
	assert.Equal(t, "group-1", resp.GetGroups()[0].GetGroupUuid())
	assert.Equal(t, "测试群", resp.GetGroups()[0].GetName())
	assert.Equal(t, int32(3), resp.GetGroups()[0].GetMemberCount())
	_, err = svc.GetGroupList(context.Background(), &pb.GetGroupListRequest{})
	requireGroupBizCode(t, err, consts.CodeUnauthorized)
}

func TestGetGroupMemberIdsDeduplicatesMembers(t *testing.T) {
	repo := &fakeGroupRepoForService{
		getGroupMembersFn: func(context.Context, string) ([]*model.GroupMember, error) {
			return []*model.GroupMember{
				{UserUuid: "user-1"},
				nil,
				{UserUuid: "user-1"},
				{UserUuid: "user-2"},
			}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.GetGroupMemberIds(context.Background(), &pb.GetGroupMemberIdsRequest{GroupUuid: " group-1 "})
	require.NoError(t, err)
	assert.Equal(t, []string{"user-1", "user-2"}, resp.GetUserUuids())
}

func TestGetMemberListBuildsProfileAwareItems(t *testing.T) {
	repo := &fakeGroupRepoForService{
		getGroupMembersFn: func(context.Context, string) ([]*model.GroupMember, error) {
			return []*model.GroupMember{
				{UserUuid: "owner-1", Role: 2},
				{UserUuid: "member-1", Role: 0},
			}, nil
		},
		getUserProfilesFn: func(_ context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
			assert.Equal(t, []string{"owner-1", "member-1"}, userUUIDs)
			return map[string]*model.UserProfile{
				"owner-1":  {UserUuid: "owner-1", Nickname: "群主", Avatar: "owner.png"},
				"member-1": {UserUuid: "member-1", Nickname: "成员", Avatar: "member.png"},
			}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.GetMemberList(context.Background(), &pb.GetMemberListRequest{GroupUuid: "group-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetMembers(), 2)
	assert.Equal(t, "owner-1", resp.GetMembers()[0].GetUserUuid())
	assert.Equal(t, "群主", resp.GetMembers()[0].GetNickname())
	assert.Equal(t, "owner.png", resp.GetMembers()[0].GetAvatar())
	assert.Equal(t, "member-1", resp.GetMembers()[1].GetUserUuid())
	assert.Equal(t, "成员", resp.GetMembers()[1].GetNickname())
}

func TestListMyJoinGroupApplicationsBuildsGroupAwareItems(t *testing.T) {
	var gotApplicantUUID string
	var gotPage int
	var gotPageSize int
	var gotGroupUUIDs []string
	reviewedAt := time.UnixMilli(1710000005000)
	repo := &fakeGroupRepoForService{
		listMyJoinAppsFn: func(_ context.Context, applicantUUID string, _ *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
			gotApplicantUUID = applicantUUID
			gotPage = page
			gotPageSize = pageSize
			return []*model.GroupJoinRequest{
				{
					Id:           101,
					GroupUuid:    "group-1",
					Status:       1,
					Reason:       "想加入群一",
					ReviewerUuid: "admin-1",
					ReviewRemark: "已通过",
					CreatedAt:    time.UnixMilli(1710000000000),
					ReviewedAt:   &reviewedAt,
				},
				{
					Id:        102,
					GroupUuid: "group-2",
					Status:    0,
					Reason:    "想加入群二",
					CreatedAt: time.UnixMilli(1710000003000),
				},
			}, 2, nil
		},
		getGroupsByUUIDsFn: func(_ context.Context, groupUUIDs []string) (map[string]*model.GroupInfo, error) {
			gotGroupUUIDs = append([]string(nil), groupUUIDs...)
			return map[string]*model.GroupInfo{
				"group-1": {Uuid: "group-1", Name: "群一", Avatar: "group-1.png"},
				"group-2": {Uuid: "group-2", Name: "群二", Avatar: "group-2.png"},
			}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.ListMyJoinGroupApplications(groupServiceTestContext("user-join"), &pb.ListMyJoinGroupApplicationsRequest{})
	require.NoError(t, err)
	assert.Equal(t, "user-join", gotApplicantUUID)
	assert.Equal(t, 1, gotPage)
	assert.Equal(t, 20, gotPageSize)
	assert.Equal(t, []string{"group-1", "group-2"}, gotGroupUUIDs)
	require.Len(t, resp.GetItems(), 2)
	assert.Equal(t, int64(2), resp.GetTotal())
	assert.Equal(t, int32(1), resp.GetPage())
	assert.Equal(t, int32(20), resp.GetPageSize())
	assert.Equal(t, int64(101), resp.GetItems()[0].GetApplyId())
	assert.Equal(t, "group-1", resp.GetItems()[0].GetGroupUuid())
	assert.Equal(t, "群一", resp.GetItems()[0].GetGroupName())
	assert.Equal(t, "group-1.png", resp.GetItems()[0].GetGroupAvatar())
	assert.Equal(t, int32(1), resp.GetItems()[0].GetStatus())
	assert.Equal(t, "已通过", resp.GetItems()[0].GetReviewRemark())
	assert.Equal(t, reviewedAt.UnixMilli(), resp.GetItems()[0].GetReviewedAt())
	assert.Equal(t, int64(102), resp.GetItems()[1].GetApplyId())
	assert.Equal(t, "群二", resp.GetItems()[1].GetGroupName())
	assert.Equal(t, int32(0), resp.GetItems()[1].GetStatus())
	assert.Zero(t, resp.GetItems()[1].GetReviewedAt())
}

func TestListReviewedJoinRequestsBuildsApplicantAwareItems(t *testing.T) {
	var gotGroupUUID string
	var gotOperatorUUID string
	var gotPage int
	var gotPageSize int
	var gotUserUUIDs []string
	reviewedAt := time.UnixMilli(1710000010000)
	repo := &fakeGroupRepoForService{
		listReviewedFn: func(_ context.Context, groupUUID, operatorUUID string, _ *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
			gotGroupUUID = groupUUID
			gotOperatorUUID = operatorUUID
			gotPage = page
			gotPageSize = pageSize
			return []*model.GroupJoinRequest{{
				Id:            201,
				ApplicantUuid: "user-a",
				Status:        2,
				Reason:        "资料不完整",
				ReviewerUuid:  "admin-reviewer",
				ReviewRemark:  "已拒绝",
				CreatedAt:     time.UnixMilli(1710000006000),
				ReviewedAt:    &reviewedAt,
			}}, 1, nil
		},
		getUserProfilesFn: func(_ context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
			gotUserUUIDs = append([]string(nil), userUUIDs...)
			return map[string]*model.UserProfile{
				"user-a": {UserUuid: "user-a", Nickname: "申请人A", Avatar: "user-a.png"},
			}, nil
		},
	}
	svc := NewGroupService(repo)
	resp, err := svc.ListReviewedJoinRequests(groupServiceTestContext("admin-reviewer"), &pb.ListReviewedJoinRequestsRequest{GroupUuid: " group-review ", Page: 2, PageSize: 50})
	require.NoError(t, err)
	assert.Equal(t, "group-review", gotGroupUUID)
	assert.Equal(t, "admin-reviewer", gotOperatorUUID)
	assert.Equal(t, 2, gotPage)
	assert.Equal(t, 50, gotPageSize)
	assert.Equal(t, []string{"user-a"}, gotUserUUIDs)
	require.Len(t, resp.GetItems(), 1)
	assert.Equal(t, int64(1), resp.GetTotal())
	assert.Equal(t, int32(2), resp.GetPage())
	assert.Equal(t, int32(50), resp.GetPageSize())
	assert.Equal(t, int64(201), resp.GetItems()[0].GetApplyId())
	assert.Equal(t, "user-a", resp.GetItems()[0].GetApplicantUuid())
	assert.Equal(t, "申请人A", resp.GetItems()[0].GetNickname())
	assert.Equal(t, "user-a.png", resp.GetItems()[0].GetAvatar())
	assert.Equal(t, int32(2), resp.GetItems()[0].GetStatus())
	assert.Equal(t, "已拒绝", resp.GetItems()[0].GetReviewRemark())
	assert.Equal(t, reviewedAt.UnixMilli(), resp.GetItems()[0].GetReviewedAt())
}
