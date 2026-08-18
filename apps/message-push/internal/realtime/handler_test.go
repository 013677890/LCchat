package realtime

import (
	"context"
	"errors"
	"sync"
	"testing"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
	"github.com/013677890/LCchat-Backend/pkg/realtimepb"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gozap "go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func init() { logger.ReplaceGlobal(gozap.NewNop()) }

func marshalRealtimeEvent(t *testing.T, event realtimepush.Event) []byte {
	t.Helper()
	data, err := event.Marshal()
	require.NoError(t, err)
	return data
}

type mockRouteRepo struct {
	userRoutes  map[string][]route.DeviceRoute
	usersRoutes map[string][]route.DeviceRoute
	err         error
}

func (m *mockRouteRepo) ListUserRoutes(ctx context.Context, userUUID string) ([]route.DeviceRoute, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]route.DeviceRoute(nil), m.userRoutes[userUUID]...), nil
}

func (m *mockRouteRepo) ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]route.DeviceRoute, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make(map[string][]route.DeviceRoute, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		result[userUUID] = append([]route.DeviceRoute(nil), m.usersRoutes[userUUID]...)
	}
	return result, nil
}

type pushCall struct {
	connectAddr string
	userUUID    string
	deviceID    string
	envelope    *connectpb.MessageEnvelope
}

type mockSender struct {
	mu    sync.Mutex
	calls []pushCall
	err   error
}

func (m *mockSender) PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, pushCall{
		connectAddr: connectAddr,
		userUUID:    userUUID,
		deviceID:    deviceID,
		envelope:    envelope,
	})
	return m.err
}

type mockGroupFetcher struct {
	members []string
	admins  []string
	err     error
}

func (m *mockGroupFetcher) GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]string(nil), m.members...), nil
}

func (m *mockGroupFetcher) GetGroupAdmins(ctx context.Context, groupUUID string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]string(nil), m.admins...), nil
}

func TestRealtimeHandle_UserTarget_SendsAllDevices(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"user-1": {
				{UserUUID: "user-1", DeviceID: "dev-1", ConnectGRPCAddr: "connect-a"},
				{UserUUID: "user-1", DeviceID: "dev-2", ConnectGRPCAddr: "connect-b"},
			},
		},
	}
	h := &Handler{routes: routes, sender: sender}
	payload, err := realtimepush.EncodePayload(&realtimepb.FriendApplyCreatedPayload{ApplyId: 1, ApplicantUuid: "applicant-1", TargetUuid: "user-1"})
	require.NoError(t, err)
	event := realtimepush.NewEvent(realtimepush.TypeFriendApplyCreated, realtimepush.NewUserTarget("user-1"), payload)
	event.TraceID = "trace-1"
	event.ServerTs = 123

	err = h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.NoError(t, err)
	require.Len(t, sender.calls, 2)
	assert.ElementsMatch(t, []string{"dev-1", "dev-2"}, []string{sender.calls[0].deviceID, sender.calls[1].deviceID})
	for _, call := range sender.calls {
		require.NotNil(t, call.envelope)
		assert.Equal(t, realtimepush.TypeFriendApplyCreated, call.envelope.GetType())
		assert.Equal(t, "trace-1", call.envelope.GetTraceId())
		assert.False(t, call.envelope.GetAckRequired())
		var gotPayload realtimepb.FriendApplyCreatedPayload
		require.NoError(t, proto.Unmarshal(call.envelope.GetData(), &gotPayload))
		assert.Equal(t, int64(1), gotPayload.GetApplyId())
		assert.Equal(t, "applicant-1", gotPayload.GetApplicantUuid())
		assert.Equal(t, "user-1", gotPayload.GetTargetUuid())
	}
}

func TestRealtimeHandle_DeviceTarget_SendsOnlyDevice(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"user-1": {
				{UserUUID: "user-1", DeviceID: "dev-1", ConnectGRPCAddr: "connect-a"},
				{UserUUID: "user-1", DeviceID: "dev-2", ConnectGRPCAddr: "connect-b"},
			},
		},
	}
	h := &Handler{routes: routes, sender: sender}
	event := realtimepush.NewEvent(realtimepush.TypeFriendApplyHandled, realtimepush.NewDeviceTarget("user-1", "dev-2"), nil)
	event.ServerTs = 123

	err := h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.NoError(t, err)
	require.Len(t, sender.calls, 1)
	assert.Equal(t, "dev-2", sender.calls[0].deviceID)
}

