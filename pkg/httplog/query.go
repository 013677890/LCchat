// Package httplog 提供 HTTP 边界日志的敏感字段清理工具。
package httplog

import (
	"net/url"
	"strings"
)

const redactedValue = "REDACTED"

// SanitizeQuery 清理查询参数中的凭据后返回可安全记录的字符串。
//
// 非法查询字符串不回退输出原文，避免解析失败时反而泄露敏感值。
func SanitizeQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "INVALID_QUERY"
	}
	for key := range values {
		if isSensitiveQueryKey(key) {
			values[key] = []string{redactedValue}
		}
	}
	return values.Encode()
}

func isSensitiveQueryKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "verifycode") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "apikey")
}
