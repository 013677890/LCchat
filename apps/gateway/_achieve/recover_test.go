package middleware

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"ChatServer/pkg/logger"
)

// init 初始化 logger（测试模式，不输出日志）
func init() {
	testLogger := zap.NewNop()
	logger.ReplaceGlobal(testLogger)
}

// TestGinRecovery_Panic 测试 panic 恢复
func TestGinRecovery_Panic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := GinRecovery(true) // 启用堆栈

	tests := []struct {
		name          string
		panicReason  interface{}
		expectedCode  int
		expectedMsg   string
	}{
		{
			name:         "字符串 panic",
			panicReason:  "test panic",
			expectedCode:  http.StatusInternalServerError,
			expectedMsg:   "服务器内部错误",
		},
		{
			name:         "错误 panic",
			panicReason:  errors.New("test error"),
			expectedCode:  http.StatusInternalServerError,
			expectedMsg:   "服务器内部错误",
		},
		{
			name:         "整数 panic",
			panicReason:  12345,
			expectedCode:  http.StatusInternalServerError,
			expectedMsg:   "服务器内部错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panicHandler := func(c *gin.Context) {
				panic(tt.panicReason)
			}

			router := gin.New()
			router.Use(middleware)
			router.GET("/test", panicHandler)

			req, _ := http.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// 验证响应状态码
			assert.Equal(t, tt.expectedCode, w.Code, "应该返回 500")

			// 验证响应体包含错误码
			body := w.Body.String()
			assert.Contains(t, body, `"code":30001`, "响应应该包含错误码")
		})
	}
}

// TestGinRecovery_NoPanic 测试正常情况（无 panic）
func TestGinRecovery_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := GinRecovery(true)

	successHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", successHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 正常情况应该返回 200
	assert.Equal(t, http.StatusOK, w.Code, "应该返回 200")
	assert.Contains(t, w.Body.String(), "success", "响应应该包含 success")
}

// TestGinRecovery_BrokenPipe 测试 Broken Pipe 错误（客户端断开连接）
func TestGinRecovery_BrokenPipe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := GinRecovery(true)

	// 模拟 Broken Pipe 错误
	networkErr := &net.OpError{
		Op:  "write",
		Err:  os.NewSyscallError("write", errors.New("broken pipe")),
	}

	panicHandler := func(c *gin.Context) {
		panic(networkErr)
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", panicHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Broken Pipe 会被捕获并处理，不会返回响应给客户端
	// 因为连接已经断开，无法写入响应
	// 这里的测试主要是验证不会导致进程崩溃
	assert.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	}, "不应该发生 panic")
}

// TestGinRecovery_ConnectionResetByPeer 测试 Connection Reset By Peer 错误
func TestGinRecovery_ConnectionResetByPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := GinRecovery(true)

	// 模拟 Connection Reset By Peer 错误
	networkErr := &net.OpError{
		Op:  "write",
		Err:  os.NewSyscallError("write", errors.New("connection reset by peer")),
	}

	panicHandler := func(c *gin.Context) {
		panic(networkErr)
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", panicHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	// 验证不会导致进程崩溃
	assert.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	}, "不应该发生 panic")
}

// TestGinRecovery_NormalNetworkError 测试普通网络错误（不是 Broken Pipe）
func TestGinRecovery_NormalNetworkError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := GinRecovery(true)

	// 模拟普通网络错误
	networkErr := &net.OpError{
		Op:  "write",
		Err:  errors.New("connection timeout"),
	}

	panicHandler := func(c *gin.Context) {
		panic(networkErr)
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", panicHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 普通网络错误应该被当作 panic 处理，返回 500
	assert.Equal(t, http.StatusInternalServerError, w.Code, "应该返回 500")
	assert.Contains(t, w.Body.String(), `"code":30001`, "响应应该包含错误码")
}

// TestGinRecovery_WithoutStack 测试不打印堆栈信息
func TestGinRecovery_WithoutStack(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := GinRecovery(false) // 禁用堆栈

	panicHandler := func(c *gin.Context) {
		panic("test panic")
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", panicHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该返回 500
	assert.Equal(t, http.StatusInternalServerError, w.Code, "应该返回 500")
	assert.Contains(t, w.Body.String(), `"code":30001`, "响应应该包含错误码")
}
