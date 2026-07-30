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
		spanID := c.GetHeader(ctxmeta.HeaderSpanID)
		parentSpanID := c.GetHeader(ctxmeta.HeaderParentSpanID)

		// 2. 如果没有，自己生成一个
		if traceId == "" {
			traceId = uuid.New().String()
		}
		if spanID == "" {
			spanID = ctxmeta.NewSpanID()
		}

		// 3. 【关键】放入 Gin 的上下文，供后续 Controller 使用
		traceId = ctxmeta.SetTraceID(c, traceId)
		spanID = ctxmeta.SetSpanID(c, spanID)
		parentSpanID = ctxmeta.SetParentSpanID(c, parentSpanID)

		reqCtx := ctxmeta.WithTraceID(c.Request.Context(), traceId)
		reqCtx = ctxmeta.WithSpanID(reqCtx, spanID)
		reqCtx = ctxmeta.WithParentSpanID(reqCtx, parentSpanID)
		c.Request = c.Request.WithContext(reqCtx)

		// 4. 【关键】放入响应头，方便前端/客户端拿着 ID 来找你报修
		c.Header(HeaderXRequestID, traceId)
		c.Header(ctxmeta.HeaderSpanID, spanID)
		if parentSpanID != "" {
			c.Header(ctxmeta.HeaderParentSpanID, parentSpanID)
		}

		c.Next()
	}
}
