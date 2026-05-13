package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/utils"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"github.com/gin-gonic/gin"
)

// GroupHandler 群组处理器。
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
		handleGroupError(c, err)
		return
	}
	result.Success(c, resp)
}

// GetGroupList 获取当前用户群列表。
func (h *GroupHandler) GetGroupList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)
	resp, err := h.groupService.GetGroupList(ctx)
	if err != nil {
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
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
		handleGroupError(c, err)
		return
	}
	result.Success(c, resp)
}
func handleGroupError(c *gin.Context, err error) {
	code := utils.ExtractErrorCode(err)
	if consts.IsNonServerError(code) {
		result.Fail(c, nil, code)
		return
	}
	result.FailServer(c, err, consts.CodeInternalError)
}
