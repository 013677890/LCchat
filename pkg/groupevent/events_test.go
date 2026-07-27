package groupevent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validStrictGroupCacheMessage(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(GroupCacheEventPayload{
		SchemaVersion:     GroupCacheSchemaVersion,
		ProjectionVersion: 8,
		EventID:           "event-8",
		Action:            ActionJoinRequestReviewed,
		GroupUUID:         "group-1",
		JoinRequest: &GroupJoinRequestSnapshot{
			ApplyID:         1,
			ApplicantUUID:   "user-1",
			CreatedAtUnixMs: 1710000000123,
		},
	})
	require.NoError(t, err)
	return data
}

func TestDecodeGroupCacheAcceptsOnlyCurrentDirectPayload(t *testing.T) {
	payload, err := DecodeGroupCache(validStrictGroupCacheMessage(t))
	require.NoError(t, err)
	assert.Equal(t, GroupCacheSchemaVersion, payload.SchemaVersion)
	assert.Equal(t, int64(8), payload.ProjectionVersion)
	assert.Equal(t, "event-8", payload.EventID)
}

func TestDecodeGroupCacheRejectsLegacyWrappersAndMissingVersion(t *testing.T) {
	direct := validStrictGroupCacheMessage(t)
	quoted, err := json.Marshal(string(direct))
	require.NoError(t, err)
	wrapped, err := json.Marshal(map[string]json.RawMessage{"payload": direct})
	require.NoError(t, err)

	var missingVersion map[string]any
	require.NoError(t, json.Unmarshal(direct, &missingVersion))
	delete(missingVersion, "projection_version")
	missingVersionBytes, err := json.Marshal(missingVersion)
	require.NoError(t, err)

	for name, message := range map[string][]byte{
		"JSON 字符串包装":   quoted,
		"payload 对象包装": wrapped,
		"缺少投影版本":       missingVersionBytes,
	} {
		t.Run(name, func(t *testing.T) {
			_, decodeErr := DecodeGroupCache(message)
			assert.Error(t, decodeErr)
		})
	}
}

func TestDecodeGroupCacheRejectsUnknownLegacyFieldsAndTrailingJSON(t *testing.T) {
	direct := validStrictGroupCacheMessage(t)
	var object map[string]any
	require.NoError(t, json.Unmarshal(direct, &object))
	for name, value := range map[string]any{
		"旧版顶层 joined_at": int64(1710000000123),
		"未知可选字段":         true,
	} {
		t.Run(name, func(t *testing.T) {
			objectCopy := make(map[string]any, len(object)+1)
			for key, fieldValue := range object {
				objectCopy[key] = fieldValue
			}
			fieldName := "future_optional_field"
			if name == "旧版顶层 joined_at" {
				fieldName = "joined_at_unix_ms"
			}
			objectCopy[fieldName] = value
			withUnknown, err := json.Marshal(objectCopy)
			require.NoError(t, err)
			_, err = DecodeGroupCache(withUnknown)
			assert.Error(t, err)
		})
	}

	_, err := DecodeGroupCache(append(direct, []byte(` {}`)...))
	assert.Error(t, err)
}
