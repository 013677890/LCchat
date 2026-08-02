package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// DeviceHandler 负责承接设备会话相关 gRPC 请求。
type DeviceHandler struct {
	authpb.UnimplementedDeviceServiceServer

	deviceService service.IDeviceService
}

// NewDeviceHandler 创建设备 Handler 实例。
func NewDeviceHandler(deviceService service.IDeviceService) *DeviceHandler {
	return &DeviceHandler{deviceService: deviceService}
}

// GetDeviceList 获取当前用户设备列表。
func (h *DeviceHandler) GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
	return h.deviceService.GetDeviceList(ctx, req)
}

// KickDevice 踢出指定设备。
func (h *DeviceHandler) KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
	return &authpb.KickDeviceResponse{}, h.deviceService.KickDevice(ctx, req)
}

// GetOnlineStatus 获取单用户在线状态。
func (h *DeviceHandler) GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
	return h.deviceService.GetOnlineStatus(ctx, req)
}

// BatchGetOnlineStatus 批量获取在线状态。
func (h *DeviceHandler) BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
	return h.deviceService.BatchGetOnlineStatus(ctx, req)
}

// UpdateDeviceStatus 更新设备在线状态。
func (h *DeviceHandler) UpdateDeviceStatus(ctx context.Context, req *authpb.UpdateDeviceStatusRequest) (*authpb.UpdateDeviceStatusResponse, error) {
	return &authpb.UpdateDeviceStatusResponse{}, h.deviceService.UpdateDeviceStatus(ctx, req)
}
