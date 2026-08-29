package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestBuildCheckOriginAllowsConfiguredElectronFileOrigin(t *testing.T) {
	t.Setenv("CONNECT_ALLOWED_ORIGINS", "http://localhost:5173,file://")

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8081/ws", nil)
	req.Header.Set("Origin", "file://")

	if !buildCheckOrigin()(req) {
		t.Fatal("configured Electron file:// origin should be allowed")
	}
}

func TestBuildCheckOriginRejectsUnconfiguredElectronFileOrigin(t *testing.T) {
	t.Setenv("CONNECT_ALLOWED_ORIGINS", "http://localhost:5173")

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8081/ws", nil)
	req.Header.Set("Origin", "file://")

	if buildCheckOrigin()(req) {
		t.Fatal("unconfigured Electron file:// origin should be rejected")
	}
}
