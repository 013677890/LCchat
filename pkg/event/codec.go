package event

import "encoding/json"

// Encode 将事件负载序列化为 JSON 字符串，便于写入 outbox_events。
func Encode(payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
