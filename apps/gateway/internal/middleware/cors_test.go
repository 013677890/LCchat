package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCorsMiddlewareUsesConfiguredWhitelist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 中间件在构造时读取配置，因此测试先固定环境变量，避免依赖开发机的本地默认白名单。
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://allowed.example")

	handlerCalls := 0
	router := gin.New()
	router.Use(CorsMiddleware())
	router.Any("/test", func(c *gin.Context) {
		handlerCalls++
		c.Status(http.StatusOK)
	})

	t.Run("白名单来源获得跨域响应头", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "https://allowed.example")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "https://allowed.example", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "Authorization, Content-Type, X-Device-ID, X-Requested-With", w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "POST, GET, OPTIONS, PUT, DELETE", w.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	})

	t.Run("非白名单来源不获得跨域授权", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "https://denied.example")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	})

	t.Run("预检请求在中间件中终止", func(t *testing.T) {
		callsBefore := handlerCalls
		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "https://allowed.example")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, callsBefore, handlerCalls)
	})
}
