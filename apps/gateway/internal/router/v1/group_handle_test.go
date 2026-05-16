package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeGroupHTTPService struct {
	updateGroupNoticeFn func(context.Context, *dto.UpdateGroupNoticeRequest) error
	applyJoinGroupFn    func(context.Context, *dto.ApplyJoinGroupRequest) (*dto.ApplyJoinGroupResponse, error)
	cancelJoinApplyFn   func(context.Context, *dto.CancelJoinGroupApplicationRequest) error
	getMyJoinApplyFn    func(context.Context, *dto.GetMyJoinGroupApplicationRequest) (*dto.GetMyJoinGroupApplicationResponse, error)
	listMyJoinAppsFn    func(context.Context, *dto.ListMyJoinGroupApplicationsRequest) (*dto.ListMyJoinGroupApplicationsResponse, error)
	reviewJoinGroupFn   func(context.Context, *dto.ReviewJoinGroupRequest) error
	listJoinRequestsFn  func(context.Context, *dto.ListJoinRequestsRequest) (*dto.ListJoinRequestsResponse, error)
	listReviewedFn      func(context.Context, *dto.ListReviewedJoinRequestsRequest) (*dto.ListReviewedJoinRequestsResponse, error)
}

var _ service.GroupService = (*fakeGroupHTTPService)(nil)

func (f *fakeGroupHTTPService) CreateGroup(context.Context, *dto.CreateGroupRequest) (*dto.CreateGroupResponse, error) {
	return &dto.CreateGroupResponse{}, nil
}
func (f *fakeGroupHTTPService) DismissGroup(context.Context, *dto.DismissGroupRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) GetGroupInfo(context.Context, *dto.GetGroupInfoRequest) (*dto.GroupInfoDTO, error) {
	return &dto.GroupInfoDTO{}, nil
}
func (f *fakeGroupHTTPService) UpdateGroupInfo(context.Context, *dto.UpdateGroupInfoRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) UpdateGroupNotice(ctx context.Context, req *dto.UpdateGroupNoticeRequest) error {
	if f.updateGroupNoticeFn == nil {
		return nil
	}
	return f.updateGroupNoticeFn(ctx, req)
}
func (f *fakeGroupHTTPService) TransferGroupOwner(context.Context, *dto.TransferGroupOwnerRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) UpdateMemberRole(context.Context, *dto.UpdateGroupMemberRoleRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) ApplyJoinGroup(ctx context.Context, req *dto.ApplyJoinGroupRequest) (*dto.ApplyJoinGroupResponse, error) {
	if f.applyJoinGroupFn == nil {
		return &dto.ApplyJoinGroupResponse{}, nil
	}
	return f.applyJoinGroupFn(ctx, req)
}
func (f *fakeGroupHTTPService) CancelJoinGroupApplication(ctx context.Context, req *dto.CancelJoinGroupApplicationRequest) error {
	if f.cancelJoinApplyFn == nil {
		return nil
	}
	return f.cancelJoinApplyFn(ctx, req)
}
func (f *fakeGroupHTTPService) GetMyJoinGroupApplication(ctx context.Context, req *dto.GetMyJoinGroupApplicationRequest) (*dto.GetMyJoinGroupApplicationResponse, error) {
	if f.getMyJoinApplyFn == nil {
		return &dto.GetMyJoinGroupApplicationResponse{}, nil
	}
	return f.getMyJoinApplyFn(ctx, req)
}
func (f *fakeGroupHTTPService) ListMyJoinGroupApplications(ctx context.Context, req *dto.ListMyJoinGroupApplicationsRequest) (*dto.ListMyJoinGroupApplicationsResponse, error) {
	if f.listMyJoinAppsFn == nil {
		return &dto.ListMyJoinGroupApplicationsResponse{}, nil
	}
	return f.listMyJoinAppsFn(ctx, req)
}
func (f *fakeGroupHTTPService) ReviewJoinGroup(ctx context.Context, req *dto.ReviewJoinGroupRequest) error {
	if f.reviewJoinGroupFn == nil {
		return nil
	}
	return f.reviewJoinGroupFn(ctx, req)
}
func (f *fakeGroupHTTPService) ListJoinRequests(ctx context.Context, req *dto.ListJoinRequestsRequest) (*dto.ListJoinRequestsResponse, error) {
	if f.listJoinRequestsFn == nil {
		return &dto.ListJoinRequestsResponse{}, nil
	}
	return f.listJoinRequestsFn(ctx, req)
}
func (f *fakeGroupHTTPService) ListReviewedJoinRequests(ctx context.Context, req *dto.ListReviewedJoinRequestsRequest) (*dto.ListReviewedJoinRequestsResponse, error) {
	if f.listReviewedFn == nil {
		return &dto.ListReviewedJoinRequestsResponse{}, nil
	}
	return f.listReviewedFn(ctx, req)
}
func (f *fakeGroupHTTPService) AddMember(context.Context, *dto.AddGroupMemberRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) RemoveMember(context.Context, *dto.RemoveGroupMemberRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) GetMemberList(context.Context, *dto.GetGroupMemberListRequest) (*dto.GetGroupMemberListResponse, error) {
	return &dto.GetGroupMemberListResponse{}, nil
}
func (f *fakeGroupHTTPService) GetGroupList(context.Context) (*dto.GetGroupListResponse, error) {
	return &dto.GetGroupListResponse{}, nil
}
func (f *fakeGroupHTTPService) GetGroupMemberIDs(context.Context, *dto.GetGroupMemberIDsRequest) (*dto.GetGroupMemberIDsResponse, error) {
	return &dto.GetGroupMemberIDsResponse{}, nil
}
func (f *fakeGroupHTTPService) LeaveGroup(context.Context, *dto.LeaveGroupRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) SearchGroupMembers(context.Context, *dto.SearchGroupMembersRequest) (*dto.SearchGroupMembersResponse, error) {
	return &dto.SearchGroupMembersResponse{}, nil
}
func (f *fakeGroupHTTPService) UpdateMyGroupNickname(context.Context, *dto.UpdateMyGroupNicknameRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) MuteGroupMember(context.Context, *dto.MuteGroupMemberRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) UpdateGroupMuteSetting(context.Context, *dto.UpdateGroupMuteSettingRequest) error {
	return nil
}
func (f *fakeGroupHTTPService) GetJoinRequestPendingCount(context.Context, *dto.GetJoinRequestPendingCountRequest) (*dto.GetJoinRequestPendingCountResponse, error) {
	return &dto.GetJoinRequestPendingCountResponse{}, nil
}

