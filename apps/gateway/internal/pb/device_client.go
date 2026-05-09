package pb

import (
	"context"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// 设备会话接口仍由 auth-service 承载，
// 但在 facade 侧单独成文件，便于和认证/账号安全调用区分开。

// GetDeviceList 获取设备列表。
func (c *userServiceClientImpl) GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
	return c.deviceClient.GetDeviceList(ctx, req)
}

// KickDevice 踢出设备。
func (c *userServiceClientImpl) KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
	return c.deviceClient.KickDevice(ctx, req)
}

// GetOnlineStatus 获取用户在线状态。
func (c *userServiceClientImpl) GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
	return c.deviceClient.GetOnlineStatus(ctx, req)
}

// BatchGetOnlineStatus 批量获取在线状态。
func (c *userServiceClientImpl) BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
	return c.deviceClient.BatchGetOnlineStatus(ctx, req)
}
