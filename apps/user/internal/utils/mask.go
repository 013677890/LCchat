package utils

import "strings"

// MaskEmail 邮箱脱敏
// 示例：test@example.com -> t***@example.com
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || len(parts[0]) == 0 {
		return "***"
	}

	if len(parts[0]) == 1 {
		return "*@" + parts[1]
	}

	return parts[0][:1] + "***@" + parts[1]
}
