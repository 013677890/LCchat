package event

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeUserCreatedAcceptsCurrentPayload(t *testing.T) {
	direct := `{"event_id":"evt-1","user_uuid":"user-1","nickname":"Alice"}`
	payload, err := DecodeUserCreated([]byte(direct))
	require.NoError(t, err)
	assert.Equal(t, "evt-1", payload.EventID)
	assert.Equal(t, "user-1", payload.UserUUID)
}

func TestDecodeUserCreatedRejectsNonCurrentPayloads(t *testing.T) {
	direct := `{"event_id":"evt-1","user_uuid":"user-1","nickname":"Alice"}`
	stringWrapped, err := json.Marshal(direct)
	require.NoError(t, err)

	tests := map[string][]byte{
		"json string":     stringWrapped,
		"envelope":        []byte(`{"payload":` + direct + `}`),
		"unknown field":   []byte(`{"event_id":"evt-1","user_uuid":"user-1","future":"x"}`),
		"multiple values": []byte(direct + ` {}`),
	}
	for name, message := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeUserCreated(message)
			require.Error(t, err)
		})
	}
}

func TestDecodeUserCreatedRejectsMissingRequiredFields(t *testing.T) {
	_, err := DecodeUserCreated([]byte(`{"event_id":"evt-1"}`))
	require.EqualError(t, err, "event payload missing required fields")
}
