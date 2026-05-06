package accountevent

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

const (
	// EventTypeUserCreated 表示账号创建后触发的资料初始化事件。
	EventTypeUserCreated = "user_created"
	// EventTypeProfileDisplayChanged 表示资料展示字段变更后的回写事件。
	EventTypeProfileDisplayChanged = "profile_display_changed"
	// EventTypeAccountDeleted 表示账号注销后的跨服务清理事件。
	EventTypeAccountDeleted = "account.deleted"
)

// UserCreatedPayload 描述注册成功后用于初始化资料的事件负载。
type UserCreatedPayload struct {
	EventID  string `json:"event_id"`
	UserUUID string `json:"user_uuid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// ProfileDisplayChangedPayload 描述资料展示字段变更后的同步负载。
type ProfileDisplayChangedPayload struct {
	EventID  string `json:"event_id"`
	UserUUID string `json:"user_uuid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// AccountDeletedPayload 描述账号注销后的广播负载。
type AccountDeletedPayload struct {
	EventID   string    `json:"event_id"`
	UserUUID  string    `json:"user_uuid"`
	DeletedAt time.Time `json:"deleted_at"`
}

// Encode 将事件负载序列化为 JSON 字符串，便于写入 outbox_events。
func Encode(payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DecodeUserCreated 兼容解析 user_created 事件负载。
func DecodeUserCreated(message []byte) (UserCreatedPayload, error) {
	return decodeEventPayload(message, func(payload *UserCreatedPayload) bool {
		return payload.EventID != "" && payload.UserUUID != ""
	})
}

// DecodeProfileDisplayChanged 兼容解析 profile_display_changed 事件负载。
func DecodeProfileDisplayChanged(message []byte) (ProfileDisplayChangedPayload, error) {
	return decodeEventPayload(message, func(payload *ProfileDisplayChangedPayload) bool {
		return payload.EventID != "" && payload.UserUUID != ""
	})
}

// DecodeAccountDeleted 兼容解析 account.deleted 事件负载。
func DecodeAccountDeleted(message []byte) (AccountDeletedPayload, error) {
	return decodeEventPayload(message, func(payload *AccountDeletedPayload) bool {
		return payload.EventID != "" && payload.UserUUID != ""
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
