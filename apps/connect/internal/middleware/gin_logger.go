package middleware

import (
	"net/http"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/httplog"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinLogger 记录 connect 服务的单条聚合 HTTP 日志。
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method
		path := c.Request.URL.Path
		query := httplog.SanitizeQuery(c.Request.URL.RawQuery)
		ip := ctxmeta.ClientIPFromGin(c)
		if ip == "" {
			ip = c.ClientIP()
		}

		c.Next()

		statusCode := c.Writer.Status()
		if path == "/health" && statusCode < 500 {
			return
		}

		ctx := ctxmeta.BuildContextFromGin(c)
		cost := time.Since(start)
		fields := []zap.Field{
			logger.String("method", method),
			logger.String("path", path),
			logger.String("query", query),
			logger.String("ip", ip),
			logger.Int("status", statusCode),
			logger.Duration("cost", cost),
		}

		if code, exists := c.Get("business_code"); exists {
			if businessCode, ok := code.(int); ok && businessCode > 0 {
				fields = append(fields, logger.Int("business_code", businessCode))
			}
		}

		switch {
		case path == "/ws" && statusCode < http.StatusInternalServerError:
			logger.Info(ctx, "Connect HTTP 请求完成", fields...)
		case statusCode >= 500:
			logger.Error(ctx, "Connect HTTP 请求完成", fields...)
		case cost > 2*time.Second:
			logger.Warn(ctx, "Connect HTTP 慢请求", fields...)
		default:
			logger.Info(ctx, "Connect HTTP 请求完成", fields...)
		}
	}
}
