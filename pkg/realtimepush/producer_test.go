package realtimepush

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
)

type fakeWriter struct {
	key  []byte
	data []byte
}

func (w *fakeWriter) SendWithKey(_ context.Context, key, data []byte) error {
	w.key = cloneBytes(key)
	w.data = cloneBytes(data)
	return nil
}

func TestEventMarshalDecodeNormalizesTargetAndPayload(t *testing.T) {
	payload, err := EncodePayload(map[string]string{"apply_id": "apply-1"})
	if err != nil {
		t.Fatalf("EncodePayload() error = %v", err)
	}

	event := NewEvent(" FRIEND_APPLY_CREATED ", NewUserListTarget([]string{" user-b ", "", "user-a", "user-b"}), payload)
	event.TraceID = " trace-1 "
	event.ServerTs = 123

	data, err := event.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Type != TypeFriendApplyCreated {
		t.Fatalf("Type = %q, want %q", decoded.Type, TypeFriendApplyCreated)
	}
	if decoded.TraceID != "trace-1" {
		t.Fatalf("TraceID = %q, want trace-1", decoded.TraceID)
	}
	if got := decoded.Target.UserUUIDs; len(got) != 2 || got[0] != "user-b" || got[1] != "user-a" {
		t.Fatalf("UserUUIDs = %#v, want [user-b user-a]", got)
	}
	if string(decoded.Data) != string(payload) {
		t.Fatalf("Data = %s, want %s", decoded.Data, payload)
	}
	if decoded.PartitionKey() != "user-b" {
		t.Fatalf("PartitionKey() = %q, want user-b", decoded.PartitionKey())
	}
}

func TestEventValidateRequiresAckSeq(t *testing.T) {
	event := NewEvent(TypeGroupMemberRemoved, NewUserTarget("user-1"), nil)
	event.AckRequired = true

	if err := event.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want ack seq error")
	}
}

func TestGroupAggregateTargetUsesGroupPartitionKey(t *testing.T) {
	payload := NewGroupStateChangedPayload(" group-1 ", []string{GroupChangedInfo, "", GroupChangedNotice, GroupChangedInfo}, 9)
	data, err := EncodePayload(payload)
	if err != nil {
		t.Fatalf("EncodePayload() error = %v", err)
	}

	event := NewEvent(TypeGroupStateChanged, NewGroupMembersTarget(" group-1 "), data)
	event.ServerTs = 123

	encoded, err := event.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Target.Kind != TargetKindGroupMembers {
		t.Fatalf("Target.Kind = %q, want %q", decoded.Target.Kind, TargetKindGroupMembers)
	}
	if decoded.Target.GroupUUID != "group-1" {
		t.Fatalf("Target.GroupUUID = %q, want group-1", decoded.Target.GroupUUID)
	}
	if decoded.PartitionKey() != "group-1" {
		t.Fatalf("PartitionKey() = %q, want group-1", decoded.PartitionKey())
	}

	var decodedPayload GroupStateChangedPayload
	if err := json.Unmarshal(decoded.Data, &decodedPayload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	if decodedPayload.GroupUUID != "group-1" || decodedPayload.Version != 9 {
		t.Fatalf("payload = %#v, want group_uuid=group-1 version=9", decodedPayload)
	}
	if got := decodedPayload.Changed; len(got) != 2 || got[0] != GroupChangedInfo || got[1] != GroupChangedNotice {
		t.Fatalf("payload.Changed = %#v, want [info notice]", got)
	}
}

func TestGroupAggregateTargetRequiresGroupUUID(t *testing.T) {
	event := NewEvent(TypeGroupJoinRequestCreated, NewGroupAdminsTarget(" "), nil)
	event.ServerTs = 123

	if err := event.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want group_uuid error")
	}
}

func TestProducerPublishFillsDefaultsAndUsesPartitionKey(t *testing.T) {
	writer := &fakeWriter{}
	producer := NewProducer(writer)
	ctx := ctxmeta.WithTraceID(context.Background(), "trace-1")

	if err := producer.Publish(ctx, NewEvent(TypeFriendApplyCreated, NewUserTarget(" user-1 "), nil)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if string(writer.key) != "user-1" {
		t.Fatalf("key = %q, want user-1", writer.key)
	}

	var event Event
	if err := json.Unmarshal(writer.data, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.TraceID != "trace-1" {
		t.Fatalf("TraceID = %q, want trace-1", event.TraceID)
	}
	if event.ServerTs <= 0 {
		t.Fatalf("ServerTs = %d, want positive timestamp", event.ServerTs)
	}
	if event.Target.UserUUID != "user-1" {
		t.Fatalf("Target.UserUUID = %q, want user-1", event.Target.UserUUID)
	}
}
