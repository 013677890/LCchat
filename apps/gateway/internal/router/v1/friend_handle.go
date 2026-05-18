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

// FriendHandler 好友处理器
type FriendHandler struct {
	friendService service.FriendService
}

// NewFriendHandler 创建好友处理器
func NewFriendHandler(friendService service.FriendService) *FriendHandler {
	return &FriendHandler{
		friendService: friendService,
	}
}

// SendFriendApply 发送好友申请接口
// @Summary 发送好友申请
// @Description 向目标用户发送好友请求
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.SendFriendApplyRequest true "发送好友申请请求"
// @Success 200 {object} dto.SendFriendApplyResponse
// @Router /api/v1/user/friend/apply [post]
func (h *FriendHandler) SendFriendApply(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.SendFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	applyResp, err := h.friendService.SendFriendApply(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如用户不存在、已经是好友等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, applyResp)
}

// GetFriendApplyList 获取好友申请列表接口
// @Summary 获取好友申请列表
// @Description 获取收到的好友申请列表
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param status query int false "状态(0:待处理 1:已同意 2:已拒绝)"
// @Param page query int false "页码(默认1)"
// @Param pageSize query int false "每页数量(默认20)"
// @Success 200 {object} dto.GetFriendApplyListResponse
// @Router /api/v1/user/friend/apply/list [get]
func (h *FriendHandler) GetFriendApplyList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定查询参数
	var req dto.GetFriendApplyListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	applyListResp, err := h.friendService.GetFriendApplyList(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, applyListResp)
}

// GetSentApplyList 获取发出的申请列表接口
// @Summary 获取发出的申请列表
// @Description 获取发出的好友申请列表
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param status query int false "状态(0:待处理 1:已同意 2:已拒绝)"
// @Param page query int false "页码(默认1)"
// @Param pageSize query int false "每页数量(默认20)"
// @Success 200 {object} dto.GetSentApplyListResponse
// @Router /api/v1/user/friend/apply/sent [get]
func (h *FriendHandler) GetSentApplyList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定查询参数
	var req dto.GetSentApplyListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	sentListResp, err := h.friendService.GetSentApplyList(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, sentListResp)
}

// HandleFriendApply 处理好友申请接口
// @Summary 处理好友申请
// @Description 同意或拒绝好友申请
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.HandleFriendApplyRequest true "处理好友申请请求"
// @Success 200 {object} dto.HandleFriendApplyResponse
// @Router /api/v1/user/friend/apply/handle [post]
func (h *FriendHandler) HandleFriendApply(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.HandleFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.friendService.HandleFriendApply(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如申请不存在、无权限等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, nil)
}

// GetUnreadApplyCount 获取未读申请数量接口
// @Summary 获取未读申请数量
// @Description 获取收到的好友申请未读数量
// @Tags 好友接口
// @Accept json
// @Produce json
// @Success 200 {object} dto.GetUnreadApplyCountResponse
// @Router /api/v1/user/friend/apply/unread [get]
func (h *FriendHandler) GetUnreadApplyCount(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 调用服务层处理业务逻辑（依赖注入）
	unreadResp, err := h.friendService.GetUnreadApplyCount(ctx, &dto.GetUnreadApplyCountRequest{})
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 2. 返回成功响应
	result.Success(c, unreadResp)
}

// MarkApplyAsRead 标记申请已读接口
// @Summary 标记申请已读
// @Description 批量标记好友申请为已读
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.MarkApplyAsReadRequest true "标记申请已读请求"
// @Success 200 {object} dto.MarkApplyAsReadResponse
// @Router /api/v1/user/friend/apply/read [post]
func (h *FriendHandler) MarkApplyAsRead(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.MarkApplyAsReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 验证申请ID列表
	if len(req.ApplyIDs) == 0 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.friendService.MarkApplyAsRead(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, nil)
}

// GetFriendList 获取好友列表接口
// @Summary 获取好友列表
// @Description 获取当前用户的好友列表
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param groupTag query string false "标签"
// @Param page query int false "页码(默认1)"
// @Param pageSize query int false "每页数量(默认20)"
// @Success 200 {object} dto.GetFriendListResponse
// @Router /api/v1/user/friend/list [get]
func (h *FriendHandler) GetFriendList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定查询参数
	var req dto.GetFriendListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	friendListResp, err := h.friendService.GetFriendList(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, friendListResp)
}

// SyncFriendList 好友增量同步接口
// @Summary 好友增量同步
// @Description 增量同步好友列表
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.SyncFriendListRequest true "增量同步请求"
// @Success 200 {object} dto.SyncFriendListResponse
// @Router /api/v1/user/friend/sync [post]
func (h *FriendHandler) SyncFriendList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.SyncFriendListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 设置默认值
	if req.Limit == 0 {
		req.Limit = 100
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	syncResp, err := h.friendService.SyncFriendList(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, syncResp)
}

// DeleteFriend 删除好友接口
// @Summary 删除好友
// @Description 删除指定的好友
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.DeleteFriendRequest true "删除好友请求"
// @Success 200 {object} dto.DeleteFriendResponse
// @Router /api/v1/user/friend/delete [post]
func (h *FriendHandler) DeleteFriend(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.DeleteFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.friendService.DeleteFriend(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如好友不存在等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, nil)
}

// SetFriendRemark 设置好友备注接口
// @Summary 设置好友备注
// @Description 设置好友的备注名
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.SetFriendRemarkRequest true "设置好友备注请求"
// @Success 200 {object} dto.SetFriendRemarkResponse
// @Router /api/v1/user/friend/remark [post]
func (h *FriendHandler) SetFriendRemark(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.SetFriendRemarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.friendService.SetFriendRemark(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, nil)
}

// SetFriendTag 设置好友标签接口
// @Summary 设置好友标签
// @Description 设置好友的标签分组
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.SetFriendTagRequest true "设置好友标签请求"
// @Success 200 {object} dto.SetFriendTagResponse
// @Router /api/v1/user/friend/tag [post]
func (h *FriendHandler) SetFriendTag(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.SetFriendTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.friendService.SetFriendTag(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, nil)
}

// CheckIsFriend 判断是否好友接口
// @Summary 判断是否好友
// @Description 判断两个用户是否为好友关系
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.CheckIsFriendRequest true "判断是否好友请求"
// @Success 200 {object} dto.CheckIsFriendResponse
// @Router /api/v1/user/friend/check [post]
func (h *FriendHandler) CheckIsFriend(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.CheckIsFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	checkResp, err := h.friendService.CheckIsFriend(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, checkResp)
}

// GetRelationStatus 获取关系状态接口
// @Summary 获取关系状态
// @Description 获取与指定用户的关系状态
// @Tags 好友接口
// @Accept json
// @Produce json
// @Param request body dto.GetRelationStatusRequest true "获取关系状态请求"
// @Success 200 {object} dto.GetRelationStatusResponse
// @Router /api/v1/user/friend/relation [post]
func (h *FriendHandler) GetRelationStatus(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.GetRelationStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	relationResp, err := h.friendService.GetRelationStatus(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, relationResp)
}
