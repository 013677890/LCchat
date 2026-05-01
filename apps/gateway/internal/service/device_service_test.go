package service

import (
	"context"
	"errors"
	"testing"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGatewayDeviceClient struct {
	gatewaypb.UserServiceClient

	getDeviceListFn        func(context.Context, *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error)
	kickDeviceFn           func(context.Context, *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error)
	getOnlineStatusFn      func(context.Context, *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error)
	batchGetOnlineStatusFn func(context.Context, *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error)
}

func (f *fakeGatewayDeviceClient) GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
	if f.getDeviceListFn == nil {
		return &authpb.GetDeviceListResponse{}, nil
	}
	return f.getDeviceListFn(ctx, req)
}

func (f *fakeGatewayDeviceClient) KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
	if f.kickDeviceFn == nil {
		return &authpb.KickDeviceResponse{}, nil
	}
	return f.kickDeviceFn(ctx, req)
}

func (f *fakeGatewayDeviceClient) GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
	if f.getOnlineStatusFn == nil {
		return &authpb.GetOnlineStatusResponse{}, nil
	}
	return f.getOnlineStatusFn(ctx, req)
}

func (f *fakeGatewayDeviceClient) BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
	if f.batchGetOnlineStatusFn == nil {
		return &authpb.BatchGetOnlineStatusResponse{}, nil
	}
	return f.batchGetOnlineStatusFn(ctx, req)
}

func TestGatewayDeviceServiceGetDeviceList(t *testing.T) {
	t.Run("success_mapping", func(t *testing.T) {
		ts := time.Date(2026, 2, 6, 12, 0, 0, 0, time.UTC)
		tsMilli := ts.UnixMilli()
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			getDeviceListFn: func(_ context.Context, _ *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
				return &authpb.GetDeviceListResponse{
					Devices: []*authpb.DeviceItem{
						{
							DeviceId:        "d1",
							DeviceName:      "iPhone",
							Platform:        "ios",
							AppVersion:      "1.0.0",
							IsCurrentDevice: true,
							Status:          0,
							LastSeenAt:      tsMilli,
						},
					},
				}, nil
			},
		})

		resp, err := svc.GetDeviceList(context.Background())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Devices, 1)
		assert.Equal(t, "d1", resp.Devices[0].DeviceID)
		assert.Equal(t, "iPhone", resp.Devices[0].DeviceName)
		assert.Equal(t, util.FormatUnixMilliRFC3339(tsMilli), resp.Devices[0].LastSeenAt)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			getDeviceListFn: func(_ context.Context, _ *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := svc.GetDeviceList(context.Background())
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayDeviceServiceKickDevice(t *testing.T) {
	t.Run("success_mapping", func(t *testing.T) {
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			kickDeviceFn: func(_ context.Context, req *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
				require.Equal(t, "d1", req.DeviceId)
				return &authpb.KickDeviceResponse{}, nil
			},
		})
		resp, err := svc.KickDevice(context.Background(), &dto.KickDeviceRequest{DeviceID: "d1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc failed")
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			kickDeviceFn: func(_ context.Context, _ *authpb.KickDeviceRequest) (*authpb.KickDeviceResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := svc.KickDevice(context.Background(), &dto.KickDeviceRequest{DeviceID: "d1"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayDeviceServiceGetOnlineStatus(t *testing.T) {
	t.Run("success_mapping", func(t *testing.T) {
		ts := time.Date(2026, 2, 6, 12, 30, 0, 0, time.UTC).UnixMilli()
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			getOnlineStatusFn: func(_ context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
				require.Equal(t, "u2", req.UserUuid)
				return &authpb.GetOnlineStatusResponse{
					Status: &authpb.OnlineStatus{
						UserUuid:        "u2",
						IsOnline:        true,
						LastSeenAt:      ts,
						OnlinePlatforms: []string{"ios", "web"},
					},
				}, nil
			},
		})

		resp, err := svc.GetOnlineStatus(context.Background(), &dto.GetOnlineStatusRequest{UserUUID: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "u2", resp.UserUUID)
		assert.True(t, resp.IsOnline)
		assert.Equal(t, util.FormatUnixMilliRFC3339(ts), resp.LastSeenAt)
		assert.Equal(t, []string{"ios", "web"}, resp.OnlinePlatforms)
	})

	t.Run("status_nil_mapping", func(t *testing.T) {
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			getOnlineStatusFn: func(_ context.Context, _ *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
				return &authpb.GetOnlineStatusResponse{Status: nil}, nil
			},
		})
		resp, err := svc.GetOnlineStatus(context.Background(), &dto.GetOnlineStatusRequest{UserUUID: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.UserUUID)
		assert.False(t, resp.IsOnline)
		assert.Empty(t, resp.LastSeenAt)
		assert.Empty(t, resp.OnlinePlatforms)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc failed")
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			getOnlineStatusFn: func(_ context.Context, _ *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := svc.GetOnlineStatus(context.Background(), &dto.GetOnlineStatusRequest{UserUUID: "u2"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayDeviceServiceBatchGetOnlineStatus(t *testing.T) {
	t.Run("success_mapping", func(t *testing.T) {
		ts := time.Date(2026, 2, 6, 13, 0, 0, 0, time.UTC).UnixMilli()
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			batchGetOnlineStatusFn: func(_ context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
				assert.Equal(t, []string{"u1", "u2"}, req.UserUuids)
				return &authpb.BatchGetOnlineStatusResponse{
					Users: []*authpb.OnlineStatusItem{
						{UserUuid: "u1", IsOnline: true, LastSeenAt: ts},
						{UserUuid: "u2", IsOnline: false, LastSeenAt: 0},
					},
				}, nil
			},
		})
		resp, err := svc.BatchGetOnlineStatus(context.Background(), &dto.BatchGetOnlineStatusRequest{UserUUIDs: []string{"u1", "u2"}})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Users, 2)
		assert.Equal(t, "u1", resp.Users[0].UserUUID)
		assert.Equal(t, util.FormatUnixMilliRFC3339(ts), resp.Users[0].LastSeenAt)
		assert.Equal(t, "", resp.Users[1].LastSeenAt)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc failed")
		svc := NewDeviceService(&fakeGatewayDeviceClient{
			batchGetOnlineStatusFn: func(_ context.Context, _ *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := svc.BatchGetOnlineStatus(context.Background(), &dto.BatchGetOnlineStatusRequest{UserUUIDs: []string{"u1"}})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}
