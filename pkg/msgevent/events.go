package msgevent

import (
	"bytes"
	"encoding/json"
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
// The inner MsgPushEvent contract stays strict, but the outer CDC/JsonConverter
// representation may wrap the JSON payload as a JSON string or envelope object.
func DecodeMsgPush(message []byte) (*msgpb.MsgPushEvent, error) {
	var lastErr error
	for _, candidate := range collectPayloadCandidates(message, 0, map[string]struct{}{}) {
		var event msgpb.MsgPushEvent
		if err := unmarshalOptions.Unmarshal(candidate, &event); err != nil {
			lastErr = err
			continue
		}
		if err := validateMsgPushEvent(&event); err != nil {
			lastErr = err
			continue
		}
		return &event, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("decode msg.push event: %w", lastErr)
	}
	return nil, errors.New("decode msg.push event: payload missing required fields")
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

func collectPayloadCandidates(raw []byte, depth int, visited map[string]struct{}) [][]byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || depth > 4 {
		return nil
	}

	key := string(trimmed)
	if _, exists := visited[key]; exists {
		return nil
	}
	visited[key] = struct{}{}

	results := [][]byte{trimmed}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		results = append(results, collectPayloadCandidates([]byte(text), depth+1, visited)...)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return results
	}

	for _, field := range []string{"payload", "after", "data"} {
		candidate, ok := object[field]
		if !ok {
			continue
		}
		results = append(results, collectPayloadCandidates(candidate, depth+1, visited)...)
	}

	return results
}
