package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var userDeviceLoggerOnce sync.Once

func initUserDeviceTestLogger() {
	userDeviceLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

func withDeviceContext(userUUID, deviceID string) context.Context {
	ctx := context.Background()
	if userUUID != "" {
		ctx = context.WithValue(ctx, util.ContextKeyUserUUID, userUUID)
	}
	if deviceID != "" {
		ctx = context.WithValue(ctx, util.ContextKeyDeviceID, deviceID)
	}
	return ctx
}

func requireDeviceStatusCode(t *testing.T, err error, wantGRPCCode codes.Code, wantBizCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantBizCode, apperr.Code(err))

	stErr := apperr.ToStatus(err)
	converted := apperr.FromStatus(stErr)
	require.Equal(t, wantBizCode, apperr.Code(converted))
	st, ok := status.FromError(stErr)
	require.True(t, ok)
	require.Equal(t, wantGRPCCode, st.Code())
}

type fakeDeviceRepository struct {
	createFn               func(context.Context, *model.DeviceSession) error
	getByUserUUIDFn        func(context.Context, string) ([]*model.DeviceSession, error)
	getByDeviceIDFn        func(context.Context, string, string) (*model.DeviceSession, error)
	upsertSessionFn        func(context.Context, *model.DeviceSession) error
	touchDeviceInfoTTLFn   func(context.Context, string) error
	getActiveTimestampsFn  func(context.Context, string, []string) (map[string]int64, error)
	batchGetActiveTsFn     func(context.Context, map[string][]string) (map[string]map[string]int64, error)
	batchGetLastSeenTsFn   func(context.Context, []string) (map[string]int64, error)
	setActiveTimestampFn   func(context.Context, string, string, int64) error
	batchSetActiveTsFn     func(context.Context, []repository.DeviceActiveItem, int64) error
	updateOnlineStatusFn   func(context.Context, string, string, int8) error
	updateLastSeenFn       func(context.Context, string, string) error
	deleteFn               func(context.Context, string, string) error
	getOnlineDevicesFn     func(context.Context, string) ([]*model.DeviceSession, error)
	batchGetOnlineStatusFn func(context.Context, []string) (map[string][]*model.DeviceSession, error)
	updateTokenFn          func(context.Context, string, string, string, string, *time.Time) error
	deleteByUserUUIDFn     func(context.Context, string) error
	storeAccessTokenFn     func(context.Context, string, string, string, time.Duration) error
	storeRefreshTokenFn    func(context.Context, string, string, string, time.Duration) error
	verifyAccessTokenFn    func(context.Context, string, string, string) (bool, error)
	getRefreshTokenFn      func(context.Context, string, string) (string, error)
	deleteTokensFn         func(context.Context, string, string) error
}

func (f *fakeDeviceRepository) Create(ctx context.Context, session *model.DeviceSession) error {
	if f.createFn == nil {
		return nil
	}
	return f.createFn(ctx, session)
}

func (f *fakeDeviceRepository) GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	if f.getByUserUUIDFn == nil {
		return nil, nil
	}
	return f.getByUserUUIDFn(ctx, userUUID)
}

func (f *fakeDeviceRepository) GetByDeviceID(ctx context.Context, userUUID, deviceID string) (*model.DeviceSession, error) {
	if f.getByDeviceIDFn == nil {
		return nil, repository.ErrRecordNotFound
	}
	return f.getByDeviceIDFn(ctx, userUUID, deviceID)
}

func (f *fakeDeviceRepository) UpsertSession(ctx context.Context, session *model.DeviceSession) error {
	if f.upsertSessionFn == nil {
		return nil
	}
	return f.upsertSessionFn(ctx, session)
}

func (f *fakeDeviceRepository) TouchDeviceInfoTTL(ctx context.Context, userUUID string) error {
	if f.touchDeviceInfoTTLFn == nil {
		return nil
	}
	return f.touchDeviceInfoTTLFn(ctx, userUUID)
}

func (f *fakeDeviceRepository) GetActiveTimestamps(ctx context.Context, userUUID string, deviceIDs []string) (map[string]int64, error) {
	if f.getActiveTimestampsFn == nil {
		return map[string]int64{}, nil
	}
	return f.getActiveTimestampsFn(ctx, userUUID, deviceIDs)
}

