package middleware

import (
	"errors"
	"net"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"github.com/gin-gonic/gin"
)

// GinRecovery recover 项目可能出现的 panic
// stack: 是否打印堆栈信息
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				// 1. 构建上下文
				ctx := NewContextWithGin(c)
				// 2. 检查是否是管道错误
				var brokenPipe bool
				// 3. 检查是否是管道错误
				if ne, ok := recovered.(*net.OpError); ok {
					// 4. 检查是否是系统调用错误
					var se *os.SyscallError
					if errors.As(ne.Err, &se) {
						// 5. 检查是否是管道错误
						errStr := strings.ToLower(se.Error())
						if strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "connection reset by peer") {
							brokenPipe = true
						}
					}
				}
				// 6. 检查是否是管道错误
				if brokenPipe {
					// 7. 记录日志
					logger.Warn(ctx, "客户端断开连接",
						logger.Any("error", recovered),
						logger.String("method", c.Request.Method),
						logger.String("path", c.Request.URL.Path),
						logger.String("ip", c.ClientIP()),
					)
					// 8. 中止请求
					c.Abort()
					return
				}

				// 9. 记录日志
				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				// 10. 创建错误
				panicErr := apperr.NewFromPanic(recovered)
				// 11. 记录日志
				logger.Error(ctx, "Gateway 捕获到 panic",
					logger.Any("error", recovered),
					logger.String("method", c.Request.Method),
					logger.String("path", c.Request.URL.Path),
					logger.String("query", c.Request.URL.RawQuery),
					logger.String("ip", c.ClientIP()),
					logger.String("user-agent", c.Request.UserAgent()),
					logger.String("request", string(httpRequest)),
					logger.String("top_frame", apperr.TopFrame(panicErr)),
					logger.StackFrames("stack", apperr.Frames(panicErr)),
				)
				// 12. 标记错误已记录
				apperr.MarkLogged(panicErr)
				// 13. 返回错误
				result.Fail(c, nil, consts.CodeInternalError)
			}
		}()
		// 14. 继续执行请求
		c.Next()
	}
}
