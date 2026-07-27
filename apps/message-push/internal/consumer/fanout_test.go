package consumer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fanoutMockSender struct {
	mu                 sync.Mutex
	deviceCalls        []pushCall
	userCalls          []userPushCall
	deviceErrors       map[string]error
	userErrors         map[string]error
	userDeliveredCount map[string]int32
	started            chan string
	release            <-chan struct{}
	activeCalls        int
	maxActiveCalls     int
}

// PushToDevice 记录设备调用，并按测试配置模拟阻塞或失败。
func (m *fanoutMockSender) PushToDevice(
	ctx context.Context,
	connectAddr string,
	userUUID string,
	deviceID string,
	envelope *connectpb.MessageEnvelope,
) error {
	m.beginDeviceCall(pushCall{
		connectAddr: connectAddr,
		userUUID:    userUUID,
		deviceID:    deviceID,
		envelope:    envelope,
	})
	defer m.endCall()
	if err := m.waitForRelease(ctx, connectAddr); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deviceErrors[connectAddr]
}

// PushToUser 记录用户调用，并返回测试配置的设备投递数量。
func (m *fanoutMockSender) PushToUser(
	ctx context.Context,
	connectAddr string,
	userUUID string,
	envelope *connectpb.MessageEnvelope,
) (int32, error) {
	m.beginUserCall(userPushCall{
		connectAddr: connectAddr,
		userUUID:    userUUID,
		envelope:    envelope,
	})
	defer m.endCall()
	if err := m.waitForRelease(ctx, connectAddr); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.userErrors[connectAddr]; err != nil {
		return 0, err
	}
	count, exists := m.userDeliveredCount[userTargetKey(connectAddr, userUUID)]
	if !exists {
		return 1, nil
	}
	return count, nil
}

// beginDeviceCall 原子记录设备调用和当前并发数。
func (m *fanoutMockSender) beginDeviceCall(call pushCall) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deviceCalls = append(m.deviceCalls, call)
	m.beginCallLocked()
}

// beginUserCall 原子记录用户调用和当前并发数。
func (m *fanoutMockSender) beginUserCall(call userPushCall) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userCalls = append(m.userCalls, call)
	m.beginCallLocked()
}

// beginCallLocked 更新并发峰值；调用方必须持有互斥锁。
func (m *fanoutMockSender) beginCallLocked() {
	m.activeCalls++
	if m.activeCalls > m.maxActiveCalls {
		m.maxActiveCalls = m.activeCalls
	}
}

// endCall 标记一次模拟 RPC 结束。
func (m *fanoutMockSender) endCall() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeCalls--
}

// waitForRelease 通知测试调用已开始，并等待放行或上下文取消。
func (m *fanoutMockSender) waitForRelease(ctx context.Context, connectAddr string) error {
	if m.started != nil {
		m.started <- connectAddr
	}
	if m.release == nil {
		return nil
	}
	select {
	case <-m.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// snapshots 返回模拟 sender 的线程安全调用快照和并发峰值。
func (m *fanoutMockSender) snapshots() ([]pushCall, []userPushCall, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	deviceCalls := append([]pushCall(nil), m.deviceCalls...)
	userCalls := append([]userPushCall(nil), m.userCalls...)
	return deviceCalls, userCalls, m.maxActiveCalls
}

// userTargetKey 生成按节点和用户索引模拟返回值的键。
func userTargetKey(connectAddr, userUUID string) string {
	return connectAddr + "\x00" + userUUID
}

// deviceTargets 创建分布在不同 connect 节点上的设备目标。
func deviceTargets(count int) []deliveryTarget {
	targets := make([]deliveryTarget, 0, count)
	for index := 0; index < count; index++ {
		targets = append(targets, deliveryTarget{Route: route.DeviceRoute{
			UserUUID:        "user-1",
			DeviceID:        fmt.Sprintf("device-%d", index),
			ConnectGRPCAddr: fmt.Sprintf("connect-%d", index),
		}})
	}
	return targets
}

// waitForStartedCalls 等待指定数量的模拟 RPC 启动。
func waitForStartedCalls(t *testing.T, started <-chan string, count int) []string {
	t.Helper()
	addresses := make([]string, 0, count)
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for len(addresses) < count {
		select {
		case address := <-started:
			addresses = append(addresses, address)
		case <-timeout.C:
			t.Fatalf("等待 %d 个并发调用超时，实际启动 %d 个", count, len(addresses))
		}
	}
	return addresses
}

// TestPushEventTargets_MultipleNodesRunConcurrently 验证不同节点会并行成功推送。
func TestPushEventTargets_MultipleNodesRunConcurrently(t *testing.T) {
	release := make(chan struct{})
	sender := &fanoutMockSender{
		started: make(chan string, 2),
		release: release,
	}
	handler := &EventHandler{sender: sender, maxFanoutConcurrency: 2}
	resultChannel := make(chan fanoutSummary, 1)
	go func() {
		resultChannel <- handler.pushEventTargets(
			context.Background(),
			"MSG_MARK_READ",
			deviceTargets(2),
			&connectpb.MessageEnvelope{Type: "MSG_MARK_READ"},
		)
	}()

	addresses := waitForStartedCalls(t, sender.started, 2)
	assert.ElementsMatch(t, []string{"connect-0", "connect-1"}, addresses)
	close(release)
	summary := <-resultChannel
	assert.Equal(t, 2, summary.SuccessCount)
	assert.Zero(t, summary.FailedCount)
	_, _, maxActiveCalls := sender.snapshots()
	assert.Equal(t, 2, maxActiveCalls)
}

// TestHandle_PartialNodeFailureStillSucceeds 验证部分节点失败时整体仍视为成功。
func TestHandle_PartialNodeFailureStillSucceeds(t *testing.T) {
	sender := &fanoutMockSender{
		deviceErrors: map[string]error{"connect-b": errors.New("connect-b unavailable")},
	}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"reader": {
			{UserUUID: "reader", DeviceID: "other-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "reader", DeviceID: "other-b", ConnectGRPCAddr: "connect-b"},
		},
	}}
	handler := &EventHandler{routes: routes, sender: sender, maxFanoutConcurrency: 2}
	err := handler.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "reader",
		DeviceId:     "current-device",
		Type:         "MSG_MARK_READ",
	}))

	require.NoError(t, err)
	deviceCalls, _, _ := sender.snapshots()
	assert.Len(t, deviceCalls, 2)
}

