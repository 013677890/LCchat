package middleware

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGinRecoveryHandlesPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Recovery 会记录结构化日志；测试使用空 logger，只验证响应与流程，不产生控制台噪声。
	logger.ReplaceGlobal(zap.NewNop())

	tests := []struct {
		name   string
		panicV any
	}{
		{name: "字符串 panic", panicV: "test panic"},
		{name: "错误 panic", panicV: errors.New("test error")},
		{name: "整数 panic", panicV: 12345},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(GinRecovery(true))
			router.GET("/test", func(*gin.Context) { panic(tt.panicV) })
			w := httptest.NewRecorder()

			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

			var body struct {
				Code int `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, consts.CodeInternalError, body.Code)
		})
	}
}

func TestGinRecoveryKeepsNormalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GinRecovery(false))
	router.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	w := httptest.NewRecorder()

	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func TestGinRecoveryStopsWritingAfterBrokenPipe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.ReplaceGlobal(zap.NewNop())
	brokenPipe := &net.OpError{
		Op:  "write",
		Err: os.NewSyscallError("write", errors.New("broken pipe")),
	}
	router := gin.New()
	router.Use(GinRecovery(true))
	router.GET("/test", func(*gin.Context) { panic(brokenPipe) })
	w := httptest.NewRecorder()

	// 客户端已断开时不能再写 JSON；关键保证是请求被中止且 panic 不逃逸到进程。
	assert.NotPanics(t, func() {
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))
	})
	assert.Empty(t, w.Body.String())
}
