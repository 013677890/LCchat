package util

import "time"

// FormatUnixMilliRFC3339 将毫秒时间戳格式化为 RFC3339（UTC）
func FormatUnixMilliRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.Unix(0, ms*int64(time.Millisecond)).UTC().Format(time.RFC3339)
}