func (f *fakeDeviceRepository) BatchGetActiveTimestamps(ctx context.Context, userDeviceIDs map[string][]string) (map[string]map[string]int64, error) {
	if f.batchGetActiveTsFn == nil {
		return map[string]map[string]int64{}, nil
	}
	return f.batchGetActiveTsFn(ctx, userDeviceIDs)
}

func (f *fakeDeviceRepository) BatchGetLastSeenTimestamps(ctx context.Context, userUUIDs []string) (map[string]int64, error) {
	if f.batchGetLastSeenTsFn == nil {
		return map[string]int64{}, nil
	}
	return f.batchGetLastSeenTsFn(ctx, userUUIDs)
}

func (f *fakeDeviceRepository) SetActiveTimestamp(ctx context.Context, userUUID, deviceID string, ts int64) error {
	if f.setActiveTimestampFn == nil {
		return nil
	}
	return f.setActiveTimestampFn(ctx, userUUID, deviceID, ts)
}

func (f *fakeDeviceRepository) BatchSetActiveTimestamps(ctx context.Context, items []repository.DeviceActiveItem, ts int64) error {
	if f.batchSetActiveTsFn == nil {
		return nil
	}
	return f.batchSetActiveTsFn(ctx, items, ts)
}

func (f *fakeDeviceRepository) UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error {
	if f.updateOnlineStatusFn == nil {
		return nil
	}
	return f.updateOnlineStatusFn(ctx, userUUID, deviceID, status)
}

func (f *fakeDeviceRepository) UpdateLastSeen(ctx context.Context, userUUID, deviceID string) error {
	if f.updateLastSeenFn == nil {
		return nil
	}
	return f.updateLastSeenFn(ctx, userUUID, deviceID)
}

func (f *fakeDeviceRepository) Delete(ctx context.Context, userUUID, deviceID string) error {
	if f.deleteFn == nil {
		return nil
	}
	return f.deleteFn(ctx, userUUID, deviceID)
}

func (f *fakeDeviceRepository) GetOnlineDevices(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	if f.getOnlineDevicesFn == nil {
		return nil, nil
	}
	return f.getOnlineDevicesFn(ctx, userUUID)
}

func (f *fakeDeviceRepository) BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
	if f.batchGetOnlineStatusFn == nil {
		return map[string][]*model.DeviceSession{}, nil
	}
	return f.batchGetOnlineStatusFn(ctx, userUUIDs)
}

func (f *fakeDeviceRepository) UpdateToken(ctx context.Context, userUUID, deviceID, token, refreshToken string, expireAt *time.Time) error {
	if f.updateTokenFn == nil {
		return nil
	}
	return f.updateTokenFn(ctx, userUUID, deviceID, token, refreshToken, expireAt)
}

func (f *fakeDeviceRepository) DeleteByUserUUID(ctx context.Context, userUUID string) error {
	if f.deleteByUserUUIDFn == nil {
		return nil
	}
	return f.deleteByUserUUIDFn(ctx, userUUID)
}

func (f *fakeDeviceRepository) StoreAccessToken(ctx context.Context, userUUID, deviceID, accessToken string, expireDuration time.Duration) error {
	if f.storeAccessTokenFn == nil {
		return nil
	}
	return f.storeAccessTokenFn(ctx, userUUID, deviceID, accessToken, expireDuration)
}

func (f *fakeDeviceRepository) StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error {
	if f.storeRefreshTokenFn == nil {
		return nil
	}
	return f.storeRefreshTokenFn(ctx, userUUID, deviceID, refreshToken, expireDuration)
}

func (f *fakeDeviceRepository) VerifyAccessToken(ctx context.Context, userUUID, deviceID, accessToken string) (bool, error) {
	if f.verifyAccessTokenFn == nil {
		return false, nil
	}
	return f.verifyAccessTokenFn(ctx, userUUID, deviceID, accessToken)
}

func (f *fakeDeviceRepository) GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error) {
	if f.getRefreshTokenFn == nil {
		return "", nil
	}
	return f.getRefreshTokenFn(ctx, userUUID, deviceID)
}

func (f *fakeDeviceRepository) DeleteTokens(ctx context.Context, userUUID, deviceID string) error {
	if f.deleteTokensFn == nil {
		return nil
	}
	return f.deleteTokensFn(ctx, userUUID, deviceID)
}

