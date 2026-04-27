package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDeviceHandlerService struct {
	getDeviceListFn        func(context.Context, *pb.GetDeviceListRequest) (*pb.GetDeviceListResponse, error)
	kickDeviceFn           func(context.Context, *pb.KickDeviceRequest) error
	getOnlineStatusFn      func(context.Context, *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error)
	batchGetOnlineStatusFn func(context.Context, *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error)
	updateDeviceActiveFn   func(context.Context, *pb.UpdateDeviceActiveRequest) error
	updateDeviceStatusFn   func(context.Context, *pb.UpdateDeviceStatusRequest) error
}

var _ service.IDeviceService = (*fakeDeviceHandlerService)(nil)

func (f *fakeDeviceHandlerService) GetDeviceList(ctx context.Context, req *pb.GetDeviceListRequest) (*pb.GetDeviceListResponse, error) {
	if f.getDeviceListFn == nil {
		return &pb.GetDeviceListResponse{}, nil
	}
	return f.getDeviceListFn(ctx, req)
}

func (f *fakeDeviceHandlerService) KickDevice(ctx context.Context, req *pb.KickDeviceRequest) error {
	if f.kickDeviceFn == nil {
		return nil
	}
	return f.kickDeviceFn(ctx, req)
}

func (f *fakeDeviceHandlerService) GetOnlineStatus(ctx context.Context, req *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error) {
	if f.getOnlineStatusFn == nil {
		return &pb.GetOnlineStatusResponse{}, nil
	}
	return f.getOnlineStatusFn(ctx, req)
}

func (f *fakeDeviceHandlerService) BatchGetOnlineStatus(ctx context.Context, req *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error) {
	if f.batchGetOnlineStatusFn == nil {
		return &pb.BatchGetOnlineStatusResponse{}, nil
	}
	return f.batchGetOnlineStatusFn(ctx, req)
}

func (f *fakeDeviceHandlerService) UpdateDeviceStatus(ctx context.Context, req *pb.UpdateDeviceStatusRequest) error {
	if f.updateDeviceStatusFn == nil {
		return nil
	}
	return f.updateDeviceStatusFn(ctx, req)
}

func (f *fakeDeviceHandlerService) UpdateDeviceActive(ctx context.Context, req *pb.UpdateDeviceActiveRequest) error {
	if f.updateDeviceActiveFn == nil {
		return nil
	}
	return f.updateDeviceActiveFn(ctx, req)
}

func TestUserDeviceHandlerGetDeviceList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.GetDeviceListResponse{Devices: []*pb.DeviceItem{{DeviceId: "d1"}}}
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			getDeviceListFn: func(_ context.Context, _ *pb.GetDeviceListRequest) (*pb.GetDeviceListResponse, error) {
				return want, nil
			},
		})
		resp, err := h.GetDeviceList(context.Background(), &pb.GetDeviceListRequest{})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("get failed")
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			getDeviceListFn: func(_ context.Context, _ *pb.GetDeviceListRequest) (*pb.GetDeviceListResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := h.GetDeviceList(context.Background(), &pb.GetDeviceListRequest{})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserDeviceHandlerKickDevice(t *testing.T) {
	t.Run("success_empty_response_contract", func(t *testing.T) {
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			kickDeviceFn: func(_ context.Context, req *pb.KickDeviceRequest) error {
				require.Equal(t, "d1", req.DeviceId)
				return nil
			},
		})
		resp, err := h.KickDevice(context.Background(), &pb.KickDeviceRequest{DeviceId: "d1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.KickDeviceResponse{}, resp)
	})

	t.Run("error_empty_response_contract", func(t *testing.T) {
		wantErr := errors.New("kick failed")
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			kickDeviceFn: func(_ context.Context, _ *pb.KickDeviceRequest) error {
				return wantErr
			},
		})
		resp, err := h.KickDevice(context.Background(), &pb.KickDeviceRequest{DeviceId: "d1"})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.KickDeviceResponse{}, resp)
	})
}

func TestUserDeviceHandlerGetOnlineStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.GetOnlineStatusResponse{
			Status: &pb.OnlineStatus{UserUuid: "u2", IsOnline: true},
		}
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			getOnlineStatusFn: func(_ context.Context, req *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error) {
				require.Equal(t, "u2", req.UserUuid)
				return want, nil
			},
		})
		resp, err := h.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: "u2"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("get status failed")
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			getOnlineStatusFn: func(_ context.Context, _ *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := h.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: "u2"})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserDeviceHandlerBatchGetOnlineStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.BatchGetOnlineStatusResponse{
			Users: []*pb.OnlineStatusItem{{UserUuid: "u1", IsOnline: true}},
		}
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			batchGetOnlineStatusFn: func(_ context.Context, req *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error) {
				require.Equal(t, []string{"u1"}, req.UserUuids)
				return want, nil
			},
		})
		resp, err := h.BatchGetOnlineStatus(context.Background(), &pb.BatchGetOnlineStatusRequest{UserUuids: []string{"u1"}})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("batch failed")
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			batchGetOnlineStatusFn: func(_ context.Context, _ *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := h.BatchGetOnlineStatus(context.Background(), &pb.BatchGetOnlineStatusRequest{UserUuids: []string{"u1"}})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestUserDeviceHandlerUpdateDeviceStatus(t *testing.T) {
	t.Run("success_empty_response_contract", func(t *testing.T) {
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			updateDeviceStatusFn: func(_ context.Context, req *pb.UpdateDeviceStatusRequest) error {
				require.Equal(t, "u1", req.UserUuid)
				require.Equal(t, "d1", req.DeviceId)
				require.Equal(t, int32(0), req.Status)
				return nil
			},
		})
		resp, err := h.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   0,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.UpdateDeviceStatusResponse{}, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("update failed")
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			updateDeviceStatusFn: func(_ context.Context, _ *pb.UpdateDeviceStatusRequest) error {
				return wantErr
			},
		})
		resp, err := h.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   1,
		})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.UpdateDeviceStatusResponse{}, resp)
	})
}

func TestUserDeviceHandlerUpdateDeviceActive(t *testing.T) {
	t.Run("success_empty_response_contract", func(t *testing.T) {
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			updateDeviceActiveFn: func(_ context.Context, req *pb.UpdateDeviceActiveRequest) error {
				require.Len(t, req.Items, 2)
				require.Equal(t, "u1", req.Items[0].UserUuid)
				require.Equal(t, "d1", req.Items[0].DeviceId)
				require.Equal(t, "u1", req.Items[1].UserUuid)
				require.Equal(t, "d2", req.Items[1].DeviceId)
				return nil
			},
		})
		resp, err := h.UpdateDeviceActive(context.Background(), &pb.UpdateDeviceActiveRequest{
			Items: []*pb.UpdateDeviceActiveItem{
				{UserUuid: "u1", DeviceId: "d1"},
				{UserUuid: "u1", DeviceId: "d2"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.UpdateDeviceActiveResponse{}, resp)
	})

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("update active failed")
		h := NewDeviceHandler(&fakeDeviceHandlerService{
			updateDeviceActiveFn: func(_ context.Context, _ *pb.UpdateDeviceActiveRequest) error {
				return wantErr
			},
		})
		resp, err := h.UpdateDeviceActive(context.Background(), &pb.UpdateDeviceActiveRequest{
			Items: []*pb.UpdateDeviceActiveItem{
				{UserUuid: "u1", DeviceId: "d1"},
			},
		})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.UpdateDeviceActiveResponse{}, resp)
	})
}
