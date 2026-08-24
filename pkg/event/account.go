package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// DecodeUserCreated 严格解析当前 user_created 事件负载。
func DecodeUserCreated(message []byte) (UserCreatedPayload, error) {
	return decodeEventPayload(message, func(payload *UserCreatedPayload) bool {
		return payload.EventID != "" && payload.UserUUID != ""
	})
}

// DecodeProfileDisplayChanged 严格解析当前 profile_display_changed 事件负载。
func DecodeProfileDisplayChanged(message []byte) (ProfileDisplayChangedPayload, error) {
	return decodeEventPayload(message, func(payload *ProfileDisplayChangedPayload) bool {
		return payload.EventID != "" && payload.UserUUID != ""
	})
}

// DecodeAccountDeleted 严格解析当前 account.deleted 事件负载。
func DecodeAccountDeleted(message []byte) (AccountDeletedPayload, error) {
	return decodeEventPayload(message, func(payload *AccountDeletedPayload) bool {
		return payload.EventID != "" && payload.UserUUID != ""
	})
}

func decodeEventPayload[T any](message []byte, isValid func(*T) bool) (T, error) {
	var zero T
	trimmed := bytes.TrimSpace(message)
	if len(trimmed) == 0 {
		return zero, errors.New("event payload is empty")
	}

	var payload T
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return zero, fmt.Errorf("event payload is not the current strict JSON contract: %w", err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return zero, errors.New("event payload contains multiple JSON values")
		}
		return zero, fmt.Errorf("event payload contains trailing data: %w", err)
	}

	if !isValid(&payload) {
		return zero, errors.New("event payload missing required fields")
	}
	return payload, nil
}
