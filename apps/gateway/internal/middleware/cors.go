package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// defaultAllowedOrigins 未配置 CORS_ALLOWED_ORIGINS 时使用的本地开发白名单。
var defaultAllowedOrigins = []string{
	"http://localhost:5173", // Vite/Electron 开发服务器
	"http://127.0.0.1:5173",
	"http://localhost:8080", // Web 本地预览
	"http://127.0.0.1:8080",
}

// CorsMiddleware 跨域中间件。
//
// 允许的来源通过环境变量 CORS_ALLOWED_ORIGINS（逗号分隔）配置，未配置时回退到本地开发白名单。
// 仅当请求 Origin 命中白名单时才回显该 Origin 并允许携带凭据；其余来源不下发任何 CORS 头，
// 由浏览器拦截。这样可避免把“带凭据的跨域访问”开放给任意站点（反射任意 Origin + 凭据的风险）。
//
// 注意：出于安全考虑，本中间件不支持 "*" 通配；如需放开请显式列出来源。
func CorsMiddleware() gin.HandlerFunc {
	allowed := loadAllowedOrigins()

	return func(c *gin.Context) {
		// Origin 会影响响应内容，需告知缓存按 Origin 区分。
		c.Header("Vary", "Origin")

		origin := c.Request.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin) // 仅回显白名单内的具体 Origin
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Device-ID, X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		}

		// 处理 OPTIONS 预检请求：命中白名单则浏览器据上面的头放行，否则缺少 CORS 头被浏览器拦截。
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// loadAllowedOrigins 读取 CORS_ALLOWED_ORIGINS（逗号分隔）并构造白名单集合；未配置时使用开发默认值。
func loadAllowedOrigins() map[string]bool {
	origins := defaultAllowedOrigins
	if raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); raw != "" {
		origins = nil // 显式配置时不再混入默认白名单
		for _, part := range strings.Split(raw, ",") {
			if item := strings.TrimSpace(part); item != "" {
				origins = append(origins, item)
			}
		}
	}

	set := make(map[string]bool, len(origins))
	for _, o := range origins {
		set[o] = true
	}
	return set
}
