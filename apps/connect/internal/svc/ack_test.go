package svc

import (
	"testing"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestParseMessageAckProto(t *testing.T) {
	s := &ConnectService{}
	payload := appendProtoStringField(nil, 1, "conv-1")
	payload = appendProtoInt64Field(payload, 2, 42)
	payload = appendProtoStringField(payload, 3, "msg-1")

	ack, err := s.ParseMessageAck(payload)
	require.NoError(t, err)
	require.Equal(t, "conv-1", ack.ConvID)
	require.Equal(t, int64(42), ack.Seq)
	require.Equal(t, "msg-1", ack.MsgID)
}

func TestParseMessageAckRejectsInvalidSeq(t *testing.T) {
	s := &ConnectService{}
	payload := appendProtoStringField(nil, 1, "conv-1")

	_, err := s.ParseMessageAck(payload)
	require.Error(t, err)
}

func TestParseProtoEnvelope(t *testing.T) {
	s := &ConnectService{}
	payload := appendProtoStringField(nil, 1, "conv-1")
	payload = appendProtoInt64Field(payload, 2, 7)
	raw, err := proto.Marshal(&connectpb.MessageEnvelope{
		Type: EnvelopeTypeMessageAck,
		Data: payload,
		Seq:  7,
	})
	require.NoError(t, err)

	envelope, err := s.ParseProtoEnvelope(raw)
	require.NoError(t, err)
	require.Equal(t, EnvelopeTypeMessageAck, envelope.GetType())
	require.Equal(t, int64(7), envelope.GetSeq())
	require.Equal(t, payload, envelope.GetData())
}
