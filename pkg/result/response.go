package result

import (
	"net/http"
	"sync"
	"time"

	"github.com/013677890/LCchat-Backend/consts" // 你的错误码定义包
	"github.com/gin-gonic/gin"
)

// Response 响应结构体
type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	TraceId   string      `json:"trace_id"`
	Timestamp int64       `json:"timestamp"` //时间戳
}

var responsePool = &sync.Pool{
	New: func() interface{} {
		return &Response{}
	},
}

// GetResponse 获取响应
func GetResponse() *Response {
	resp := responsePool.Get().(*Response)
	resp.Code = 0
	resp.Message = ""
	resp.Data = nil
	resp.TraceId = ""
	resp.Timestamp = 0
	return resp
}

// PutResponse 放回响应
func PutResponse(resp *Response) {
	responsePool.Put(resp)
}

// Result 返回响应
// HTTP 状态码策略：
//   - 业务成功或业务失败（如参数错误、密码错误等）：返回 200，业务状态码在 body 的 code 字段
//   - 系统内部错误（code >= 30000）：返回 500
func Result(c *gin.Context, data interface{}, message string, code int) {
	ResultWithStatus(c, data, message, code, resolveHTTPStatus(code))
}

// ResultWithStatus 返回响应，并允许显式覆盖 HTTP 状态码。
func ResultWithStatus(c *gin.Context, data interface{}, message string, code int, httpStatus int) {
	traceId := c.GetString("trace_id")
	if message == "" {
		message = consts.GetMessage(code)
	}

	if httpStatus == 0 {
		httpStatus = resolveHTTPStatus(code)
	}

	// 将业务状态码存储到 context 中供监控中间件使用
	c.Set("business_code", code)

	resp := GetResponse()
	defer PutResponse(resp)
	resp.Code = code
	resp.Message = message
	resp.Data = data
	resp.TraceId = traceId
	resp.Timestamp = time.Now().Unix()

	c.JSON(httpStatus, resp)
}

// Success 返回成功响应
func Success(c *gin.Context, data interface{}) {
	Result(c, data, "", consts.CodeSuccess)
}

// Fail 返回失败响应
func Fail(c *gin.Context, data interface{}, code int) {
	Result(c, data, "", code)
}

// FailServer 返回失败响应，并将 upstreamErr 挂入 gin 错误链，供 GinLogger 在请求结束时统一记录（含当前层 apperr 堆栈）。
// 业务错误（如密码错误）请只用 Fail，不要传入 upstreamErr。
func FailServer(c *gin.Context, upstreamErr error, code int) {
	if upstreamErr != nil {
		_ = c.Error(upstreamErr)
	}
	Fail(c, nil, code)
}

// FailWithStatusMessage 返回失败响应，并显式指定 HTTP 状态码与消息。
func FailWithStatusMessage(c *gin.Context, data interface{}, message string, code int, httpStatus int) {
	ResultWithStatus(c, data, message, code, httpStatus)
}

func resolveHTTPStatus(code int) int {
	if code >= 30000 && code < 40000 {
		return http.StatusInternalServerError
	}
	return http.StatusOK
}
