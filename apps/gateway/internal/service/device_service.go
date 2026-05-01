package service

import (
	"context"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
)

// DeviceServiceImpl 设备服务实现
type DeviceServiceImpl struct {
	userClient pb.UserServiceClient
}

// NewDeviceService 创建设备服务实例
// userClient: 用户服务 gRPC 客户端
func NewDeviceService(userClient pb.UserServiceClient) DeviceService {
	return &DeviceServiceImpl{
		userClient: userClient,
	}
}

// GetDeviceList 获取设备列表
func (s *DeviceServiceImpl) GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error) {
	grpcResp, err := s.userClient.GetDeviceList(ctx, &authpb.GetDeviceListRequest{})
	if err != nil {
		return nil, err
	}

	return dto.ConvertGetDeviceListResponseFromProto(grpcResp), nil
}

// KickDevice 踢出设备
func (s *DeviceServiceImpl) KickDevice(ctx context.Context, req *dto.KickDeviceRequest) (*dto.KickDeviceResponse, error) {

	grpcReq := dto.ConvertToProtoKickDeviceRequest(req)
	grpcResp, err := s.userClient.KickDevice(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertKickDeviceResponseFromProto(grpcResp), nil
}

// GetOnlineStatus 获取用户在线状态
func (s *DeviceServiceImpl) GetOnlineStatus(ctx context.Context, req *dto.GetOnlineStatusRequest) (*dto.GetOnlineStatusResponse, error) {

	grpcReq := dto.ConvertToProtoGetOnlineStatusRequest(req)
	grpcResp, err := s.userClient.GetOnlineStatus(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertGetOnlineStatusResponseFromProto(grpcResp), nil
}

// BatchGetOnlineStatus 批量获取在线状态
func (s *DeviceServiceImpl) BatchGetOnlineStatus(ctx context.Context, req *dto.BatchGetOnlineStatusRequest) (*dto.BatchGetOnlineStatusResponse, error) {

	grpcReq := dto.ConvertToProtoBatchGetOnlineStatusRequest(req)
	grpcResp, err := s.userClient.BatchGetOnlineStatus(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertBatchGetOnlineStatusResponseFromProto(grpcResp), nil
}
