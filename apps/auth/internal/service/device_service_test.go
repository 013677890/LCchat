package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/presence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDeviceRepoForPresence 只实现在线状态路径用到的方法，其余方法沿用内嵌接口（调用即 panic）。
type fakeDeviceRepoForPresence struct {
	repository.IDeviceRepository
	batchGetOnlineStatusFn func(context.Context, []string) (map[string][]*model.DeviceSession, error)
}

func (f *fakeDeviceRepoForPresence) BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
	if f.batchGetOnlineStatusFn == nil {
		return map[string][]*model.DeviceSession{}, nil
	}
	return f.batchGetOnlineStatusFn(ctx, userUUIDs)
}

// fakePresenceRepo 以静态数据驱动 presence 读取结果。
type fakePresenceRepo struct {
	routesByUser map[string][]presence.DeviceRoute
	err          error
}

func (f *fakePresenceRepo) ListUserRoutes(_ context.Context, userUUID string) ([]presence.DeviceRoute, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.routesByUser[userUUID], nil
}

func (f *fakePresenceRepo) ListUsersRoutes(_ context.Context, userUUIDs []string) (map[string][]presence.DeviceRoute, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := make(map[string][]presence.DeviceRoute)
	for _, userUUID := range userUUIDs {
		if routes := f.routesByUser[userUUID]; len(routes) > 0 {
			result[userUUID] = routes
		}
	}
	return result, nil
}

func deviceSession(deviceID, platform string, status int8, updatedAt time.Time) *model.DeviceSession {
	return &model.DeviceSession{
		DeviceId:  deviceID,
		Platform:  platform,
		Status:    status,
		UpdatedAt: updatedAt,
	}
}

func TestDeviceServiceGetOnlineStatusPresence(t *testing.T) {
	now := time.Now()
	baseSessions := map[string][]*model.DeviceSession{
		"u1": {
			deviceSession("d1", "Web", model.DeviceStatusOnline, now.Add(-time.Hour)),
			deviceSession("d2", "iOS", model.DeviceStatusOffline, now.Add(-30*time.Minute)),
		},
	}
	repoFn := func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
		return baseSessions, nil
	}

	t.Run("routed_device_online_with_platform_and_last_seen", func(t *testing.T) {
		routeActiveAt := now.Add(-10 * time.Second)
		svc := NewDeviceService(
			&fakeDeviceRepoForPresence{batchGetOnlineStatusFn: repoFn},
			&fakePresenceRepo{routesByUser: map[string][]presence.DeviceRoute{
				"u1": {{UserUUID: "u1", DeviceID: "d1", ConnectGRPCAddr: "connect:9091", LastActiveMs: routeActiveAt.UnixMilli()}},
			}},
		)

		resp, err := svc.GetOnlineStatus(context.Background(), &authpb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		require.NotNil(t, resp.GetStatus())
		assert.True(t, resp.GetStatus().GetIsOnline())
		assert.Equal(t, []string{"Web"}, resp.GetStatus().GetOnlinePlatforms())
		// last_seen 取路由心跳时间（比会话迁移时刻更新）。
		assert.Equal(t, routeActiveAt.Unix()*1000, resp.GetStatus().GetLastSeenAt())
	})

	t.Run("no_route_means_offline_with_transition_last_seen", func(t *testing.T) {
		svc := NewDeviceService(
			&fakeDeviceRepoForPresence{batchGetOnlineStatusFn: repoFn},
			&fakePresenceRepo{},
		)

		resp, err := svc.GetOnlineStatus(context.Background(), &authpb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		assert.False(t, resp.GetStatus().GetIsOnline())
		// 离线 last_seen 回退到最近一次会话状态迁移时刻（d2 的 -30min）。
		assert.Equal(t, now.Add(-30*time.Minute).Unix()*1000, resp.GetStatus().GetLastSeenAt())
	})

	t.Run("kicked_session_excluded_even_with_fresh_route", func(t *testing.T) {
		kickedSessions := map[string][]*model.DeviceSession{
			"u1": {deviceSession("d1", "Web", model.DeviceStatusKicked, now.Add(-time.Minute))},
		}
		svc := NewDeviceService(
			&fakeDeviceRepoForPresence{batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
				return kickedSessions, nil
			}},
			&fakePresenceRepo{routesByUser: map[string][]presence.DeviceRoute{
				"u1": {{UserUUID: "u1", DeviceID: "d1", ConnectGRPCAddr: "connect:9091", LastActiveMs: now.UnixMilli()}},
			}},
		)

		resp, err := svc.GetOnlineStatus(context.Background(), &authpb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		// 被踢终态设备即使路由仍新鲜也不展示为在线。
		assert.False(t, resp.GetStatus().GetIsOnline())
	})

	t.Run("presence_read_error_degrades_to_offline", func(t *testing.T) {
		svc := NewDeviceService(
			&fakeDeviceRepoForPresence{batchGetOnlineStatusFn: repoFn},
			&fakePresenceRepo{err: errors.New("redis down")},
		)

		resp, err := svc.GetOnlineStatus(context.Background(), &authpb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		assert.False(t, resp.GetStatus().GetIsOnline())
	})

	t.Run("param_error", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepoForPresence{}, &fakePresenceRepo{})
		_, err := svc.GetOnlineStatus(context.Background(), nil)
		require.Error(t, err)
		assert.Equal(t, consts.CodeParamError, apperr.Code(err))
	})
}

