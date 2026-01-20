package handler

import (
	"ChatServer/apps/user/internal/service"
	pb "ChatServer/apps/user/pb"
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
	return nil, nil
}

// KickDevice 踢出设备
func (h *DeviceHandler) KickDevice(ctx context.Context, req *pb.KickDeviceRequest) (*pb.KickDeviceResponse, error) {
	return nil, nil
}

// GetOnlineStatus 获取用户在线状态
func (h *DeviceHandler) GetOnlineStatus(ctx context.Context, req *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error) {
	return nil, nil
}

// BatchGetOnlineStatus 批量获取在线状态
func (h *DeviceHandler) BatchGetOnlineStatus(ctx context.Context, req *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error) {
	return nil, nil
}