func TestUserDeviceServiceGetDeviceList(t *testing.T) {
	initUserDeviceTestLogger()

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		resp, err := svc.GetDeviceList(context.Background(), &pb.GetDeviceListRequest{})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("batch_get_status_error", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
				assert.Equal(t, []string{"u1"}, userUUIDs)
				return nil, errors.New("redis failed")
			},
		})
		resp, err := svc.GetDeviceList(withDeviceContext("u1", "d1"), &pb.GetDeviceListRequest{})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_sort_and_current_device", func(t *testing.T) {
		nowSec := time.Now().Unix()

		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
				assert.Equal(t, []string{"u1"}, userUUIDs)
				return map[string][]*model.DeviceSession{
					"u1": {
						{UserUuid: "u1", DeviceId: "d2", DeviceName: "Device 2", Platform: "ios", AppVersion: "1.0", Status: model.DeviceStatusOnline},
						nil,
						{UserUuid: "u1", DeviceId: "d1", DeviceName: "Device 1", Platform: "android", AppVersion: "1.1", Status: model.DeviceStatusOffline},
					},
				}, nil
			},
			getActiveTimestampsFn: func(_ context.Context, userUUID string, deviceIDs []string) (map[string]int64, error) {
				assert.Equal(t, "u1", userUUID)
				assert.ElementsMatch(t, []string{"d2", "d1"}, deviceIDs)
				return map[string]int64{
					"d2": nowSec - 30,
				}, nil
			},
		})

		resp, err := svc.GetDeviceList(withDeviceContext("u1", "d2"), &pb.GetDeviceListRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Devices, 2)

		// d2 有活跃时间，d1 无活跃时间（0），排序应 d2 在前。
		assert.Equal(t, "d2", resp.Devices[0].DeviceId)
		assert.Equal(t, "d1", resp.Devices[1].DeviceId)
		assert.True(t, resp.Devices[0].IsCurrentDevice)
		assert.False(t, resp.Devices[1].IsCurrentDevice)
		assert.Greater(t, resp.Devices[0].LastSeenAt, int64(0))
		assert.Equal(t, int64(0), resp.Devices[1].LastSeenAt)
	})

	t.Run("active_time_read_or_write_error_does_not_fail", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
				return map[string][]*model.DeviceSession{
					"u1": {
						{UserUuid: "u1", DeviceId: "d1", DeviceName: "Device 1", Platform: "ios", AppVersion: "1.0", Status: model.DeviceStatusOnline},
					},
				}, nil
			},
			getActiveTimestampsFn: func(_ context.Context, _ string, _ []string) (map[string]int64, error) {
				return nil, errors.New("active redis down")
			},
		})

		resp, err := svc.GetDeviceList(withDeviceContext("u1", "d1"), &pb.GetDeviceListRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Devices, 1)
		assert.Equal(t, "d1", resp.Devices[0].DeviceId)
		assert.Equal(t, int64(0), resp.Devices[0].LastSeenAt)
	})
}

func TestUserDeviceServiceKickDevice(t *testing.T) {
	initUserDeviceTestLogger()

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.KickDevice(context.Background(), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("invalid_request", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})

		err := svc.KickDevice(withDeviceContext("u1", "d2"), nil)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)

		err = svc.KickDevice(withDeviceContext("u1", "d2"), &pb.KickDeviceRequest{DeviceId: ""})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("cannot_kick_current_device", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.KickDevice(withDeviceContext("u1", "d1"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.FailedPrecondition, consts.CodeCannotKickCurrent)
	})

	t.Run("get_device_errors", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return nil, repository.ErrRecordNotFound
			},
		})
		err := svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.NotFound, consts.CodeDeviceNotFound)

		svc = NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return nil, errors.New("db failed")
			},
		})
		err = svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)

		svc = NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return nil, nil
			},
		})
		err = svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.NotFound, consts.CodeDeviceNotFound)
	})

	t.Run("delete_token_and_update_errors", func(t *testing.T) {
		baseSession := &model.DeviceSession{UserUuid: "u1", DeviceId: "d1", Status: model.DeviceStatusOnline}

		svc := NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return baseSession, nil
			},
			deleteTokensFn: func(_ context.Context, _, _ string) error {
				return errors.New("redis failed")
			},
		})
		err := svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)

		svc = NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return baseSession, nil
			},
			deleteTokensFn: func(_ context.Context, _, _ string) error { return nil },
			updateOnlineStatusFn: func(_ context.Context, _, _ string, _ int8) error {
				return repository.ErrRecordNotFound
			},
		})
		err = svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.NotFound, consts.CodeDeviceNotFound)

		svc = NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return baseSession, nil
			},
			deleteTokensFn: func(_ context.Context, _, _ string) error { return nil },
			updateOnlineStatusFn: func(_ context.Context, _, _ string, _ int8) error {
				return errors.New("db failed")
			},
		})
		err = svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"})
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_paths", func(t *testing.T) {
		var updateCalls int
		svc := NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return &model.DeviceSession{UserUuid: "u1", DeviceId: "d1", Status: model.DeviceStatusOnline}, nil
			},
			deleteTokensFn: func(_ context.Context, userUUID, deviceID string) error {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, "d1", deviceID)
				return nil
			},
			updateOnlineStatusFn: func(_ context.Context, userUUID, deviceID string, status int8) error {
				updateCalls++
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, "d1", deviceID)
				assert.Equal(t, model.DeviceStatusKicked, status)
				return nil
			},
		})
		require.NoError(t, svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"}))
		assert.Equal(t, 1, updateCalls)

		updateCalls = 0
		svc = NewDeviceService(&fakeDeviceRepository{
			getByDeviceIDFn: func(_ context.Context, _, _ string) (*model.DeviceSession, error) {
				return &model.DeviceSession{UserUuid: "u1", DeviceId: "d1", Status: model.DeviceStatusLoggedOut}, nil
			},
			deleteTokensFn: func(_ context.Context, _, _ string) error { return nil },
			updateOnlineStatusFn: func(_ context.Context, _, _ string, _ int8) error {
				updateCalls++
				return nil
			},
		})
		require.NoError(t, svc.KickDevice(withDeviceContext("u1", "d9"), &pb.KickDeviceRequest{DeviceId: "d1"}))
		assert.Equal(t, 0, updateCalls)
	})
}

