package msgevent

import (
	"errors"
	"fmt"

	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"google.golang.org/protobuf/encoding/protojson"
)

const EventTypeMsgPush = "msg.push"

var (
	marshalOptions = protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
	}
	unmarshalOptions = protojson.UnmarshalOptions{
		DiscardUnknown: false,
		AllowPartial:   false,
	}
)

// EncodeMsgPush encodes a msg.push outbox payload as strict protojson.
func EncodeMsgPush(event *msgpb.MsgPushEvent) (string, error) {
	if err := validateMsgPushEvent(event); err != nil {
		return "", err
	}
	data, err := marshalOptions.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("encode msg.push event: %w", err)
	}
	return string(data), nil
}

// DecodeMsgPush decodes the Kafka value produced by the CDC outbox EventRouter.
// The contract is intentionally strict: only protojson MsgPushEvent is accepted.
func DecodeMsgPush(message []byte) (*msgpb.MsgPushEvent, error) {
	var event msgpb.MsgPushEvent
	if err := unmarshalOptions.Unmarshal(message, &event); err != nil {
		return nil, fmt.Errorf("decode msg.push event: %w", err)
	}
	if err := validateMsgPushEvent(&event); err != nil {
		return nil, err
	}
	return &event, nil
}

func validateMsgPushEvent(event *msgpb.MsgPushEvent) error {
	if event == nil {
		return errors.New("msg.push event is nil")
	}
	if event.GetEventId() == "" {
		return errors.New("msg.push event_id is required")
	}
	return nil
}
