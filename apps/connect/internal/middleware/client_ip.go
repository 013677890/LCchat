package middleware

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	headerXRealIP       = "X-Real-IP"
	headerXForwardedFor = "X-Forwarded-For"
	headerClientIP      = "Client-IP"
	headerXClientIP     = "X-Client-IP"
)

// ClientIPMiddleware 解析并注入客户端真实 IP。
// 优先级：
// 1. X-Real-IP
// 2. X-Forwarded-For（取首个合法 IP）
// 3. Client-IP / X-Client-IP
// 4. Gin 内建 ClientIP
func ClientIPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := resolveClientIP(c)
		if ip == "" {
			ip = c.ClientIP()
		}

		ctxmeta.SetClientIP(c, ip)
		c.Request = c.Request.WithContext(ctxmeta.WithClientIP(c.Request.Context(), ip))
		c.Next()
	}
}

// resolveClientIP 返回当前请求的真实客户端 IP（无端口）。
func resolveClientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}

	if ip := normalizeIP(c.GetHeader(headerXRealIP)); ip != "" {
		return ip
	}

	if xff := c.GetHeader(headerXForwardedFor); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			if ip := normalizeIP(strings.TrimSpace(part)); ip != "" {
				return ip
			}
		}
	}

	if ip := normalizeIP(c.GetHeader(headerClientIP)); ip != "" {
		return ip
	}
	if ip := normalizeIP(c.GetHeader(headerXClientIP)); ip != "" {
		return ip
	}

	return normalizeIP(c.ClientIP())
}

func normalizeIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}

	if parsed := net.ParseIP(raw); parsed != nil {
		return parsed.String()
	}
	return ""
}