func TestUserDeviceServiceGetOnlineStatus(t *testing.T) {
	initUserDeviceTestLogger()

	t.Run("invalid_request", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})

		resp, err := svc.GetOnlineStatus(context.Background(), nil)
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)

		resp, err = svc.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: ""})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("batch_status_error", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
				assert.Equal(t, []string{"u1"}, userUUIDs)
				return nil, errors.New("db failed")
			},
		})
		resp, err := svc.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("no_sessions", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
				return map[string][]*model.DeviceSession{}, nil
			},
		})
		resp, err := svc.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Status)
		assert.False(t, resp.Status.IsOnline)
		assert.Equal(t, int64(0), resp.Status.LastSeenAt)
		assert.Empty(t, resp.Status.OnlinePlatforms)
	})

	t.Run("active_time_error_degrade_offline", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
				return map[string][]*model.DeviceSession{
					"u1": {
						{UserUuid: "u1", DeviceId: "d1", Platform: "ios", Status: model.DeviceStatusOnline},
					},
				}, nil
			},
			getActiveTimestampsFn: func(_ context.Context, _ string, _ []string) (map[string]int64, error) {
				return nil, errors.New("redis failed")
			},
		})
		resp, err := svc.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Status.IsOnline)
		assert.Equal(t, int64(0), resp.Status.LastSeenAt)
		assert.Empty(t, resp.Status.OnlinePlatforms)
	})

	t.Run("mixed_sessions_online_window_and_platforms", func(t *testing.T) {
		now := time.Now().Unix()
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, _ []string) (map[string][]*model.DeviceSession, error) {
				return map[string][]*model.DeviceSession{
					"u1": {
						{UserUuid: "u1", DeviceId: "d1", Platform: "ios", Status: model.DeviceStatusOnline},
						{UserUuid: "u1", DeviceId: "d2", Platform: "android", Status: model.DeviceStatusOnline},
						{UserUuid: "u1", DeviceId: "d3", Platform: "web", Status: model.DeviceStatusOffline},
					},
				}, nil
			},
			getActiveTimestampsFn: func(_ context.Context, _ string, deviceIDs []string) (map[string]int64, error) {
				assert.ElementsMatch(t, []string{"d1", "d2", "d3"}, deviceIDs)
				return map[string]int64{
					"d1": now - 30,   // 在线
					"d2": now - 2000, // 超窗口
					"d3": now - 10,   // 离线状态，但用于 lastSeen
				}, nil
			},
			batchGetLastSeenTsFn: func(_ context.Context, userUUIDs []string) (map[string]int64, error) {
				assert.Equal(t, []string{"u1"}, userUUIDs)
				return map[string]int64{"u1": now - 10}, nil
			},
		})

		resp, err := svc.GetOnlineStatus(context.Background(), &pb.GetOnlineStatusRequest{UserUuid: "u1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Status)
		assert.True(t, resp.Status.IsOnline)
		assert.Equal(t, (now-10)*1000, resp.Status.LastSeenAt)
		assert.Equal(t, []string{"ios"}, resp.Status.OnlinePlatforms)
	})
}

