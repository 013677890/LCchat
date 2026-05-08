package middleware

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewContextWithGin 从 gin.Context 创建包含 trace_id、user_uuid、device_id 的 context.Context
// 用于将 Gin 上下文中的 trace_id、user_uuid、device_id 传递到日志系统
func NewContextWithGin(c *gin.Context) context.Context {
	return ctxmeta.BuildContextFromGin(c)
}

// GinLogger 在请求结束后输出单条聚合访问日志。
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		clientIP := ctxmeta.ClientIPFromGin(c)
		if clientIP == "" {
			clientIP = c.ClientIP()
		}

		c.Next()
		if IsPlatformPath(path) && c.Writer.Status() < 500 {
			return
		}

		ctx := NewContextWithGin(c)
		cost := time.Since(start)
		statusCode := c.Writer.Status()
		fields := []zap.Field{
			logger.String("method", c.Request.Method),
			logger.String("path", path),
			logger.String("query", query),
			logger.String("ip", clientIP),
			logger.Int("status", statusCode),
			logger.Duration("cost", cost),
		}

		businessCode := 0
		if code, exists := c.Get("business_code"); exists {
			if bc, ok := code.(int); ok && bc > 0 {
				businessCode = bc
				fields = append(fields, logger.Int("business_code", bc))
			}
		}

		if len(c.Errors) > 0 {
			for _, ge := range c.Errors {
				if ge.Err == nil {
					continue
				}
				fields = append(fields, logger.ErrorField("error", ge.Err))
				if top := apperr.TopFrame(ge.Err); top != "" {
					fields = append(fields, logger.String("top_frame", top))
					fields = append(fields, logger.StackFrames("stack", apperr.Frames(ge.Err)))
				}
				break
			}
		}

		switch {
		case businessCode >= 30000:
			logger.Error(ctx, "Gateway HTTP 请求错误", fields...)
		case statusCode >= 500:
			logger.Error(ctx, "Gateway HTTP 请求错误", fields...)
		case cost > 2*time.Second:
			logger.Warn(ctx, "Gateway HTTP 慢请求", fields...)
		default:
			logger.Info(ctx, "Gateway HTTP 请求成功", fields...)
		}
	}
}
