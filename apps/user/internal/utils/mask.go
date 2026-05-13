package utils

import "strings"

// MaskPhone 手机号脱敏
// 示例：13812345678 -> 138****5678
func MaskPhone(phone string) string {
	if len(phone) != 11 {
		return "***"
	}
	return phone[:3] + "****" + phone[7:]
}

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

// MaskName 姓名脱敏
// 示例：张三 -> 张*，张三丰 -> 张**
func MaskName(name string) string {
	runes := []rune(name)
	if len(runes) == 0 {
		return "***"
	}

	if len(runes) == 1 {
		return string(runes[0])
	}

	result := string(runes[0])
	for i := 1; i < len(runes); i++ {
		result += "*"
	}

	return result
}