// TestHandle_AllNodesFailReturnsRetriable 验证所有节点失败时仍触发重试。
func TestHandle_AllNodesFailReturnsRetriable(t *testing.T) {
	sender := &fanoutMockSender{deviceErrors: map[string]error{
		"connect-a": errors.New("connect-a unavailable"),
		"connect-b": errors.New("connect-b unavailable"),
	}}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"reader": {
			{UserUUID: "reader", DeviceID: "other-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "reader", DeviceID: "other-b", ConnectGRPCAddr: "connect-b"},
		},
	}}
	handler := &EventHandler{routes: routes, sender: sender, maxFanoutConcurrency: 2}
	err := handler.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "reader",
		DeviceId:     "current-device",
		Type:         "MSG_MARK_READ",
	}))

	require.Error(t, err)
	assert.True(t, errors.Is(err, errRetriable))
}

// TestHandle_ContextCancellationStopsFanout 验证取消后阻塞调用和后续扇出会快速停止。
func TestHandle_ContextCancellationStopsFanout(t *testing.T) {
	release := make(chan struct{})
	sender := &fanoutMockSender{
		started: make(chan string, 4),
		release: release,
	}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"reader": {
			{UserUUID: "reader", DeviceID: "other-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "reader", DeviceID: "other-b", ConnectGRPCAddr: "connect-b"},
			{UserUUID: "reader", DeviceID: "other-c", ConnectGRPCAddr: "connect-c"},
			{UserUUID: "reader", DeviceID: "other-d", ConnectGRPCAddr: "connect-d"},
		},
	}}
	handler := &EventHandler{routes: routes, sender: sender, maxFanoutConcurrency: 2}
	ctx, cancel := context.WithCancel(context.Background())
	payload := marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "reader",
		DeviceId:     "current-device",
		Type:         "MSG_MARK_READ",
	})
	resultChannel := make(chan error, 1)
	go func() {
		resultChannel <- handler.Handle(ctx, payload)
	}()

	waitForStartedCalls(t, sender.started, 2)
	cancel()
	select {
	case err := <-resultChannel:
		require.Error(t, err)
		assert.True(t, errors.Is(err, errRetriable))
	case <-time.After(500 * time.Millisecond):
		t.Fatal("上下文取消后扇出未快速停止")
	}
}

