package msgpush

import (
	"context"
	"errors"
	"sync"
	"testing"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/event"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gozap "go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func init() { logger.ReplaceGlobal(gozap.NewNop()) }

// testMaxFanoutConcurrency 让直接构造的测试处理器满足生产代码的必填并发契约。
const testMaxFanoutConcurrency = 32

func marshalEvent(t *testing.T, e *msgpb.MsgPushEvent) []byte {
	t.Helper()
	if e.EventId == "" {
		e.EventId = "evt-test"
	}
	data, err := event.EncodeMsgPush(e)
	require.NoError(t, err)
	return []byte(data)
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

type userPushCall struct {
	connectAddr string
	userUUID    string
	envelope    *connectpb.MessageEnvelope
}

type mockSender struct {
	mu               sync.Mutex
	calls            []pushCall
	userCalls        []userPushCall
	err              error
	failDeviceErrors map[string]error
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
	if err := m.failDeviceErrors[deviceID]; err != nil {
		return err
	}
	return m.err
}

// PushToUser 让通用 mock 实现消息下行 Handler 的完整发送契约。
// 基础行为测试的完整用户目标都只有一台设备，因此成功时返回一次投递。
func (m *mockSender) PushToUser(
	ctx context.Context,
	connectAddr string,
	userUUID string,
	envelope *connectpb.MessageEnvelope,
) (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userCalls = append(m.userCalls, userPushCall{
		connectAddr: connectAddr,
		userUUID:    userUUID,
		envelope:    envelope,
	})
	if m.err != nil {
		return 0, m.err
	}
	return 1, nil
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

func TestHandle_InvalidProto_SkipsNonRetriable(t *testing.T) {
	h := &Handler{}
	err := h.Handle(context.Background(), []byte("garbage"))
	assert.NoError(t, err)
}

func TestHandle_ProtoBytes_SkipsNonRetriable(t *testing.T) {
	h := &Handler{}
	data, err := proto.Marshal(&msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		EventId:      "evt-old-proto",
	})
	require.NoError(t, err)
	err = h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_UnsupportedEventType_SkipsNonRetriable(t *testing.T) {
	h := &Handler{}
	data := marshalEvent(t, &msgpb.MsgPushEvent{ReceiverUuid: "user1", Type: "MSG_UNKNOWN"})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_EmptyReceiver_SkipsNonRetriable(t *testing.T) {
	h := &Handler{}
	data := marshalEvent(t, &msgpb.MsgPushEvent{Type: "MSG_PUSH"})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_UnsupportedConvType_SkipsNonRetriable(t *testing.T) {
	h := &Handler{}
	data := marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "user1",
		Type:         "MSG_PUSH",
		ConvType:     999,
	})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_MissingRouteRepository_ReturnsRetriable(t *testing.T) {
	h := &Handler{sender: &mockSender{}}
	err := h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, pusherr.ErrRetriable))
}

func TestHandle_MissingSender_ReturnsRetriable(t *testing.T) {
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"receiver": {{UserUUID: "receiver", DeviceID: "receiver-dev", ConnectGRPCAddr: "connect-a"}},
		},
	}
	h := &Handler{routes: routes}
	err := h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, pusherr.ErrRetriable))
}

func TestHandle_P2PPush_SendsReceiverAndSenderOtherDevices(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"receiver": {{UserUUID: "receiver", DeviceID: "receiver-dev", ConnectGRPCAddr: "connect-a"}},
			"sender": {
				{UserUUID: "sender", DeviceID: "current-dev", ConnectGRPCAddr: "connect-b"},
				{UserUUID: "sender", DeviceID: "other-dev", ConnectGRPCAddr: "connect-c"},
			},
		},
	}
	itemData, err := proto.Marshal(&msgpb.MsgItem{ConvId: "conv-1", Seq: 9})
	require.NoError(t, err)

	h := &Handler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err = h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		DeviceId:     "current-dev",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Data:         itemData,
		FromUuid:     "sender",
	}))
	require.NoError(t, err)
	require.Len(t, sender.userCalls, 1)
	assert.Equal(t, "connect-a", sender.userCalls[0].connectAddr)
	assert.Equal(t, "receiver", sender.userCalls[0].userUUID)
	require.Len(t, sender.calls, 1)
	assert.Equal(t, "other-dev", sender.calls[0].deviceID)
	for _, envelope := range []*connectpb.MessageEnvelope{
		sender.userCalls[0].envelope,
		sender.calls[0].envelope,
	} {
		require.NotNil(t, envelope)
		assert.True(t, envelope.GetAckRequired())
		assert.Equal(t, int64(9), envelope.GetSeq())
	}
}

