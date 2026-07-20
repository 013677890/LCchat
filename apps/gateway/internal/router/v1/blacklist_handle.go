package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/result"

	"github.com/gin-gonic/gin"
)

// BlacklistHandler 黑名单处理器
type BlacklistHandler struct {
	blacklistService service.BlacklistService
}

// NewBlacklistHandler 创建黑名单处理器
func NewBlacklistHandler(blacklistService service.BlacklistService) *BlacklistHandler {
	return &BlacklistHandler{
		blacklistService: blacklistService,
	}
}

// AddBlacklist 拉黑用户接口
// @Summary 拉黑用户
// @Description 将用户加入黑名单
// @Tags 黑名单接口
// @Accept json
// @Produce json
// @Param request body dto.AddBlacklistRequest true "拉黑用户请求"
// @Success 200 {object} dto.AddBlacklistResponse
// @Router /api/v1/auth/blacklist [post]
func (h *BlacklistHandler) AddBlacklist(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.AddBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	resp, err := h.blacklistService.AddBlacklist(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// RemoveBlacklist 取消拉黑接口
// @Summary 取消拉黑
// @Description 将用户移出黑名单
// @Tags 黑名单接口
// @Accept json
// @Produce json
// @Param userUuid path string true "用户UUID"
// @Success 200 {object} dto.RemoveBlacklistResponse
// @Router /api/v1/auth/blacklist/{userUuid} [delete]
func (h *BlacklistHandler) RemoveBlacklist(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	userUuid := c.Param("userUuid")
	if userUuid == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	req := &dto.RemoveBlacklistRequest{
		UserUUID: userUuid,
	}

	resp, err := h.blacklistService.RemoveBlacklist(ctx, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// GetBlacklistList 获取黑名单列表接口
// @Summary 获取黑名单列表
// @Description 获取所有被拉黑的用户
// @Tags 黑名单接口
// @Accept json
// @Produce json
// @Param page query int false "页码(默认1)"
// @Param pageSize query int false "每页数量(默认20)"
// @Success 200 {object} dto.GetBlacklistListResponse
// @Router /api/v1/auth/blacklist [get]
func (h *BlacklistHandler) GetBlacklistList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.GetBlacklistListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	resp, err := h.blacklistService.GetBlacklistList(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// CheckIsBlacklist 判断是否拉黑接口
// @Summary 判断是否拉黑
// @Description 判断用户是否在黑名单中
// @Tags 黑名单接口
// @Accept json
// @Produce json
// @Param request body dto.CheckIsBlacklistRequest true "判断是否拉黑请求"
// @Success 200 {object} dto.CheckIsBlacklistResponse
// @Router /api/v1/auth/blacklist/check [post]
func (h *BlacklistHandler) CheckIsBlacklist(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.CheckIsBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 强制以当前登录用户为"本人"侧，禁止探测任意两人之间的拉黑关系（越权防护）。
	currentUserUUID, ok := middleware.GetUserUUID(c)
	if !ok {
		result.Fail(c, nil, consts.CodeUnauthorized)
		return
	}
	req.UserUUID = currentUserUUID

	resp, err := h.blacklistService.CheckIsBlacklist(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}
