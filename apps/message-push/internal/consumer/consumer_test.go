package consumer

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gozap "go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func init() { logger.ReplaceGlobal(gozap.NewNop()) }

// stubHandler 仅用于验证 consumer Pool 壳与 best-effort 重试适配层。
type stubHandler struct {
	calls atomic.Int32
	err   error
}

// Handle 记录调用次数并返回预设错误。
func (h *stubHandler) Handle(ctx context.Context, value []byte) error {
	h.calls.Add(1)
	return h.err
}

func TestNewConsumerBuildsManualPoolAndRejectsInvalidWorkers(t *testing.T) {
	handler := &stubHandler{}
	consumer, err := NewConsumer([]string{"127.0.0.1:9092"}, "msg.push", "message-push-test", 3, handler)
	require.NoError(t, err)
	require.NotNil(t, consumer.pool)
	assert.Equal(t, 3, consumer.pool.WorkerCount())
	require.NoError(t, consumer.Close())

	_, err = NewConsumer([]string{"127.0.0.1:9092"}, "msg.push", "message-push-test", 0, handler)
	require.Error(t, err)
}

func TestEventTypeForMetric(t *testing.T) {
	data, err := msgevent.EncodeMsgPush(&msgpb.MsgPushEvent{
		EventId:      "evt-test",
		ReceiverUuid: "user1",
		Type:         "MSG_READ_RECEIPT",
	})
	require.NoError(t, err)
	assert.Equal(t, "MSG_READ_RECEIPT", eventTypeForMetric([]byte(data)))

	realtimeData, err := realtimepush.NewEvent(
		realtimepush.TypeFriendApplyCreated,
		realtimepush.NewUserTarget("user1"),
		nil,
	).Marshal()
	require.NoError(t, err)
	assert.Equal(t, realtimepush.TypeFriendApplyCreated, eventTypeForMetric(realtimeData))

	assert.Equal(t, "unknown", eventTypeForMetric([]byte("garbage")))

	protoBytes, err := proto.Marshal(&msgpb.MsgPushEvent{Type: "MSG_PUSH", EventId: "evt-x"})
	require.NoError(t, err)
	assert.Equal(t, "unknown", eventTypeForMetric(protoBytes))
}

func TestRunHandleWithRetry_ReturnsAfterMaxRetriableAttempts(t *testing.T) {
	handler := &stubHandler{err: pusherr.ErrRetriable}
	c := &Consumer{handler: handler}
	originalBackoffs := handleBackoffs
	handleBackoffs = []time.Duration{0, 0, 0}
	defer func() { handleBackoffs = originalBackoffs }()

	err := c.runHandleWithRetry(context.Background(), []byte("payload"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, pusherr.ErrRetriable))
	assert.Equal(t, int32(handleMaxAttempts), handler.calls.Load())
}

func TestRunHandleWithRetry_ReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Consumer{handler: &stubHandler{}}
	err := c.runHandleWithRetry(ctx, []byte("payload"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}
