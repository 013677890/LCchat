package handler

import (
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"context"
)

// DeviceHandler 设备会话服务Handler
type DeviceHandler struct {
	pb.UnimplementedDeviceServiceServer

	deviceService service.IDeviceService
}

// NewDeviceHandler 创建设备Handler实例
func NewDeviceHandler(deviceService service.IDeviceService) *DeviceHandler {
	return &DeviceHandler{
		deviceService: deviceService,
	}
}

// GetDeviceList 获取设备列表
func (h *DeviceHandler) GetDeviceList(ctx context.Context, req *pb.GetDeviceListRequest) (*pb.GetDeviceListResponse, error) {
	return h.deviceService.GetDeviceList(ctx, req)
}

// KickDevice 踢出设备
func (h *DeviceHandler) KickDevice(ctx context.Context, req *pb.KickDeviceRequest) (*pb.KickDeviceResponse, error) {
	return &pb.KickDeviceResponse{}, h.deviceService.KickDevice(ctx, req)
}

// GetOnlineStatus 获取用户在线状态
func (h *DeviceHandler) GetOnlineStatus(ctx context.Context, req *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error) {
	return h.deviceService.GetOnlineStatus(ctx, req)
}

// BatchGetOnlineStatus 批量获取在线状态
func (h *DeviceHandler) BatchGetOnlineStatus(ctx context.Context, req *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error) {
	return h.deviceService.BatchGetOnlineStatus(ctx, req)
}

// UpdateDeviceActive 批量更新设备活跃时间（内部调用）
func (h *DeviceHandler) UpdateDeviceActive(ctx context.Context, req *pb.UpdateDeviceActiveRequest) (*pb.UpdateDeviceActiveResponse, error) {
	return &pb.UpdateDeviceActiveResponse{}, h.deviceService.UpdateDeviceActive(ctx, req)
}

// UpdateDeviceStatus 更新设备在线状态（内部调用）
func (h *DeviceHandler) UpdateDeviceStatus(ctx context.Context, req *pb.UpdateDeviceStatusRequest) (*pb.UpdateDeviceStatusResponse, error) {
	return &pb.UpdateDeviceStatusResponse{}, h.deviceService.UpdateDeviceStatus(ctx, req)
}
