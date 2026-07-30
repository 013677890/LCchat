package utils

import (
	"strings"
	"unicode/utf8"
)

// MaskEmail 对邮箱进行脱敏处理
// 示例: example@gmail.com -> e*****e@gmail.com
func MaskEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	username := parts[0]
	if utf8.RuneCountInString(username) <= 2 {
		return email
	}
	return string(username[0]) + "*****" + string(username[len(username)-1]) + "@" + parts[1]
}