func TestRealtimeHandle_UserListTarget_BatchRoutesAndDeduplicates(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		usersRoutes: map[string][]route.DeviceRoute{
			"user-a": {
				{UserUUID: "user-a", DeviceID: "a-1", ConnectGRPCAddr: "connect-a"},
				{UserUUID: "user-a", DeviceID: "a-1", ConnectGRPCAddr: "connect-a"},
			},
			"user-b": {{UserUUID: "user-b", DeviceID: "b-1", ConnectGRPCAddr: "connect-b"}},
		},
	}
	h := &Handler{routes: routes, sender: sender}
	event := realtimepush.NewEvent(realtimepush.TypeFriendRelationChanged, realtimepush.NewUserListTarget([]string{"user-a", "user-b", "user-a"}), nil)
	event.ServerTs = 123

	err := h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.NoError(t, err)
	require.Len(t, sender.calls, 2)
	assert.ElementsMatch(t, []string{"a-1", "b-1"}, []string{sender.calls[0].deviceID, sender.calls[1].deviceID})
}

func TestRealtimeHandle_GroupMembersTarget_ExpandsMembers(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		usersRoutes: map[string][]route.DeviceRoute{
			"member-a": {{UserUUID: "member-a", DeviceID: "a-1", ConnectGRPCAddr: "connect-a"}},
			"member-b": {{UserUUID: "member-b", DeviceID: "b-1", ConnectGRPCAddr: "connect-b"}},
		},
	}
	groups := &mockGroupFetcher{members: []string{"member-a", "member-b", "member-a"}}
	h := &Handler{routes: routes, sender: sender, groups: groups}
	event := realtimepush.NewEvent(realtimepush.TypeGroupStateChanged, realtimepush.NewGroupMembersTarget("group-1"), nil)
	event.ServerTs = 123

	err := h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.NoError(t, err)
	require.Len(t, sender.calls, 2)
	assert.ElementsMatch(t, []string{"a-1", "b-1"}, []string{sender.calls[0].deviceID, sender.calls[1].deviceID})
}

func TestRealtimeHandle_GroupAdminsTarget_ExpandsAdmins(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		usersRoutes: map[string][]route.DeviceRoute{
			"admin-a": {{UserUUID: "admin-a", DeviceID: "a-1", ConnectGRPCAddr: "connect-a"}},
			"owner":   {{UserUUID: "owner", DeviceID: "owner-1", ConnectGRPCAddr: "connect-b"}},
		},
	}
	groups := &mockGroupFetcher{admins: []string{"admin-a", "owner", "admin-a"}}
	h := &Handler{routes: routes, sender: sender, groups: groups}
	event := realtimepush.NewEvent(realtimepush.TypeGroupJoinRequestCreated, realtimepush.NewGroupAdminsTarget("group-1"), nil)
	event.ServerTs = 123

	err := h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.NoError(t, err)
	require.Len(t, sender.calls, 2)
	assert.ElementsMatch(t, []string{"a-1", "owner-1"}, []string{sender.calls[0].deviceID, sender.calls[1].deviceID})
}

func TestRealtimeHandle_InvalidProto_SkipsNonRetriable(t *testing.T) {
	h := &Handler{}
	err := h.Handle(context.Background(), []byte("garbage"))
	assert.NoError(t, err)
}

func TestRealtimeHandle_MissingSender_ReturnsRetriable(t *testing.T) {
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"user-1": {{UserUUID: "user-1", DeviceID: "dev-1", ConnectGRPCAddr: "connect-a"}},
		},
	}
	h := &Handler{routes: routes}
	event := realtimepush.NewEvent(realtimepush.TypeFriendApplyCreated, realtimepush.NewUserTarget("user-1"), nil)
	event.ServerTs = 123

	err := h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.Error(t, err)
	assert.True(t, errors.Is(err, pusherr.ErrRetriable))
}

func TestRealtimeHandle_ReturnsRetriableWhenAllPushesFail(t *testing.T) {
	sender := &mockSender{err: errors.New("push failed")}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"user-1": {{UserUUID: "user-1", DeviceID: "dev-1", ConnectGRPCAddr: "connect-a"}},
		},
	}
	h := &Handler{routes: routes, sender: sender}
	event := realtimepush.NewEvent(realtimepush.TypeFriendApplyCreated, realtimepush.NewUserTarget("user-1"), nil)
	event.ServerTs = 123

	err := h.Handle(context.Background(), marshalRealtimeEvent(t, event))
	require.Error(t, err)
	assert.True(t, errors.Is(err, pusherr.ErrRetriable))
}
