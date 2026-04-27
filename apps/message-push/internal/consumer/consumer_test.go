package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gozap "go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func init() { logger.ReplaceGlobal(gozap.NewNop()) }

func marshalEvent(e *msgpb.MsgPushEvent) []byte {
	data, _ := proto.Marshal(e)
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
	calls            []pushCall
	err              error
	failDeviceErrors map[string]error
}

func (m *mockSender) PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error {
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
	err     error
}

func (m *mockGroupFetcher) GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]string(nil), m.members...), nil
}

func TestHandle_InvalidProto_SkipsNonRetriable(t *testing.T) {
	h := &EventHandler{}
	err := h.Handle(context.Background(), []byte("garbage"))
	assert.NoError(t, err)
}

func TestHandle_UnsupportedEventType_SkipsNonRetriable(t *testing.T) {
	h := &EventHandler{}
	data := marshalEvent(&msgpb.MsgPushEvent{ReceiverUuid: "user1", Type: "MSG_UNKNOWN"})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_EmptyReceiver_SkipsNonRetriable(t *testing.T) {
	h := &EventHandler{}
	data := marshalEvent(&msgpb.MsgPushEvent{Type: "MSG_PUSH"})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_UnsupportedConvType_SkipsNonRetriable(t *testing.T) {
	h := &EventHandler{}
	data := marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "user1",
		Type:         "MSG_PUSH",
		ConvType:     999,
	})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
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

	h := &EventHandler{routes: routes, sender: sender}
	err = h.Handle(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		DeviceId:     "current-dev",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Data:         itemData,
		FromUuid:     "sender",
	}))
	require.NoError(t, err)
	require.Len(t, sender.calls, 2)
	assert.ElementsMatch(t, []string{"receiver-dev", "other-dev"}, []string{sender.calls[0].deviceID, sender.calls[1].deviceID})
	for _, call := range sender.calls {
		require.NotNil(t, call.envelope)
		assert.True(t, call.envelope.GetAckRequired())
		assert.Equal(t, int64(9), call.envelope.GetSeq())
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

	h := &EventHandler{routes: routes, sender: sender}
	err = h.Handle(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Data:         itemData,
	}))
	require.NoError(t, err)
	require.Len(t, sender.calls, 1)
	require.NotNil(t, sender.calls[0].envelope)
	assert.Equal(t, int64(42), sender.calls[0].envelope.GetSeq())
	assert.True(t, sender.calls[0].envelope.GetAckRequired())
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
	h := &EventHandler{routes: routes, sender: sender}
	err := h.Handle(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "reader",
		DeviceId:     "current-dev",
		Type:         "MSG_MARK_READ",
		ConvType:     msgpb.ConvType_CONV_TYPE_GROUP,
	}))
	require.NoError(t, err)
	require.Len(t, sender.calls, 1)
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
	h := &EventHandler{routes: routes, sender: sender}
	err := h.Handle(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "sender",
		FromUuid:     "reader",
		DeviceId:     "reader-current",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Seq:          12,
	}))
	require.NoError(t, err)
	require.Len(t, sender.calls, 1)
	call := sender.calls[0]
	assert.Equal(t, "connect-a", call.connectAddr)
	assert.Equal(t, "sender", call.userUUID)
	assert.Equal(t, "sender-dev", call.deviceID)
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

	h := &EventHandler{routes: routes, sender: sender, groups: groups}
	err = h.Handle(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "group-1",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_GROUP,
		Data:         itemData,
		FromUuid:     "sender",
		Seq:          5,
	}))
	require.NoError(t, err)
	require.Len(t, sender.calls, 3)
	assert.ElementsMatch(t, []string{"a-1", "b-1", "sender-other"}, []string{sender.calls[0].deviceID, sender.calls[1].deviceID, sender.calls[2].deviceID})
}

func TestHandle_ReturnsRetriableWhenAllPushesFail(t *testing.T) {
	sender := &mockSender{err: errors.New("push failed")}
	routes := &mockRouteRepo{
		userRoutes: map[string][]route.DeviceRoute{
			"receiver": {{UserUUID: "receiver", DeviceID: "receiver-dev", ConnectGRPCAddr: "connect-a"}},
		},
	}
	h := &EventHandler{routes: routes, sender: sender}
	err := h.Handle(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errRetriable))
}

func TestRunHandleWithRetry_ReturnsAfterMaxRetriableAttempts(t *testing.T) {
	routes := &mockRouteRepo{err: errors.New("redis down")}
	h := &EventHandler{routes: routes, sender: &mockSender{}}
	c := &Consumer{handler: h}
	originalBackoffs := handleBackoffs
	handleBackoffs = []time.Duration{0, 0, 0}
	defer func() { handleBackoffs = originalBackoffs }()

	err := c.runHandleWithRetry(context.Background(), marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errRetriable))
}

func TestRunHandleWithRetry_ReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Consumer{handler: &EventHandler{}}
	err := c.runHandleWithRetry(ctx, []byte("payload"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}
