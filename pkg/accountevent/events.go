package accountevent

import (
	"encoding/json"
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