func TestDeviceServiceBatchGetOnlineStatusPresence(t *testing.T) {
	now := time.Now()
	svc := NewDeviceService(
		&fakeDeviceRepoForPresence{batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
			return map[string][]*model.DeviceSession{
				"u1": {deviceSession("d1", "Web", model.DeviceStatusOnline, now.Add(-time.Hour))},
				"u2": {deviceSession("d2", "iOS", model.DeviceStatusOffline, now.Add(-time.Minute))},
			}, nil
		}},
		&fakePresenceRepo{routesByUser: map[string][]presence.DeviceRoute{
			"u1": {{UserUUID: "u1", DeviceID: "d1", ConnectGRPCAddr: "connect:9091", LastActiveMs: now.UnixMilli()}},
		}},
	)

	resp, err := svc.BatchGetOnlineStatus(context.Background(), &authpb.BatchGetOnlineStatusRequest{UserUuids: []string{"u1", "u2", "u3"}})
	require.NoError(t, err)
	require.Len(t, resp.GetUsers(), 3)

	// 结果按请求顺序返回：u1 在线；u2 有会话但无路由=离线；u3 无任何数据=离线。
	assert.Equal(t, "u1", resp.GetUsers()[0].GetUserUuid())
	assert.True(t, resp.GetUsers()[0].GetIsOnline())
	assert.Equal(t, "u2", resp.GetUsers()[1].GetUserUuid())
	assert.False(t, resp.GetUsers()[1].GetIsOnline())
	assert.Equal(t, "u3", resp.GetUsers()[2].GetUserUuid())
	assert.False(t, resp.GetUsers()[2].GetIsOnline())
}

func TestDeviceServiceGetDeviceListLastSeen(t *testing.T) {
	now := time.Now()
	routeActiveAt := now.Add(-5 * time.Second)
	svc := NewDeviceService(
		&fakeDeviceRepoForPresence{batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
			return map[string][]*model.DeviceSession{
				"u1": {
					deviceSession("d1", "Web", model.DeviceStatusOnline, now.Add(-time.Hour)),
					deviceSession("d2", "iOS", model.DeviceStatusOffline, now.Add(-30*time.Minute)),
				},
			}, nil
		}},
		&fakePresenceRepo{routesByUser: map[string][]presence.DeviceRoute{
			"u1": {{UserUUID: "u1", DeviceID: "d1", ConnectGRPCAddr: "connect:9091", LastActiveMs: routeActiveAt.UnixMilli()}},
		}},
	)

	ctx := context.WithValue(context.Background(), "user_uuid", "u1")
	resp, err := svc.GetDeviceList(ctx, &authpb.GetDeviceListRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetDevices(), 2)

	// d1 在路由中，last_seen 取心跳时间且排在最前；d2 回退到会话迁移时刻。
	assert.Equal(t, "d1", resp.GetDevices()[0].GetDeviceId())
	assert.Equal(t, routeActiveAt.Unix()*1000, resp.GetDevices()[0].GetLastSeenAt())
	assert.Equal(t, "d2", resp.GetDevices()[1].GetDeviceId())
	assert.Equal(t, now.Add(-30*time.Minute).Unix()*1000, resp.GetDevices()[1].GetLastSeenAt())
}
