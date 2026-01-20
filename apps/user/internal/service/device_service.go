package service

import (
	"ChatServer/apps/user/internal/dto"
	"ChatServer/apps/user/internal/repository"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// deviceServiceImpl 设备会话服务实现
type deviceServiceImpl struct {
	deviceRepo repository.IDeviceRepository
}

// NewDeviceService 创建设备服务实例
func NewDeviceService(deviceRepo repository.IDeviceRepository) DeviceService {
	return &deviceServiceImpl{
		deviceRepo: deviceRepo,
	}
}

// GetDeviceList 获取设备列表
func (s *deviceServiceImpl) GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取设备列表功能暂未实现")
}

// KickDevice 踢出设备
func (s *deviceServiceImpl) KickDevice(ctx context.Context, req *dto.KickDeviceRequestNew) error {
	return status.Error(codes.Unimplemented, "踢出设备功能暂未实现")
}

// GetOnlineStatus 获取用户在线状态
func (s *deviceServiceImpl) GetOnlineStatus(ctx context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取用户在线状态功能暂未实现")
}

// BatchGetOnlineStatus 批量获取在线状态
func (s *deviceServiceImpl) BatchGetOnlineStatus(ctx context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "批量获取在线状态功能暂未实现")
}