type groupHandlerResultBody struct {
	Code int `json:"code"`
	Data any `json:"data"`
}

var gatewayGroupHandlerLoggerOnce sync.Once

func initGatewayGroupHandlerLogger() {
	gatewayGroupHandlerLoggerOnce.Do(func() {
		gin.SetMode(gin.TestMode)
	})
}
func decodeGroupHandlerBody(t *testing.T, w *httptest.ResponseRecorder) groupHandlerResultBody {
	t.Helper()
	var body groupHandlerResultBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}
func newGroupJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}
func TestGroupHandlerUpdateGroupNotice(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		updateGroupNoticeFn: func(_ context.Context, req *dto.UpdateGroupNoticeRequest) error {
			called = true
			assert.Equal(t, "group-1", req.GroupUUID)
			assert.Equal(t, "新公告", req.Notice)
			return nil
		},
	})
	w := httptest.NewRecorder()
	req := newGroupJSONRequest(t, http.MethodPut, "/api/v1/auth/groups/group-1/notice", `{"notice":"新公告"}`)
	c, r := gin.CreateTestContext(w)
	r.PUT("/api/v1/auth/groups/:groupUuid/notice", h.UpdateGroupNotice)
	c.Request = req
	c.Params = gin.Params{{Key: "groupUuid", Value: "group-1"}}
	h.UpdateGroupNotice(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
func TestGroupHandlerApplyJoinGroup(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		applyJoinGroupFn: func(_ context.Context, req *dto.ApplyJoinGroupRequest) (*dto.ApplyJoinGroupResponse, error) {
			called = true
			assert.Equal(t, "group-2", req.GroupUUID)
			assert.Equal(t, "想加入", req.Reason)
			return &dto.ApplyJoinGroupResponse{ApplyID: 10, JoinedDirectly: false}, nil
		},
	})
	w := httptest.NewRecorder()
	req := newGroupJSONRequest(t, http.MethodPost, "/api/v1/auth/groups/group-2/apply", `{"reason":"想加入"}`)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "groupUuid", Value: "group-2"}}
	h.ApplyJoinGroup(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
func TestGroupHandlerCancelJoinGroupApplication(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		cancelJoinApplyFn: func(_ context.Context, req *dto.CancelJoinGroupApplicationRequest) error {
			called = true
			assert.Equal(t, "group-cancel", req.GroupUUID)
			return nil
		},
	})
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodDelete, "/api/v1/auth/groups/group-cancel/apply", nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "groupUuid", Value: "group-cancel"}}
	h.CancelJoinGroupApplication(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
func TestGroupHandlerGetMyJoinGroupApplication(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		getMyJoinApplyFn: func(_ context.Context, req *dto.GetMyJoinGroupApplicationRequest) (*dto.GetMyJoinGroupApplicationResponse, error) {
			called = true
			assert.Equal(t, "group-status", req.GroupUUID)
			return &dto.GetMyJoinGroupApplicationResponse{
				HasApplication: true,
				Application: &dto.MyJoinGroupApplicationDTO{
					ApplyID: 31,
					Status:  3,
					Reason:  "已撤销",
				},
			}, nil
		},
	})
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/groups/group-status/my-join-application", nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "groupUuid", Value: "group-status"}}
	h.GetMyJoinGroupApplication(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
func TestGroupHandlerListMyJoinGroupApplications(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		listMyJoinAppsFn: func(_ context.Context, req *dto.ListMyJoinGroupApplicationsRequest) (*dto.ListMyJoinGroupApplicationsResponse, error) {
			called = true
			assert.Equal(t, int32(3), req.Page)
			assert.Equal(t, int32(10), req.PageSize)
			return &dto.ListMyJoinGroupApplicationsResponse{
				Total: 1,
				Items: []*dto.MyJoinGroupApplicationListItemDTO{{
					ApplyID:   41,
					GroupUUID: "group-9",
					GroupName: "群九",
				}},
			}, nil
		},
	})
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/groups/join-applications?page=3&pageSize=10", nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.ListMyJoinGroupApplications(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
func TestGroupHandlerReviewJoinGroup(t *testing.T) {
	initGatewayGroupHandlerLogger()
	t.Run("invalid_apply_id", func(t *testing.T) {
		called := false
		h := NewGroupHandler(&fakeGroupHTTPService{
			reviewJoinGroupFn: func(_ context.Context, _ *dto.ReviewJoinGroupRequest) error {
				called = true
				return nil
			},
		})
		w := httptest.NewRecorder()
		req := newGroupJSONRequest(t, http.MethodPost, "/api/v1/auth/groups/group-3/join-requests/bad/review", `{"action":1}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = gin.Params{{Key: "groupUuid", Value: "group-3"}, {Key: "applyId", Value: "bad"}}
		h.ReviewJoinGroup(c)
		body := decodeGroupHandlerBody(t, w)
		assert.Equal(t, consts.CodeParamError, body.Code)
		assert.False(t, called)
	})
	t.Run("success", func(t *testing.T) {
		called := false
		h := NewGroupHandler(&fakeGroupHTTPService{
			reviewJoinGroupFn: func(_ context.Context, req *dto.ReviewJoinGroupRequest) error {
				called = true
				assert.Equal(t, "group-3", req.GroupUUID)
				assert.Equal(t, int64(12), req.ApplyID)
				assert.Equal(t, int32(2), req.Action)
				assert.Equal(t, "拒绝", req.Remark)
				return nil
			},
		})
		w := httptest.NewRecorder()
		req := newGroupJSONRequest(t, http.MethodPost, "/api/v1/auth/groups/group-3/join-requests/12/review", `{"action":2,"remark":"拒绝"}`)
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = gin.Params{{Key: "groupUuid", Value: "group-3"}, {Key: "applyId", Value: "12"}}
		h.ReviewJoinGroup(c)
		body := decodeGroupHandlerBody(t, w)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, consts.CodeSuccess, body.Code)
		assert.True(t, called)
	})
}
func TestGroupHandlerListJoinRequests(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		listJoinRequestsFn: func(_ context.Context, req *dto.ListJoinRequestsRequest) (*dto.ListJoinRequestsResponse, error) {
			called = true
			assert.Equal(t, "group-4", req.GroupUUID)
			assert.Equal(t, int32(2), req.Page)
			assert.Equal(t, int32(50), req.PageSize)
			return &dto.ListJoinRequestsResponse{Total: 0, Items: []*dto.GroupJoinRequestItemDTO{}}, nil
		},
	})
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/groups/group-4/join-requests?page=2&pageSize=50", nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "groupUuid", Value: "group-4"}}
	h.ListJoinRequests(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
func TestGroupHandlerListReviewedJoinRequests(t *testing.T) {
	initGatewayGroupHandlerLogger()
	called := false
	h := NewGroupHandler(&fakeGroupHTTPService{
		listReviewedFn: func(_ context.Context, req *dto.ListReviewedJoinRequestsRequest) (*dto.ListReviewedJoinRequestsResponse, error) {
			called = true
			assert.Equal(t, "group-reviewed", req.GroupUUID)
			assert.Equal(t, int32(2), req.Page)
			assert.Equal(t, int32(30), req.PageSize)
			return &dto.ListReviewedJoinRequestsResponse{
				Total: 1,
				Items: []*dto.ReviewedJoinRequestItemDTO{{
					ApplyID:       55,
					ApplicantUUID: "user-reviewed",
				}},
			}, nil
		},
	})
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/groups/group-reviewed/join-requests/reviewed?page=2&pageSize=30", nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "groupUuid", Value: "group-reviewed"}}
	h.ListReviewedJoinRequests(c)
	body := decodeGroupHandlerBody(t, w)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, consts.CodeSuccess, body.Code)
	assert.True(t, called)
}
