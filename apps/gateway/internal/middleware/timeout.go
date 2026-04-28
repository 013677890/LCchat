package middleware

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"github.com/gin-gonic/gin"
)

// TimeoutMiddleware 请求超时控制中间件。
// 它基于父请求 ctx 派生 deadline：若父 ctx 已经更短，则自动保留更短的那个。
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return TimeoutMiddlewareWithPath(nil, timeout)
}

// TimeoutMiddlewareWithPath 为不同路由提供超时覆盖。
// 路由匹配优先使用 gin 的 FullPath（如 /users/:id），取不到时再回退到 URL.Path。
func TimeoutMiddlewareWithPath(pathTimeouts map[string]time.Duration, defaultTimeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		timeout := resolveRequestTimeout(c, pathTimeouts, defaultTimeout)
		if timeout <= 0 {
			c.Next()
			return
		}

		ctx, cancel, effectiveTimeout := newRequestTimeoutContext(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		if ctx.Err() != context.DeadlineExceeded || c.Writer.Written() {
			return
		}

		logCtx := NewContextWithGin(c)
		logger.Warn(logCtx, "网关层强制超时断开",
			logger.String("path", requestRouteKey(c)),
			logger.Duration("timeout", effectiveTimeout),
		)
		result.Fail(c, nil, consts.CodeTimeoutError)
	}
}

func newRequestTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, time.Duration) {
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		if remaining <= timeout {
			ctx, cancel := context.WithCancel(parent)
			return ctx, cancel, remaining
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, timeout
}

func resolveRequestTimeout(c *gin.Context, pathTimeouts map[string]time.Duration, defaultTimeout time.Duration) time.Duration {
	if len(pathTimeouts) == 0 {
		return defaultTimeout
	}

	if routeKey := requestRouteKey(c); routeKey != "" {
		if timeout, exists := pathTimeouts[routeKey]; exists {
			return timeout
		}
	}

	return defaultTimeout
}

func requestRouteKey(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if fullPath := c.FullPath(); fullPath != "" {
		return fullPath
	}
	if c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}