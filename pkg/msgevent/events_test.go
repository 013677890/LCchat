package msgevent

import (
	"strings"
	"testing"

	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestEncodeDecodeMsgPush(t *testing.T) {
	encoded, err := EncodeMsgPush(&msgpb.MsgPushEvent{
		EventId:      "evt-1",
		ReceiverUuid: "receiver",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
		Data:         []byte("payload"),
		FromUuid:     "sender",
		Seq:          12,
	})
	require.NoError(t, err)

	decoded, err := DecodeMsgPush([]byte(encoded))
	require.NoError(t, err)
	assert.Equal(t, "evt-1", decoded.GetEventId())
	assert.Equal(t, "receiver", decoded.GetReceiverUuid())
	assert.Equal(t, []byte("payload"), decoded.GetData())
	assert.Equal(t, int64(12), decoded.GetSeq())
}

func TestDecodeMsgPushAcceptsCDCStringWrappedPayload(t *testing.T) {
	encoded, err := EncodeMsgPush(&msgpb.MsgPushEvent{
		EventId:      "evt-1",
		ReceiverUuid: "receiver",
		Type:         "MSG_PUSH",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	})
	require.NoError(t, err)

	wrapped := `"` + strings.ReplaceAll(encoded, `"`, `\"`) + `"`
	decoded, err := DecodeMsgPush([]byte(wrapped))
	require.NoError(t, err)
	assert.Equal(t, "evt-1", decoded.GetEventId())
	assert.Equal(t, "receiver", decoded.GetReceiverUuid())
}

func TestDecodeMsgPushAcceptsEnvelopePayload(t *testing.T) {
	encoded, err := EncodeMsgPush(&msgpb.MsgPushEvent{
		EventId:      "evt-1",
		ReceiverUuid: "receiver",
		Type:         "MSG_RECALL",
		ConvType:     msgpb.ConvType_CONV_TYPE_P2P,
	})
	require.NoError(t, err)

	decoded, err := DecodeMsgPush([]byte(`{"payload":` + encoded + `}`))
	require.NoError(t, err)
	assert.Equal(t, "MSG_RECALL", decoded.GetType())
}

func TestDecodeMsgPushRejectsUnknownFields(t *testing.T) {
	_, err := DecodeMsgPush([]byte(`{"event_id":"evt-1","type":"MSG_PUSH","unexpected":"x"}`))
	require.Error(t, err)
}

func TestDecodeMsgPushRejectsMissingEventID(t *testing.T) {
	_, err := DecodeMsgPush([]byte(`{"type":"MSG_PUSH"}`))
	require.Error(t, err)
}

func TestDecodeMsgPushRejectsLegacyProtoBytes(t *testing.T) {
	legacy, err := proto.Marshal(&msgpb.MsgPushEvent{
		EventId:      "evt-1",
		ReceiverUuid: "receiver",
		Type:         "MSG_PUSH",
	})
	require.NoError(t, err)

	_, err = DecodeMsgPush(legacy)
	require.Error(t, err)
}
