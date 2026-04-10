package middleware

import (
	"errors"
	"net"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"github.com/gin-gonic/gin"
)

// RecoverMiddleware 捕获握手阶段 panic，避免进程崩溃。
func RecoverMiddleware(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				ctx := ctxmeta.BuildContextFromGin(c)
				var brokenPipe bool
				if ne, ok := recovered.(*net.OpError); ok {
					var se *os.SyscallError
					if errors.As(ne.Err, &se) {
						errText := strings.ToLower(se.Error())
						if strings.Contains(errText, "broken pipe") || strings.Contains(errText, "connection reset by peer") {
							brokenPipe = true
						}
					}
				}
				if brokenPipe {
					logger.Warn(ctx, "客户端在握手阶段提前断开连接",
						logger.Any("error", recovered),
						logger.String("method", c.Request.Method),
						logger.String("path", c.Request.URL.Path),
						logger.String("ip", c.ClientIP()),
					)
					c.Abort()
					return
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				panicErr := apperr.NewFromPanic(recovered)
				logger.Error(ctx, "Connect 服务捕获到 panic",
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
				apperr.MarkLogged(panicErr)
				result.Fail(c, nil, consts.CodeInternalError)
				c.Abort()
			}
		}()
		c.Next()
	}
}
