package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"github.com/gin-gonic/gin"
	"strconv"
)

// GroupHandler 是 gateway 的群 HTTP 入站适配层。
//
// 这里专注做参数绑定、路径参数补齐和错误响应转换，
// 真实群业务规则统一下沉到下游 group-service 维护。
type GroupHandler struct {
	groupService service.GroupService
}

// NewGroupHandler 创建群组处理器。
func NewGroupHandler(groupService service.GroupService) *GroupHandler {
	return &GroupHandler{groupService: groupService}
}

// CreateGroup 创建群。
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	var req dto.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.CreateGroup(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// GetGroupList 获取当前用户群列表。
func (h *GroupHandler) GetGroupList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	resp, err := h.groupService.GetGroupList(ctx)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// GetGroupInfo 获取群资料。
func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.GetGroupInfo(ctx, &dto.GetGroupInfoRequest{GroupUUID: groupUUID})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// UpdateGroupInfo 更新群资料。
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	if err := h.groupService.UpdateGroupInfo(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// UpdateGroupNotice 独立更新群公告。
func (h *GroupHandler) UpdateGroupNotice(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.UpdateGroupNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	if err := h.groupService.UpdateGroupNotice(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// TransferGroupOwner 转让群主。
func (h *GroupHandler) TransferGroupOwner(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.TransferGroupOwnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	if err := h.groupService.TransferGroupOwner(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// UpdateMemberRole 更新群成员角色。
func (h *GroupHandler) UpdateMemberRole(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	userUUID := c.Param("userUuid")
	if groupUUID == "" || userUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.UpdateGroupMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	req.UserUUID = userUUID
	if err := h.groupService.UpdateMemberRole(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// ApplyJoinGroup 申请加入群聊。
func (h *GroupHandler) ApplyJoinGroup(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.ApplyJoinGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	resp, err := h.groupService.ApplyJoinGroup(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
func (h *GroupHandler) CancelJoinGroupApplication(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if err := h.groupService.CancelJoinGroupApplication(ctx, &dto.CancelJoinGroupApplicationRequest{GroupUUID: groupUUID}); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
func (h *GroupHandler) GetMyJoinGroupApplication(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.GetMyJoinGroupApplication(ctx, &dto.GetMyJoinGroupApplicationRequest{GroupUUID: groupUUID})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
func (h *GroupHandler) ListMyJoinGroupApplications(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	var req dto.ListMyJoinGroupApplicationsRequest
	// 该接口是“当前登录用户”的全局申请列表，只接收分页参数，不依赖群路径参数。
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.ListMyJoinGroupApplications(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// ReviewJoinGroup 审批入群申请。
func (h *GroupHandler) ReviewJoinGroup(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	applyID, err := strconv.ParseInt(c.Param("applyId"), 10, 64)
	if groupUUID == "" || err != nil || applyID <= 0 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.ReviewJoinGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	req.ApplyID = applyID
	if err := h.groupService.ReviewJoinGroup(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// DismissGroup 解散群。
func (h *GroupHandler) DismissGroup(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if err := h.groupService.DismissGroup(ctx, &dto.DismissGroupRequest{GroupUUID: groupUUID}); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// AddMember 添加群成员。
func (h *GroupHandler) AddMember(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.AddGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	if err := h.groupService.AddMember(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// LeaveGroup 当前用户主动退出群聊。
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if err := h.groupService.LeaveGroup(ctx, &dto.LeaveGroupRequest{GroupUUID: groupUUID}); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// RemoveMember 移除群成员。
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	userUUID := c.Param("userUuid")
	if groupUUID == "" || userUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	err := h.groupService.RemoveMember(ctx, &dto.RemoveGroupMemberRequest{
		GroupUUID: groupUUID,
		UserUUID:  userUUID,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// GetMemberList 获取群成员列表。
func (h *GroupHandler) GetMemberList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.GetMemberList(ctx, &dto.GetGroupMemberListRequest{GroupUUID: groupUUID})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// SearchGroupMembers 搜索群成员。
func (h *GroupHandler) SearchGroupMembers(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.SearchGroupMembersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	resp, err := h.groupService.SearchGroupMembers(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// SearchGroups 按群名或完整群号搜索正常群。
func (h *GroupHandler) SearchGroups(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	var req dto.SearchGroupsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.SearchGroups(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// UpdateMyGroupNickname 更新当前用户自己的群名片。
func (h *GroupHandler) UpdateMyGroupNickname(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.UpdateMyGroupNicknameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	if err := h.groupService.UpdateMyGroupNickname(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// UpdateGroupMemberNickname 管理员或群主修改指定成员群名片。
func (h *GroupHandler) UpdateGroupMemberNickname(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	userUUID := c.Param("userUuid")
	if groupUUID == "" || userUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.UpdateGroupMemberNicknameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	req.UserUUID = userUUID
	if err := h.groupService.UpdateGroupMemberNickname(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// MuteGroupMember 设置或取消指定成员单人禁言。
func (h *GroupHandler) MuteGroupMember(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	userUUID := c.Param("userUuid")
	if groupUUID == "" || userUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.MuteGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	req.UserUUID = userUUID
	if err := h.groupService.MuteGroupMember(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// UpdateGroupMuteSetting 更新全员禁言开关。
func (h *GroupHandler) UpdateGroupMuteSetting(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.UpdateGroupMuteSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	if err := h.groupService.UpdateGroupMuteSetting(ctx, &req); err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, nil)
}

// ListJoinRequests 获取待审批入群申请列表。
func (h *GroupHandler) ListJoinRequests(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.ListJoinRequestsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	resp, err := h.groupService.ListJoinRequests(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// ListReviewedJoinRequests 获取群已审批申请列表。
func (h *GroupHandler) ListReviewedJoinRequests(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	var req dto.ListReviewedJoinRequestsRequest
	// 路由参数决定审批记录的群范围，query 只负责分页，避免请求体里再传一份重复 group_uuid。
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	req.GroupUUID = groupUUID
	resp, err := h.groupService.ListReviewedJoinRequests(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// GetJoinRequestPendingCount 获取群待审批入群申请数量。
func (h *GroupHandler) GetJoinRequestPendingCount(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.GetJoinRequestPendingCount(ctx, &dto.GetJoinRequestPendingCountRequest{GroupUUID: groupUUID})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}

// GetGroupMemberIDs 获取群成员 UUID 列表。
func (h *GroupHandler) GetGroupMemberIDs(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	groupUUID := c.Param("groupUuid")
	if groupUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	resp, err := h.groupService.GetGroupMemberIDs(ctx, &dto.GetGroupMemberIDsRequest{GroupUUID: groupUUID})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	result.Success(c, resp)
}