func TestUserDeviceServiceBatchGetOnlineStatus(t *testing.T) {
	initUserDeviceTestLogger()

	t.Run("invalid_request", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})

		resp, err := svc.BatchGetOnlineStatus(context.Background(), nil)
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)

		resp, err = svc.BatchGetOnlineStatus(context.Background(), &pb.BatchGetOnlineStatusRequest{UserUuids: []string{}})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)

		tooMany := make([]string, 101)
		for i := range tooMany {
			tooMany[i] = "u" + strconv.Itoa(i)
		}
		resp, err = svc.BatchGetOnlineStatus(context.Background(), &pb.BatchGetOnlineStatusRequest{UserUuids: tooMany})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)

		resp, err = svc.BatchGetOnlineStatus(context.Background(), &pb.BatchGetOnlineStatusRequest{UserUuids: []string{"u1", ""}})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("repo_error", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
				assert.Equal(t, []string{"u1", "u2"}, userUUIDs)
				return nil, errors.New("db failed")
			},
		})

		resp, err := svc.BatchGetOnlineStatus(context.Background(), &pb.BatchGetOnlineStatusRequest{UserUuids: []string{"u1", "u2"}})
		require.Nil(t, resp)
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success_dedup_order_and_degrade", func(t *testing.T) {
		now := time.Now().Unix()
		var capturedUsers []string
		svc := NewDeviceService(&fakeDeviceRepository{
			batchGetOnlineStatusFn: func(_ context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
				capturedUsers = append(capturedUsers, userUUIDs...)
				return map[string][]*model.DeviceSession{
					"u1": {
						{UserUuid: "u1", DeviceId: "d1", Platform: "ios", Status: model.DeviceStatusOnline},
					},
					"u2": {
						{UserUuid: "u2", DeviceId: "d2", Platform: "android", Status: model.DeviceStatusOnline},
					},
				}, nil
			},
			batchGetActiveTsFn: func(_ context.Context, userDeviceIDs map[string][]string) (map[string]map[string]int64, error) {
				assert.Equal(t, []string{"d1"}, userDeviceIDs["u1"])
				assert.Equal(t, []string{"d2"}, userDeviceIDs["u2"])
				_, exists := userDeviceIDs["u3"]
				assert.False(t, exists)
				return map[string]map[string]int64{
					"u1": {"d1": now - 10},
				}, nil
			},
			batchGetLastSeenTsFn: func(_ context.Context, userUUIDs []string) (map[string]int64, error) {
				assert.Equal(t, []string{"u1", "u2", "u3"}, userUUIDs)
				return map[string]int64{
					"u1": now - 10,
				}, nil
			},
		})

		req := &pb.BatchGetOnlineStatusRequest{UserUuids: []string{"u1", "u1", "u2", "u3"}}
		resp, err := svc.BatchGetOnlineStatus(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Users, 4)

		// 去重查询（保持首次出现顺序）
		assert.Equal(t, []string{"u1", "u2", "u3"}, capturedUsers)

		// 响应按请求顺序返回，并保留重复项
		assert.Equal(t, "u1", resp.Users[0].UserUuid)
		assert.True(t, resp.Users[0].IsOnline)
		assert.Equal(t, (now-10)*1000, resp.Users[0].LastSeenAt)

		assert.Equal(t, "u1", resp.Users[1].UserUuid)
		assert.True(t, resp.Users[1].IsOnline)
		assert.Equal(t, (now-10)*1000, resp.Users[1].LastSeenAt)

		assert.Equal(t, "u2", resp.Users[2].UserUuid)
		assert.False(t, resp.Users[2].IsOnline)
		assert.Equal(t, int64(0), resp.Users[2].LastSeenAt)

		assert.Equal(t, "u3", resp.Users[3].UserUuid)
		assert.False(t, resp.Users[3].IsOnline)
		assert.Equal(t, int64(0), resp.Users[3].LastSeenAt)
	})
}

