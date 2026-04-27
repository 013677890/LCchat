package handler

import "testing"

func TestMessageAckSeqWithinDelivered(t *testing.T) {
	tests := []struct {
		name            string
		ackSeq          int64
		maxDeliveredSeq int64
		want            bool
	}{
		{name: "equal delivered seq", ackSeq: 5, maxDeliveredSeq: 5, want: true},
		{name: "below delivered seq", ackSeq: 4, maxDeliveredSeq: 5, want: true},
		{name: "zero ack seq", ackSeq: 0, maxDeliveredSeq: 5, want: false},
		{name: "beyond delivered seq", ackSeq: 6, maxDeliveredSeq: 5, want: false},
		{name: "no delivered seq", ackSeq: 1, maxDeliveredSeq: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := messageAckSeqWithinDelivered(tt.ackSeq, tt.maxDeliveredSeq)
			if got != tt.want {
				t.Fatalf("messageAckSeqWithinDelivered(%d, %d) = %v, want %v", tt.ackSeq, tt.maxDeliveredSeq, got, tt.want)
			}
		})
	}
}
