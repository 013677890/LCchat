package service

import (
	"context"
	"errors"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type fakeGatewayGroupClient struct {
	gatewaypb.GroupServiceClient
	updateGroupNoticeFn func(context.Context, *grouppb.UpdateGroupNoticeRequest) (*grouppb.UpdateGroupNoticeResponse, error)
	applyJoinGroupFn    func(context.Context, *grouppb.ApplyJoinGroupRequest) (*grouppb.ApplyJoinGroupResponse, error)
	cancelJoinApplyFn   func(context.Context, *grouppb.CancelJoinGroupApplicationRequest) (*grouppb.CancelJoinGroupApplicationResponse, error)
	getMyJoinApplyFn    func(context.Context, *grouppb.GetMyJoinGroupApplicationRequest) (*grouppb.GetMyJoinGroupApplicationResponse, error)
	listMyJoinAppsFn    func(context.Context, *grouppb.ListMyJoinGroupApplicationsRequest) (*grouppb.ListMyJoinGroupApplicationsResponse, error)
	reviewJoinGroupFn   func(context.Context, *grouppb.ReviewJoinGroupRequest) (*grouppb.ReviewJoinGroupResponse, error)
	listJoinRequestsFn  func(context.Context, *grouppb.ListJoinRequestsRequest) (*grouppb.ListJoinRequestsResponse, error)
	listReviewedFn      func(context.Context, *grouppb.ListReviewedJoinRequestsRequest) (*grouppb.ListReviewedJoinRequestsResponse, error)
}

func (f *fakeGatewayGroupClient) CreateGroup(context.Context, *grouppb.CreateGroupRequest) (*grouppb.CreateGroupResponse, error) {
	return &grouppb.CreateGroupResponse{}, nil
}
func (f *fakeGatewayGroupClient) DismissGroup(context.Context, *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error) {
	return &grouppb.DismissGroupResponse{}, nil
}
func (f *fakeGatewayGroupClient) GetGroupInfo(context.Context, *grouppb.GetGroupInfoRequest) (*grouppb.GetGroupInfoResponse, error) {
	return &grouppb.GetGroupInfoResponse{}, nil
}
func (f *fakeGatewayGroupClient) UpdateGroupInfo(context.Context, *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error) {
	return &grouppb.UpdateGroupInfoResponse{}, nil
}
func (f *fakeGatewayGroupClient) UpdateGroupNotice(ctx context.Context, req *grouppb.UpdateGroupNoticeRequest) (*grouppb.UpdateGroupNoticeResponse, error) {
	if f.updateGroupNoticeFn == nil {
		return &grouppb.UpdateGroupNoticeResponse{}, nil
	}
	return f.updateGroupNoticeFn(ctx, req)
}
func (f *fakeGatewayGroupClient) TransferGroupOwner(context.Context, *grouppb.TransferGroupOwnerRequest) (*grouppb.TransferGroupOwnerResponse, error) {
	return &grouppb.TransferGroupOwnerResponse{}, nil
}
func (f *fakeGatewayGroupClient) UpdateMemberRole(context.Context, *grouppb.UpdateMemberRoleRequest) (*grouppb.UpdateMemberRoleResponse, error) {
	return &grouppb.UpdateMemberRoleResponse{}, nil
}
func (f *fakeGatewayGroupClient) ApplyJoinGroup(ctx context.Context, req *grouppb.ApplyJoinGroupRequest) (*grouppb.ApplyJoinGroupResponse, error) {
	if f.applyJoinGroupFn == nil {
		return &grouppb.ApplyJoinGroupResponse{}, nil
	}
	return f.applyJoinGroupFn(ctx, req)
}
func (f *fakeGatewayGroupClient) CancelJoinGroupApplication(ctx context.Context, req *grouppb.CancelJoinGroupApplicationRequest) (*grouppb.CancelJoinGroupApplicationResponse, error) {
	if f.cancelJoinApplyFn == nil {
		return &grouppb.CancelJoinGroupApplicationResponse{}, nil
	}
	return f.cancelJoinApplyFn(ctx, req)
}
func (f *fakeGatewayGroupClient) GetMyJoinGroupApplication(ctx context.Context, req *grouppb.GetMyJoinGroupApplicationRequest) (*grouppb.GetMyJoinGroupApplicationResponse, error) {
	if f.getMyJoinApplyFn == nil {
		return &grouppb.GetMyJoinGroupApplicationResponse{}, nil
	}
	return f.getMyJoinApplyFn(ctx, req)
}
func (f *fakeGatewayGroupClient) ListMyJoinGroupApplications(ctx context.Context, req *grouppb.ListMyJoinGroupApplicationsRequest) (*grouppb.ListMyJoinGroupApplicationsResponse, error) {
	if f.listMyJoinAppsFn == nil {
		return &grouppb.ListMyJoinGroupApplicationsResponse{}, nil
	}
	return f.listMyJoinAppsFn(ctx, req)
}
func (f *fakeGatewayGroupClient) ReviewJoinGroup(ctx context.Context, req *grouppb.ReviewJoinGroupRequest) (*grouppb.ReviewJoinGroupResponse, error) {
	if f.reviewJoinGroupFn == nil {
		return &grouppb.ReviewJoinGroupResponse{}, nil
	}
	return f.reviewJoinGroupFn(ctx, req)
}
func (f *fakeGatewayGroupClient) ListJoinRequests(ctx context.Context, req *grouppb.ListJoinRequestsRequest) (*grouppb.ListJoinRequestsResponse, error) {
	if f.listJoinRequestsFn == nil {
		return &grouppb.ListJoinRequestsResponse{}, nil
	}
	return f.listJoinRequestsFn(ctx, req)
}
func (f *fakeGatewayGroupClient) ListReviewedJoinRequests(ctx context.Context, req *grouppb.ListReviewedJoinRequestsRequest) (*grouppb.ListReviewedJoinRequestsResponse, error) {
	if f.listReviewedFn == nil {
		return &grouppb.ListReviewedJoinRequestsResponse{}, nil
	}
	return f.listReviewedFn(ctx, req)
}
func (f *fakeGatewayGroupClient) AddMember(context.Context, *grouppb.AddMemberRequest) (*grouppb.AddMemberResponse, error) {
	return &grouppb.AddMemberResponse{}, nil
}
func (f *fakeGatewayGroupClient) RemoveMember(context.Context, *grouppb.RemoveMemberRequest) (*grouppb.RemoveMemberResponse, error) {
	return &grouppb.RemoveMemberResponse{}, nil
}
func (f *fakeGatewayGroupClient) GetMemberList(context.Context, *grouppb.GetMemberListRequest) (*grouppb.GetMemberListResponse, error) {
	return &grouppb.GetMemberListResponse{}, nil
}
func (f *fakeGatewayGroupClient) GetGroupList(context.Context, *grouppb.GetGroupListRequest) (*grouppb.GetGroupListResponse, error) {
	return &grouppb.GetGroupListResponse{}, nil
}
func (f *fakeGatewayGroupClient) GetGroupMemberIds(context.Context, *grouppb.GetGroupMemberIdsRequest) (*grouppb.GetGroupMemberIdsResponse, error) {
	return &grouppb.GetGroupMemberIdsResponse{}, nil
}
func (f *fakeGatewayGroupClient) CheckGroupMember(context.Context, *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error) {
	return &grouppb.CheckGroupMemberResponse{}, nil
}
func (f *fakeGatewayGroupClient) LeaveGroup(context.Context, *grouppb.LeaveGroupRequest) (*grouppb.LeaveGroupResponse, error) {
	return &grouppb.LeaveGroupResponse{}, nil
}
func (f *fakeGatewayGroupClient) SearchGroupMembers(context.Context, *grouppb.SearchGroupMembersRequest) (*grouppb.SearchGroupMembersResponse, error) {
	return &grouppb.SearchGroupMembersResponse{}, nil
}
func (f *fakeGatewayGroupClient) UpdateMyGroupNickname(context.Context, *grouppb.UpdateMyGroupNicknameRequest) (*grouppb.UpdateMyGroupNicknameResponse, error) {
	return &grouppb.UpdateMyGroupNicknameResponse{}, nil
}
func (f *fakeGatewayGroupClient) MuteGroupMember(context.Context, *grouppb.MuteGroupMemberRequest) (*grouppb.MuteGroupMemberResponse, error) {
	return &grouppb.MuteGroupMemberResponse{}, nil
}
func (f *fakeGatewayGroupClient) UpdateGroupMuteSetting(context.Context, *grouppb.UpdateGroupMuteSettingRequest) (*grouppb.UpdateGroupMuteSettingResponse, error) {
	return &grouppb.UpdateGroupMuteSettingResponse{}, nil
}
func (f *fakeGatewayGroupClient) CheckGroupSendPermission(context.Context, *grouppb.CheckGroupSendPermissionRequest) (*grouppb.CheckGroupSendPermissionResponse, error) {
	return &grouppb.CheckGroupSendPermissionResponse{}, nil
}
func (f *fakeGatewayGroupClient) GetJoinRequestPendingCount(context.Context, *grouppb.GetJoinRequestPendingCountRequest) (*grouppb.GetJoinRequestPendingCountResponse, error) {
	return &grouppb.GetJoinRequestPendingCountResponse{}, nil
}
func TestGatewayGroupServiceJoinRequestMethods(t *testing.T) {
	t.Run("update_notice_maps_request", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			updateGroupNoticeFn: func(_ context.Context, req *grouppb.UpdateGroupNoticeRequest) (*grouppb.UpdateGroupNoticeResponse, error) {
				require.Equal(t, "group-1", req.GetGroupUuid())
				require.Equal(t, "新公告", req.GetNotice())
				return &grouppb.UpdateGroupNoticeResponse{}, nil
			},
		})
		err := svc.UpdateGroupNotice(context.Background(), &dto.UpdateGroupNoticeRequest{GroupUUID: "group-1", Notice: "新公告"})
		require.NoError(t, err)
	})
	t.Run("apply_join_group_maps_response", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			applyJoinGroupFn: func(_ context.Context, req *grouppb.ApplyJoinGroupRequest) (*grouppb.ApplyJoinGroupResponse, error) {
				require.Equal(t, "group-2", req.GetGroupUuid())
				require.Equal(t, "想加入", req.GetReason())
				return &grouppb.ApplyJoinGroupResponse{ApplyId: 8, JoinedDirectly: false}, nil
			},
		})
		resp, err := svc.ApplyJoinGroup(context.Background(), &dto.ApplyJoinGroupRequest{GroupUUID: "group-2", Reason: "想加入"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(8), resp.ApplyID)
		assert.False(t, resp.JoinedDirectly)
	})
	t.Run("cancel_join_group_application_maps_request", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			cancelJoinApplyFn: func(_ context.Context, req *grouppb.CancelJoinGroupApplicationRequest) (*grouppb.CancelJoinGroupApplicationResponse, error) {
				require.Equal(t, "group-cancel", req.GetGroupUuid())
				return &grouppb.CancelJoinGroupApplicationResponse{}, nil
			},
		})
		err := svc.CancelJoinGroupApplication(context.Background(), &dto.CancelJoinGroupApplicationRequest{GroupUUID: "group-cancel"})
		require.NoError(t, err)
	})
	t.Run("get_my_join_group_application_maps_response", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			getMyJoinApplyFn: func(_ context.Context, req *grouppb.GetMyJoinGroupApplicationRequest) (*grouppb.GetMyJoinGroupApplicationResponse, error) {
				require.Equal(t, "group-status", req.GetGroupUuid())
				return &grouppb.GetMyJoinGroupApplicationResponse{
					HasApplication: true,
					Application: &grouppb.MyJoinGroupApplication{
						ApplyId:      21,
						Status:       3,
						Reason:       "撤销原因",
						ReviewerUuid: "",
						ReviewRemark: "",
						CreatedAt:    1710000000000,
					},
				}, nil
			},
		})
		resp, err := svc.GetMyJoinGroupApplication(context.Background(), &dto.GetMyJoinGroupApplicationRequest{GroupUUID: "group-status"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.HasApplication)
		require.NotNil(t, resp.Application)
		assert.Equal(t, int64(21), resp.Application.ApplyID)
		assert.Equal(t, int32(3), resp.Application.Status)
		assert.Equal(t, "撤销原因", resp.Application.Reason)
	})
	t.Run("list_my_join_group_applications_maps_items", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			listMyJoinAppsFn: func(_ context.Context, req *grouppb.ListMyJoinGroupApplicationsRequest) (*grouppb.ListMyJoinGroupApplicationsResponse, error) {
				require.Equal(t, int32(3), req.GetPage())
				require.Equal(t, int32(10), req.GetPageSize())
				return &grouppb.ListMyJoinGroupApplicationsResponse{
					Items: []*grouppb.MyJoinGroupApplicationListItem{{
						ApplyId:      41,
						GroupUuid:    "group-9",
						GroupName:    "群九",
						GroupAvatar:  "group-9.png",
						Status:       1,
						Reason:       "想加入群九",
						ReviewerUuid: "admin-9",
						ReviewRemark: "已通过",
						CreatedAt:    1710000010000,
						ReviewedAt:   1710000015000,
					}},
					Total:    1,
					Page:     3,
					PageSize: 10,
				}, nil
			},
		})
		resp, err := svc.ListMyJoinGroupApplications(context.Background(), &dto.ListMyJoinGroupApplicationsRequest{Page: 3, PageSize: 10})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
		assert.Equal(t, int64(1), resp.Total)
		assert.Equal(t, int32(3), resp.Page)
		assert.Equal(t, int32(10), resp.PageSize)
		assert.Equal(t, int64(41), resp.Items[0].ApplyID)
		assert.Equal(t, "group-9", resp.Items[0].GroupUUID)
		assert.Equal(t, "群九", resp.Items[0].GroupName)
		assert.Equal(t, "group-9.png", resp.Items[0].GroupAvatar)
		assert.Equal(t, int32(1), resp.Items[0].Status)
		assert.Equal(t, "已通过", resp.Items[0].ReviewRemark)
	})
	t.Run("review_join_group_passes_args", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			reviewJoinGroupFn: func(_ context.Context, req *grouppb.ReviewJoinGroupRequest) (*grouppb.ReviewJoinGroupResponse, error) {
				require.Equal(t, "group-3", req.GetGroupUuid())
				require.Equal(t, int64(9), req.GetApplyId())
				require.Equal(t, int32(2), req.GetAction())
				require.Equal(t, "拒绝", req.GetRemark())
				return &grouppb.ReviewJoinGroupResponse{}, nil
			},
		})
		err := svc.ReviewJoinGroup(context.Background(), &dto.ReviewJoinGroupRequest{GroupUUID: "group-3", ApplyID: 9, Action: 2, Remark: "拒绝"})
		require.NoError(t, err)
	})
	t.Run("list_join_requests_maps_items", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			listJoinRequestsFn: func(_ context.Context, req *grouppb.ListJoinRequestsRequest) (*grouppb.ListJoinRequestsResponse, error) {
				require.Equal(t, "group-4", req.GetGroupUuid())
				require.Equal(t, int32(2), req.GetPage())
				require.Equal(t, int32(50), req.GetPageSize())
				return &grouppb.ListJoinRequestsResponse{
					Items: []*grouppb.GroupJoinRequestItem{{
						ApplyId:       11,
						ApplicantUuid: "user-1",
						Nickname:      "张三",
						Avatar:        "avatar.png",
						Reason:        "申请加入",
						CreatedAt:     1710000000000,
					}},
					Total:    1,
					Page:     2,
					PageSize: 50,
				}, nil
			},
		})
		resp, err := svc.ListJoinRequests(context.Background(), &dto.ListJoinRequestsRequest{GroupUUID: "group-4", Page: 2, PageSize: 50})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
		assert.Equal(t, int64(1), resp.Total)
		assert.Equal(t, int64(11), resp.Items[0].ApplyID)
		assert.Equal(t, "张三", resp.Items[0].Nickname)
	})
	t.Run("list_reviewed_join_requests_maps_items", func(t *testing.T) {
		svc := NewGroupService(&fakeGatewayGroupClient{
			listReviewedFn: func(_ context.Context, req *grouppb.ListReviewedJoinRequestsRequest) (*grouppb.ListReviewedJoinRequestsResponse, error) {
				require.Equal(t, "group-reviewed", req.GetGroupUuid())
				require.Equal(t, int32(2), req.GetPage())
				require.Equal(t, int32(30), req.GetPageSize())
				return &grouppb.ListReviewedJoinRequestsResponse{
					Items: []*grouppb.ReviewedJoinRequestItem{{
						ApplyId:       55,
						ApplicantUuid: "user-reviewed",
						Nickname:      "李四",
						Avatar:        "reviewed.png",
						Status:        2,
						Reason:        "资料不足",
						ReviewerUuid:  "admin-reviewed",
						ReviewRemark:  "已拒绝",
						CreatedAt:     1710000020000,
						ReviewedAt:    1710000025000,
					}},
					Total:    1,
					Page:     2,
					PageSize: 30,
				}, nil
			},
		})
		resp, err := svc.ListReviewedJoinRequests(context.Background(), &dto.ListReviewedJoinRequestsRequest{GroupUUID: "group-reviewed", Page: 2, PageSize: 30})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
		assert.Equal(t, int64(1), resp.Total)
		assert.Equal(t, int32(2), resp.Page)
		assert.Equal(t, int32(30), resp.PageSize)
		assert.Equal(t, int64(55), resp.Items[0].ApplyID)
		assert.Equal(t, "user-reviewed", resp.Items[0].ApplicantUUID)
		assert.Equal(t, "李四", resp.Items[0].Nickname)
		assert.Equal(t, "reviewed.png", resp.Items[0].Avatar)
		assert.Equal(t, int32(2), resp.Items[0].Status)
		assert.Equal(t, "已拒绝", resp.Items[0].ReviewRemark)
	})
	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		svc := NewGroupService(&fakeGatewayGroupClient{
			applyJoinGroupFn: func(_ context.Context, _ *grouppb.ApplyJoinGroupRequest) (*grouppb.ApplyJoinGroupResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := svc.ApplyJoinGroup(context.Background(), &dto.ApplyJoinGroupRequest{GroupUUID: "group-5"})
		require.Nil(t, resp)
		require.Equal(t, wantErr, err)
	})
}