func TestUserDeviceServiceUpdateDeviceStatus(t *testing.T) {
	initUserDeviceTestLogger()

	t.Run("nil_request", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceStatus(context.Background(), nil)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("empty_user_uuid", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "",
			DeviceId: "d1",
			Status:   0,
		})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("empty_device_id", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "",
			Status:   0,
		})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("invalid_status_kicked", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   int32(model.DeviceStatusKicked), // 3, 不允许
		})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("invalid_status_logged_out", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   int32(model.DeviceStatusLoggedOut), // 2, 不允许
		})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("success_online", func(t *testing.T) {
		var captured struct {
			userUUID string
			deviceID string
			status   int8
		}
		svc := NewDeviceService(&fakeDeviceRepository{
			updateOnlineStatusFn: func(_ context.Context, userUUID, deviceID string, status int8) error {
				captured.userUUID = userUUID
				captured.deviceID = deviceID
				captured.status = status
				return nil
			},
		})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   0, // Online
		})
		require.NoError(t, err)
		assert.Equal(t, "u1", captured.userUUID)
		assert.Equal(t, "d1", captured.deviceID)
		assert.Equal(t, model.DeviceStatusOnline, captured.status)
	})

	t.Run("success_offline", func(t *testing.T) {
		var capturedStatus int8
		svc := NewDeviceService(&fakeDeviceRepository{
			updateOnlineStatusFn: func(_ context.Context, _, _ string, status int8) error {
				capturedStatus = status
				return nil
			},
		})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   1, // Offline
		})
		require.NoError(t, err)
		assert.Equal(t, model.DeviceStatusOffline, capturedStatus)
	})

	t.Run("idempotent_device_not_found", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			updateOnlineStatusFn: func(_ context.Context, _, _ string, _ int8) error {
				return repository.ErrRecordNotFound
			},
		})
		// 设备不存在时应返回成功（幂等语义）
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d_gone",
			Status:   1,
		})
		require.NoError(t, err)
	})

	t.Run("repo_error", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			updateOnlineStatusFn: func(_ context.Context, _, _ string, _ int8) error {
				return errors.New("db write failed")
			},
		})
		err := svc.UpdateDeviceStatus(context.Background(), &pb.UpdateDeviceStatusRequest{
			UserUuid: "u1",
			DeviceId: "d1",
			Status:   0,
		})
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})
}

func TestUserDeviceServiceUpdateDeviceActive(t *testing.T) {
	initUserDeviceTestLogger()

	t.Run("nil_request", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceActive(context.Background(), nil)
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("empty_items", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceActive(context.Background(), &pb.UpdateDeviceActiveRequest{Items: []*pb.UpdateDeviceActiveItem{}})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("invalid_item", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{})
		err := svc.UpdateDeviceActive(context.Background(), &pb.UpdateDeviceActiveRequest{
			Items: []*pb.UpdateDeviceActiveItem{
				{UserUuid: "u1", DeviceId: "d1"},
				{UserUuid: "", DeviceId: "d2"},
			},
		})
		requireDeviceStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("repo_error", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchSetActiveTsFn: func(_ context.Context, items []repository.DeviceActiveItem, ts int64) error {
				require.Len(t, items, 1)
				assert.Equal(t, "u1", items[0].UserUUID)
				assert.Equal(t, "d1", items[0].DeviceID)
				assert.Greater(t, ts, int64(0))
				return errors.New("redis write failed")
			},
		})
		err := svc.UpdateDeviceActive(context.Background(), &pb.UpdateDeviceActiveRequest{
			Items: []*pb.UpdateDeviceActiveItem{
				{UserUuid: "u1", DeviceId: "d1"},
			},
		})
		requireDeviceStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success", func(t *testing.T) {
		svc := NewDeviceService(&fakeDeviceRepository{
			batchSetActiveTsFn: func(_ context.Context, items []repository.DeviceActiveItem, ts int64) error {
				assert.Greater(t, ts, int64(0))
				require.Len(t, items, 3)
				got := make(map[string]bool, len(items))
				for _, item := range items {
					got[item.UserUUID+":"+item.DeviceID] = true
				}
				assert.True(t, got["u1:d1"])
				assert.True(t, got["u1:d2"])
				assert.True(t, got["u2:d3"])
				return nil
			},
		})
		err := svc.UpdateDeviceActive(context.Background(), &pb.UpdateDeviceActiveRequest{
			Items: []*pb.UpdateDeviceActiveItem{
				{UserUuid: "u1", DeviceId: "d1"},
				{UserUuid: "u1", DeviceId: "d2"},
				{UserUuid: "u2", DeviceId: "d3"},
			},
		})
		require.NoError(t, err)
	})
}
