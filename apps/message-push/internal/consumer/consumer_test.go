package consumer

import (
	"context"
	"testing"

	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func init() { logger.ReplaceGlobal(zap.NewNop()) }

func newHandler() *EventHandler {
	return &EventHandler{}
}

func marshalEvent(e *msgpb.MsgPushEvent) []byte {
	data, _ := proto.Marshal(e)
	return data
}

func TestHandle_InvalidProto_SkipsNonRetriable(t *testing.T) {
	h := newHandler()
	err := h.Handle(context.Background(), []byte("garbage"))
	assert.NoError(t, err)
}

func TestHandle_UnsupportedEventType_SkipsNonRetriable(t *testing.T) {
	h := newHandler()
	data := marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "user1",
		Type:         "MSG_UNKNOWN",
	})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_EmptyReceiver_SkipsNonRetriable(t *testing.T) {
	h := newHandler()
	data := marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "",
		Type:         "MSG_PUSH",
	})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_UnsupportedConvType_SkipsNonRetriable(t *testing.T) {
	h := newHandler()
	data := marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "user1",
		Type:         "MSG_PUSH",
		ConvType:     999,
	})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_ProtoFieldsPreserved(t *testing.T) {
	event := &msgpb.MsgPushEvent{
		ReceiverUuid: "user1",
		DeviceId:     "dev-1",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		FromUuid:     "sender1",
		TraceId:      "trace-123",
		ServerTs:     1700000000000,
	}
	data := marshalEvent(event)

	var decoded msgpb.MsgPushEvent
	err := proto.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "user1", decoded.ReceiverUuid)
	assert.Equal(t, "dev-1", decoded.DeviceId)
	assert.Equal(t, "MSG_PUSH", decoded.Type)
	assert.Equal(t, msgpb.ConvType_CONV_TYPE_P2P, decoded.ConvType)
	assert.Equal(t, "sender1", decoded.FromUuid)
	assert.Equal(t, "trace-123", decoded.TraceId)
	assert.Equal(t, int64(1700000000000), decoded.ServerTs)
}

func TestHandle_Group_NilGroupClient_SkipsNonRetriable(t *testing.T) {
	h := newHandler()
	data := marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "group1",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_GROUP,
	})
	err := h.Handle(context.Background(), data)
	assert.NoError(t, err)
}

func TestHandle_AllSupportedEventTypes(t *testing.T) {
	types := []string{"MSG_PUSH", "MSG_RECALL", "MSG_MARK_READ", "MSG_READ_RECEIPT"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			h := newHandler()
			data := marshalEvent(&msgpb.MsgPushEvent{
				ReceiverUuid: "group1",
				Type:         typ,
				ConvType:     msgpb.ConvType_CONV_TYPE_GROUP,
			})
			err := h.Handle(context.Background(), data)
			assert.NoError(t, err)
		})
	}
}

func TestRunHandleWithRetry_NilHandler_Returns(t *testing.T) {
	c := &Consumer{
		handler: newHandler(),
	}
	data := marshalEvent(&msgpb.MsgPushEvent{
		ReceiverUuid: "",
		Type:         "MSG_PUSH",
	})
	err := c.runHandleWithRetry(context.Background(), data)
	assert.NoError(t, err)
}
