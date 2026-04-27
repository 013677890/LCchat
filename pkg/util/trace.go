package util

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderXRequestID = ctxmeta.HeaderRequestID

// TraceLogger 追踪中间件，生成或获取 trace_id 并存入 Gin 上下文
func TraceLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 尝试从请求头拿（防止是 Nginx 传过来的）
		traceId := c.GetHeader(HeaderXRequestID)

		// 2. 如果没有，自己生成一个
		if traceId == "" {
			traceId = uuid.New().String()
		}

		// 3. 【关键】放入 Gin 的上下文，供后续 Controller 使用
		ctxmeta.SetTraceID(c, traceId)

		// 4. 【关键】放入响应头，方便前端/客户端拿着 ID 来找你报修
		c.Header(HeaderXRequestID, traceId)

		c.Next()
	}
}

// NewUUID 生成新的 UUID
func NewUUID() string {
	return uuid.New().String()
}
