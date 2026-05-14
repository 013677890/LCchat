package manager

import "testing"

func TestRecordDeliveredSeqOnlyMovesForward(t *testing.T) {
	client := NewClient(nil, "user-1", "dev-1")
	client.RecordDeliveredSeq("conv-1", 10)
	client.RecordDeliveredSeq("conv-1", 8)
	client.RecordDeliveredSeq("conv-1", 12)

	if got := client.MaxDeliveredSeq("conv-1"); got != 12 {
		t.Fatalf("expected max seq 12, got %d", got)
	}
}

func TestDeliveryMetadataRecordable(t *testing.T) {
	if (DeliveryMetadata{ConvID: "conv-1", Seq: 1, AckRequired: true}).recordable() != true {
		t.Fatalf("expected metadata to be recordable")
	}
	if (DeliveryMetadata{ConvID: "", Seq: 1, AckRequired: true}).recordable() != false {
		t.Fatalf("expected empty conv id to be non-recordable")
	}
	if (DeliveryMetadata{ConvID: "conv-1", Seq: 0, AckRequired: true}).recordable() != false {
		t.Fatalf("expected zero seq to be non-recordable")
	}
	if (DeliveryMetadata{ConvID: "conv-1", Seq: 1, AckRequired: false}).recordable() != false {
		t.Fatalf("expected ack not required to be non-recordable")
	}
}
