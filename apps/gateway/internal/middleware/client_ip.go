// apps/gateway/internal/middleware/client_ip.go
package middleware

import (
	"context"
	"net"
	"os"
	"strings"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// ConfigureTrustedProxies 按环境变量 GATEWAY_TRUSTED_PROXIES（逗号分隔的 IP/CIDR）配置 gin 可信代理。
// 必须在注册路由前调用。
//
// 安全语义：
//   - 未配置（空）时不信任任何代理，c.ClientIP() 只返回直连地址 RemoteAddr，
//     忽略一切可被客户端伪造的 X-Real-IP / X-Forwarded-For，避免伪造来源 IP 绕过限流/黑名单；
//   - 配置为反代 / LB 所在网段后，仅当直连 peer 属于可信代理时，才按 X-Real-IP /
//     X-Forwarded-For 解析真实客户端 IP（XFF 从右向左跳过可信跳）。
//
// 部署提示：网关位于 nginx / ingress 之后时，应把该变量设为这些代理的 IP/网段
// （例如容器网络 CIDR），否则取到的将是代理自身地址。
func ConfigureTrustedProxies(r *gin.Engine) {
	r.ForwardedByClientIP = true
	// 与历史行为保持一致的取值优先级：先 X-Real-IP，再 X-Forwarded-For。
	r.RemoteIPHeaders = []string{"X-Real-IP", "X-Forwarded-For"}

	proxies := parseTrustedProxies(os.Getenv("GATEWAY_TRUSTED_PROXIES"))
	if err := r.SetTrustedProxies(proxies); err != nil {
		// 配置非法时回退到“不信任任何代理”，保证安全默认而非沿用 gin 的信任所有。
		logger.Warn(context.Background(), "GATEWAY_TRUSTED_PROXIES 配置无效，回退为仅信任直连地址",
			logger.ErrorField("error", err),
		)
		_ = r.SetTrustedProxies(nil)
	}
}

// parseTrustedProxies 解析逗号分隔的可信代理列表；空字符串返回 nil（不信任任何代理）。
func parseTrustedProxies(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var proxies []string
	for _, item := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(item); v != "" {
			proxies = append(proxies, v)
		}
	}
	return proxies
}

// GetClientIP 返回客户端真实 IP。
// 委托给 gin 的 c.ClientIP()，由其依据 ConfigureTrustedProxies 设置的可信代理边界，
// 安全地解析 X-Real-IP / X-Forwarded-For 或回退到直连地址，不再无条件信任转发头。
func GetClientIP(c *gin.Context) string {
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

// ClientIPMiddleware 注入 IP 到 Context
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