// TestPushEventTargets_ConcurrencyLimit 验证同时运行的节点数不超过配置上限。
func TestPushEventTargets_ConcurrencyLimit(t *testing.T) {
	release := make(chan struct{})
	sender := &fanoutMockSender{
		started: make(chan string, 5),
		release: release,
	}
	handler := &EventHandler{sender: sender, maxFanoutConcurrency: 2}
	resultChannel := make(chan fanoutSummary, 1)
	go func() {
		resultChannel <- handler.pushEventTargets(
			context.Background(),
			"MSG_MARK_READ",
			deviceTargets(5),
			&connectpb.MessageEnvelope{Type: "MSG_MARK_READ"},
		)
	}()

	waitForStartedCalls(t, sender.started, 2)
	select {
	case address := <-sender.started:
		t.Fatalf("并发上限为 2 时提前启动了第三个节点：%s", address)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	summary := <-resultChannel
	assert.Equal(t, 5, summary.SuccessCount)
	_, _, maxActiveCalls := sender.snapshots()
	assert.Equal(t, 2, maxActiveCalls)
}

// TestFanoutConcurrency_RejectsMissingLimit 验证包内绕过构造器时也不会静默采用默认值。
func TestFanoutConcurrency_RejectsMissingLimit(t *testing.T) {
	handler := &EventHandler{}
	assert.PanicsWithValue(
		t,
		"message-push: maxFanoutConcurrency 必须大于零",
		func() { handler.fanoutConcurrency(1) },
	)
}

// TestHandle_PushToUserBatchesSameNodeDevices 验证同节点同用户只调用一次 PushToUser。
func TestHandle_PushToUserBatchesSameNodeDevices(t *testing.T) {
	sender := &fanoutMockSender{userDeliveredCount: map[string]int32{
		userTargetKey("connect-a", "receiver"): 2,
	}}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"receiver": {
			{UserUUID: "receiver", DeviceID: "device-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "receiver", DeviceID: "device-b", ConnectGRPCAddr: "connect-a"},
		},
	}}
	handler := &EventHandler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := handler.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
		Seq:          12,
	}))

	require.NoError(t, err)
	deviceCalls, userCalls, _ := sender.snapshots()
	assert.Empty(t, deviceCalls)
	require.Len(t, userCalls, 1)
	assert.Equal(t, "receiver", userCalls[0].userUUID)
	assert.Equal(t, int64(12), userCalls[0].envelope.GetSeq())
}

// TestHandle_PushToUserPartialDeliveryStillSucceeds 验证批量调用部分投递时整体仍成功。
func TestHandle_PushToUserPartialDeliveryStillSucceeds(t *testing.T) {
	sender := &fanoutMockSender{userDeliveredCount: map[string]int32{
		userTargetKey("connect-a", "receiver"): 1,
	}}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"receiver": {
			{UserUUID: "receiver", DeviceID: "device-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "receiver", DeviceID: "device-b", ConnectGRPCAddr: "connect-a"},
		},
	}}
	handler := &EventHandler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := handler.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
	}))

	require.NoError(t, err)
	deviceCalls, userCalls, _ := sender.snapshots()
	assert.Empty(t, deviceCalls)
	assert.Len(t, userCalls, 1)
}

// TestHandle_PushToUserZeroDeliveryReturnsRetriable 验证批量调用零投递时仍触发重试。
func TestHandle_PushToUserZeroDeliveryReturnsRetriable(t *testing.T) {
	sender := &fanoutMockSender{userDeliveredCount: map[string]int32{
		userTargetKey("connect-a", "receiver"): 0,
	}}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"receiver": {
			{UserUUID: "receiver", DeviceID: "device-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "receiver", DeviceID: "device-b", ConnectGRPCAddr: "connect-a"},
		},
	}}
	handler := &EventHandler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := handler.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		Type:         "MSG_READ_RECEIPT",
	}))

	require.Error(t, err)
	assert.True(t, errors.Is(err, errRetriable))
	deviceCalls, userCalls, _ := sender.snapshots()
	assert.Empty(t, deviceCalls)
	assert.Len(t, userCalls, 1)
}

// TestHandle_PushToUserPreservesExcludedDevice 验证完整用户目标批量推送且发送方仍排除当前设备。
func TestHandle_PushToUserPreservesExcludedDevice(t *testing.T) {
	sender := &fanoutMockSender{userDeliveredCount: map[string]int32{
		userTargetKey("connect-a", "receiver"): 2,
	}}
	routes := &mockRouteRepo{userRoutes: map[string][]route.DeviceRoute{
		"receiver": {
			{UserUUID: "receiver", DeviceID: "receiver-a", ConnectGRPCAddr: "connect-a"},
			{UserUUID: "receiver", DeviceID: "receiver-b", ConnectGRPCAddr: "connect-a"},
		},
		"sender": {
			{UserUUID: "sender", DeviceID: "current-device", ConnectGRPCAddr: "connect-b"},
			{UserUUID: "sender", DeviceID: "other-device", ConnectGRPCAddr: "connect-b"},
		},
	}}
	handler := &EventHandler{
		routes:               routes,
		sender:               sender,
		maxFanoutConcurrency: testMaxFanoutConcurrency,
	}
	err := handler.Handle(context.Background(), marshalEvent(t, &msgpb.MsgPushEvent{
		ReceiverUuid: "receiver",
		FromUuid:     "sender",
		DeviceId:     "current-device",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	}))

	require.NoError(t, err)
	deviceCalls, userCalls, _ := sender.snapshots()
	require.Len(t, userCalls, 1)
	require.Len(t, deviceCalls, 1)
	assert.Equal(t, "other-device", deviceCalls[0].deviceID)
}
