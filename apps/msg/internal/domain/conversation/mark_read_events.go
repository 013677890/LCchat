package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/convid"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/event"
	"github.com/013677890/LCchat-Backend/pkg/id"
	"google.golang.org/protobuf/proto"
)

// markReadPushSpec 描述一次已读类 msg.push 事件的可变字段。
type markReadPushSpec struct {
	receiverUUID string
	deviceID     string
	pushType     string
	fromUUID     string
	convType     pb.ConvType
	noticeData   []byte
	serverTs     int64
}

// buildMarkReadOutboxEvents 构造标记已读需要随会话更新同事务写入的 outbox 事件。
func buildMarkReadOutboxEvents(ctx context.Context, ownerUuid, convId string, readSeq int64) ([]OutboxEvent, error) {
	noticeData, err := proto.Marshal(&pb.MarkReadNotice{ConvId: convId, ReadSeq: readSeq})
	if err != nil {
		return nil, fmt.Errorf("marshal MarkReadNotice failed: %w", err)
	}

	convType := resolveMarkReadConvType(convId)
	serverTs := time.Now().UnixMilli()
	specs := []markReadPushSpec{{
		receiverUUID: ownerUuid,
		deviceID:     ctxmeta.DeviceID(ctx),
		pushType:     "MSG_MARK_READ",
		fromUUID:     ownerUuid,
		convType:     convType,
		noticeData:   noticeData,
		serverTs:     serverTs,
	}}

	if convType == pb.ConvType_CONV_TYPE_P2P {
		peerUUID, err := convid.PeerUUID(convId, ownerUuid)
		if err != nil {
			return nil, fmt.Errorf("resolve mark-read P2P peer: %w", err)
		}
		specs = append(specs, markReadPushSpec{
			receiverUUID: peerUUID,
			pushType:     "MSG_READ_RECEIPT",
			fromUUID:     ownerUuid,
			convType:     pb.ConvType_CONV_TYPE_P2P,
			noticeData:   noticeData,
			serverTs:     serverTs,
		})
	}

	events := make([]OutboxEvent, 0, len(specs))
	for _, spec := range specs {
		payload, err := encodeMarkReadPushEvent(ctx, spec)
		if err != nil {
			return nil, err
		}
		events = append(events, OutboxEvent{
			EventType: event.EventTypeMsgPush,
			EntityID:  convId,
			Payload:   payload,
		})
	}

	return events, nil
}

// encodeMarkReadPushEvent 将已读类事件编码为严格 protojson MsgPushEvent。
func encodeMarkReadPushEvent(ctx context.Context, spec markReadPushSpec) (string, error) {
	return event.EncodeMsgPush(&pb.MsgPushEvent{
		EventId:      id.GenerateULID(),
		ReceiverUuid: spec.receiverUUID,
		DeviceId:     spec.deviceID,
		Type:         spec.pushType,
		ConvType:     spec.convType,
		Data:         spec.noticeData,
		TraceId:      ctxmeta.TraceID(ctx),
		FromUuid:     spec.fromUUID,
		ServerTs:     spec.serverTs,
		Seq:          0,
	})
}

// resolveMarkReadConvType 根据会话 ID 判断已读事件的会话类型。
func resolveMarkReadConvType(convId string) pb.ConvType {
	if strings.HasPrefix(convId, "p2p-") {
		return pb.ConvType_CONV_TYPE_P2P
	}
	return pb.ConvType_CONV_TYPE_GROUP
}
