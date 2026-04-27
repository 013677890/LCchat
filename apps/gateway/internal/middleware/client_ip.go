// apps/gateway/internal/middleware/client_ip.go
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
	headerClientIP      = "Client-IP" // 或 X-Client-IP
	headerXClientIP     = "X-Client-IP"
)

// GetClientIP 从 Gin Context 中获取客户端真实 IP
// 优先级：X-Real-IP > X-Forwarded-For > Client-IP > RemoteAddr
func GetClientIP(c *gin.Context) string {
	// 1. 优先使用网关设置的真实 IP
	if ip := c.GetHeader(headerXRealIP); ip != "" {
		return strings.TrimSpace(ip)
	}

	// 2. 使用 X-Forwarded-For（代理链）
	if xff := c.GetHeader(headerXForwardedFor); xff != "" {
		// 取第一个 IP（原始客户端）
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// 3. 使用客户端传入的 IP（可选）
	if ip := c.GetHeader(headerClientIP); ip != "" {
		// 验证 IP 格式
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	if ip := c.GetHeader(headerXClientIP); ip != "" {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	// 4. 使用 Gin 的 ClientIP 方法（包含 RemoteAddr 逻辑）
	return c.ClientIP()
}

// GetClientIPSafe 安全获取 IP（包含验证）
func GetClientIPSafe(c *gin.Context) (string, bool) {
	ip := GetClientIP(c)
	if ip == "" {
		return "", false
	}

	// 验证 IP 格式
	if net.ParseIP(ip) == nil {
		return "", false
	}

	return ip, true
}

// GetClientIPOrDefault 获取 IP，失败时返回默认值
func GetClientIPOrDefault(c *gin.Context, defaultIP string) string {
	if ip, ok := GetClientIPSafe(c); ok {
		return ip
	}
	return defaultIP
}

// Middleware 注入 IP 到 Context
func ClientIPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := GetClientIP(c)

		// 注入到 Gin Context
		ctxmeta.SetClientIP(c, ip)

		// 注入到 request context（传递给下游）
		ctx := ctxmeta.WithClientIP(c.Request.Context(), ip)
		*c.Request = *c.Request.WithContext(ctx)

		c.Next()
	}
}

// 从 Gin Context 获取 IP（便捷方法）
func ClientIPFromGinContext(c *gin.Context) string {
	return ctxmeta.ClientIPFromGin(c)
}
