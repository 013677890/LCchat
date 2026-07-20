package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/result"

	"github.com/gin-gonic/gin"
)

// DeviceHandler 设备处理器
type DeviceHandler struct {
	deviceService service.DeviceService
}

// NewDeviceHandler 创建设备处理器
func NewDeviceHandler(deviceService service.DeviceService) *DeviceHandler {
	return &DeviceHandler{
		deviceService: deviceService,
	}
}

// GetDeviceList 获取设备列表
// @Summary 获取设备列表
// @Description 查看当前登录的所有设备
// @Tags 设备接口
// @Produce json
// @Success 200 {object} dto.GetDeviceListResponse
// @Router /api/v1/auth/user/devices [get]
func (h *DeviceHandler) GetDeviceList(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	resp, err := h.deviceService.GetDeviceList(ctx)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// KickDevice 踢出设备
// @Summary 踢出设备
// @Description 强制下线某个设备
// @Tags 设备接口
// @Param deviceId path string true "设备ID"
// @Success 200 {object} dto.KickDeviceResponse
// @Router /api/v1/auth/user/devices/{deviceId} [delete]
func (h *DeviceHandler) KickDevice(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	deviceID := c.Param("deviceId")
	if deviceID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	req := &dto.KickDeviceRequest{DeviceID: deviceID}
	resp, err := h.deviceService.KickDevice(ctx, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// GetOnlineStatus 获取用户在线状态
// @Summary 获取在线状态
// @Description 查询用户是否在线
// @Tags 设备接口
// @Param userUuid path string true "用户UUID"
// @Success 200 {object} dto.GetOnlineStatusResponse
// @Router /api/v1/auth/user/online-status/{userUuid} [get]
func (h *DeviceHandler) GetOnlineStatus(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	userUUID := c.Param("userUuid")
	if userUUID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	req := &dto.GetOnlineStatusRequest{UserUUID: userUUID}
	resp, err := h.deviceService.GetOnlineStatus(ctx, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// BatchGetOnlineStatus 批量获取在线状态
// @Summary 批量获取在线状态
// @Description 批量查询多个用户在线状态
// @Tags 设备接口
// @Accept json
// @Produce json
// @Param request body dto.BatchGetOnlineStatusRequest true "批量获取在线状态请求"
// @Success 200 {object} dto.BatchGetOnlineStatusResponse
// @Router /api/v1/auth/user/batch-online-status [post]
func (h *DeviceHandler) BatchGetOnlineStatus(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.BatchGetOnlineStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if len(req.UserUUIDs) == 0 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if len(req.UserUUIDs) > 100 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	resp, err := h.deviceService.BatchGetOnlineStatus(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}