func TestHandle_MsgPush_FillsSeqFromData(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"receiver": {{UserUUID: "receiver", DeviceID: "receiver-dev", ConnectGRPCAddr: "connect-a"}},
		},
	}
	itemData, err := proto.Marshal(&msgpb.MsgItem{ConvId: "conv-1", Seq: 42})
	require.NoError(t, err)

	h := &Handler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err = h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Data:         itemData,
	}))
	require.NoError(t, err)
	assert.Empty(t, sender.calls)
	require.Len(t, sender.userCalls, 1)
	require.NotNil(t, sender.userCalls[0].envelope)
	assert.Equal(t, int64(42), sender.userCalls[0].envelope.GetSeq())
	assert.True(t, sender.userCalls[0].envelope.GetAckRequired())
}

func TestHandle_MarkRead_OnlySyncsOtherDevices(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"reader": {
				{UserUUID: "reader", DeviceID: "current-dev", ConnectGRPCAddr: "connect-a"},
				{UserUUID: "reader", DeviceID: "other-dev", ConnectGRPCAddr: "connect-b"},
			},
		},
	}
	h := &Handler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "reader",
		DeviceId:     "current-dev",
		Type:         "MSG_MARK_READ",
		ConvType:     msgpb.ConvType_CONV_TYPE_GROUP,
	}))
	require.NoError(t, err)
	require.Len(t, sender.calls, 1)
	assert.Empty(t, sender.userCalls)
	assert.Equal(t, "other-dev", sender.calls[0].deviceID)
	assert.False(t, sender.calls[0].envelope.GetAckRequired())
}

func TestHandle_ReadReceipt_SendsReceiver(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"sender": {{UserUUID: "sender", DeviceID: "sender-dev", ConnectGRPCAddr: "connect-a"}},
			"reader": {{UserUUID: "reader", DeviceID: "reader-other", ConnectGRPCAddr: "connect-b"}},
		},
	}
	h := &Handler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "sender",
		FromUuid:     "reader",
		DeviceId:     "reader-current",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Seq:          12,
	}))
	require.NoError(t, err)
	assert.Empty(t, sender.calls)
	require.Len(t, sender.userCalls, 1)
	call := sender.userCalls[0]
	assert.Equal(t, "connect-a", call.connectAddr)
	assert.Equal(t, "sender", call.userUUID)
	require.NotNil(t, call.envelope)
	assert.Equal(t, "MSG_READ_RECEIPT", call.envelope.GetType())
	assert.False(t, call.envelope.GetAckRequired())
}

func TestHandle_GroupPush_ExpandsMembersAndDeduplicates(t *testing.T) {
	sender := &mockSender{}
	routes := &mockRouteRepo{
		usersRoutes: map[string][]route.DeviceRoute{
			"member-a": {{UserUUID: "member-a", DeviceID: "a-1", ConnectGRPCAddr: "connect-a"}},
			"member-b": {{UserUUID: "member-b", DeviceID: "b-1", ConnectGRPCAddr: "connect-b"}},
		},
		userRoutes: map[string][]route.DeviceRoute{
			"sender": {{UserUUID: "sender", DeviceID: "sender-other", ConnectGRPCAddr: "connect-c"}},
		},
	}
	groups := &mockGroupFetcher{members: []string{"member-a", "member-b", "sender", "member-a"}}
	itemData, err := proto.Marshal(&msgpb.MsgItem{ConvId: "group-1", Seq: 5})
	require.NoError(t, err)

	h := &Handler{
		routes:               routes,
		sender:               sender,
		groups:               groups,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err = h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "group-1",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_GROUP,
		Data:         itemData,
		FromUuid:     "sender",
		DeviceId:     "sender-current",
		Seq:          5,
	}))
	require.NoError(t, err)
	require.Len(t, sender.userCalls, 2)
	assert.ElementsMatch(
		t,
		[]string{"member-a", "member-b"},
		[]string{sender.userCalls[0].userUUID, sender.userCalls[1].userUUID},
	)
	require.Len(t, sender.calls, 1)
	assert.Equal(t, "sender-other", sender.calls[0].deviceID)
}

func TestHandle_ReturnsRetriableWhenAllPushesFail(t *testing.T) {
	sender := &mockSender{err: errors.New("push failed")}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"receiver": {{UserUUID: "receiver", DeviceID: "receiver-dev", ConnectGRPCAddr: "connect-a"}},
		},
	}
	h := &Handler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := h.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, pusherr.ErrRetriable))
}
