package groupevent

import (
	"bytes"
	"encoding/json"
	"errors"
)

const EventTypeGroupCache = "group.cache"
const (
	ActionGroupCreated      = "group_created"
	ActionMemberAdded       = "member_added"
	ActionMemberRemoved     = "member_removed"
	ActionGroupDismissed    = "group_dismissed"
	ActionGroupInfoUpdated  = "group_info_updated"
	ActionOwnerTransferred  = "owner_transferred"
	ActionMemberRoleUpdated = "member_role_updated"
)

type GroupSnapshot struct {
	GroupUUID     string `json:"group_uuid"`
	Name          string `json:"name"`
	Avatar        string `json:"avatar"`
	Notice        string `json:"notice"`
	OwnerUUID     string `json:"owner_uuid"`
	MemberCount   int32  `json:"member_count"`
	AddMode       int32  `json:"add_mode"`
	Status        int32  `json:"status"`
	UpdatedAtUnix int64  `json:"updated_at_unix"`
}
type GroupMemberSnapshot struct {
	UserUUID       string `json:"user_uuid"`
	Role           int32  `json:"role"`
	JoinedAtUnixMs int64  `json:"joined_at_unix_ms"`
}
type GroupCacheEventPayload struct {
	EventID        string                `json:"event_id"`
	Action         string                `json:"action"`
	GroupUUID      string                `json:"group_uuid"`
	OperatorUUID   string                `json:"operator_uuid,omitempty"`
	Group          *GroupSnapshot        `json:"group,omitempty"`
	Members        []GroupMemberSnapshot `json:"members,omitempty"`
	UserUUID       string                `json:"user_uuid,omitempty"`
	UserUUIDs      []string              `json:"user_uuids,omitempty"`
	JoinedAtUnixMs int64                 `json:"joined_at_unix_ms,omitempty"`
}

func Encode(payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
func DecodeGroupCache(message []byte) (GroupCacheEventPayload, error) {
	return decodeEventPayload(message, func(payload *GroupCacheEventPayload) bool {
		return payload.EventID != "" && payload.GroupUUID != "" && payload.Action != ""
	})
}
func decodeEventPayload[T any](message []byte, isValid func(*T) bool) (T, error) {
	var zero T
	var lastErr error
	for _, candidate := range collectPayloadCandidates(message, 0, map[string]struct{}{}) {
		var payload T
		if err := json.Unmarshal(candidate, &payload); err != nil {
			lastErr = err
			continue
		}
		if isValid(&payload) {
			return payload, nil
		}
	}
	if lastErr != nil {
		return zero, lastErr
	}
	return zero, errors.New("event payload missing required fields")
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
