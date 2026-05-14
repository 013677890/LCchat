package grpc

import (
	"context"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/connect/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDeliveryMetadata_EmptyWhenAckNotRequired(t *testing.T) {
	metadata := deliveryMetadata(context.Background(), &pb.MessageEnvelope{Seq: 10, AckRequired: false})
	require.False(t, metadata.AckRequired)
	require.Zero(t, metadata.Seq)
	require.Empty(t, metadata.ConvID)
}

func TestDeliveryMetadata_MSGPushExtractsConvID(t *testing.T) {
	itemData, err := proto.Marshal(&msgpb.MsgItem{ConvId: "conv-1", Seq: 12})
	require.NoError(t, err)

	metadata := deliveryMetadata(context.Background(), &pb.MessageEnvelope{
		Type:        "MSG_PUSH",
		Data:        itemData,
		Seq:         12,
		AckRequired: true,
	})
	require.True(t, metadata.AckRequired)
	require.Equal(t, int64(12), metadata.Seq)
	require.Equal(t, "conv-1", metadata.ConvID)
}

func TestDeliveryMetadata_NonPushKeepsSeqOnly(t *testing.T) {
	metadata := deliveryMetadata(context.Background(), &pb.MessageEnvelope{
		Type:        "MSG_READ_RECEIPT",
		Seq:         5,
		AckRequired: true,
	})
	require.True(t, metadata.AckRequired)
	require.Equal(t, int64(5), metadata.Seq)
	require.Empty(t, metadata.ConvID)
}

func TestDeliveryMetadata_InvalidPushPayloadDoesNotSetConvID(t *testing.T) {
	metadata := deliveryMetadata(context.Background(), &pb.MessageEnvelope{
		Type:        "MSG_PUSH",
		Data:        []byte("bad"),
		Seq:         9,
		AckRequired: true,
	})
	require.True(t, metadata.AckRequired)
	require.Equal(t, int64(9), metadata.Seq)
	require.Empty(t, metadata.ConvID)
}
