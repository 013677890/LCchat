package middleware

import (
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// TimeoutMiddleware 请求超时控制中间件
// 安全版本：不开启 Goroutine，依赖下游 Context 感知
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 创建带超时的 context
		// 注意：这里基于 c.Request.Context() 派生
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// 2. 替换请求的 context
		// 这样后续的 Handler、gRPC 调用都能拿到这个有时间限制的 ctx
		c.Request = c.Request.WithContext(ctx)

		// 3. 直接在当前协程执行
		// 假如 handler 里面调用了 gRPC，gRPC 客户端发现 ctx 超时会自动返回 deadline exceeded
		c.Next()

		// 4. 后置检查：检查处理过程中是否发生了超时
		// 注意：如果 c.Next() 里已经处理了超时并返回了响应（比如我们在 Handler 里做了错误处理），
		// 这里就需要判断是否还需要写入。
		
		// 这里的逻辑稍微有点绕：
		// 情况 A: 下游 gRPC 即使超时了，Handler 捕获了错误并正常返回了 JSON (code=500)。
		//         此时 c.Writer.Written() 为 true。我们啥都不用做。
		// 情况 B: 下游处理得太慢，甚至没来得及写 Response，ctx 就过期了。
		
		if ctx.Err() == context.DeadlineExceeded {
			// 只有当 Response 还没写出去的时候，中间件才介入兜底
			if !c.Writer.Written() {
				logCtx := NewContextWithGin(c)
				logger.Warn(logCtx, "网关层强制超时断开",
					logger.String("path", c.Request.URL.Path),
					logger.Duration("timeout", timeout),
				)
				
				// 强制返回 500 Gateway Timeout
				result.Fail(c, nil, consts.CodeTimeoutError)

			}
		}
	}
}

// TimeoutMiddlewareWithPath 同样修改为安全版本
func TimeoutMiddlewareWithPath(pathTimeouts map[string]time.Duration, defaultTimeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		timeout := defaultTimeout
		if t, exists := pathTimeouts[c.Request.URL.Path]; exists {
			timeout = t
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		// 兜底超时处理
		if ctx.Err() == context.DeadlineExceeded {
			if !c.Writer.Written() {
				logger.Warn(context.Background(), "请求超时",
					logger.String("path", c.Request.URL.Path),
				)
				result.Fail(c, nil, consts.CodeTimeoutError)
			}
		}
	}
}