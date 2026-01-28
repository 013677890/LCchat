package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestCorsMiddleware 测试 CORS 中间件
func TestCorsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := CorsMiddleware()

	successHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	router := gin.New()
	router.Use(middleware)
	router.Any("/test", successHandler) // 允许所有 HTTP 方法

	t.Run("OPTIONS 预检请求", func(t *testing.T) {
		req, _ := http.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://localhost:8080")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// OPTIONS 请求应该返回 204 No Content
		assert.Equal(t, http.StatusNoContent, w.Code, "OPTIONS 请求应该返回 204")

		// 验证 CORS 响应头
		assert.Equal(t, "http://localhost:8080", w.Header().Get("Access-Control-Allow-Origin"), "应该设置允许的 Origin")
		assert.Equal(t, "Authorization, Content-Type, x-requested-with", w.Header().Get("Access-Control-Allow-Headers"), "应该设置允许的 Headers")
		assert.Equal(t, "POST, GET, OPTIONS, PUT, DELETE", w.Header().Get("Access-Control-Allow-Methods"), "应该设置允许的 Methods")
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"), "应该允许携带凭据")
		assert.Equal(t, "Origin", w.Header().Get("Vary"), "应该设置 Vary 头")
	})

	t.Run("GET 正常请求", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://localhost:8080")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 正常请求应该返回 200
		assert.Equal(t, http.StatusOK, w.Code, "GET 请求应该返回 200")

		// 验证 CORS 响应头
		assert.Equal(t, "http://localhost:8080", w.Header().Get("Access-Control-Allow-Origin"), "应该设置允许的 Origin")
		assert.Equal(t, "Authorization, Content-Type, x-requested-with", w.Header().Get("Access-Control-Allow-Headers"), "应该设置允许的 Headers")
		assert.Equal(t, "POST, GET, OPTIONS, PUT, DELETE", w.Header().Get("Access-Control-Allow-Methods"), "应该设置允许的 Methods")
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"), "应该允许携带凭据")
		assert.Equal(t, "Origin", w.Header().Get("Vary"), "应该设置 Vary 头")

		// 验证响应体
		assert.Contains(t, w.Body.String(), "success", "响应应该包含 success")
	})

	t.Run("POST 正常请求", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/test", nil)
		req.Header.Set("Origin", "https://example.com")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 正常请求应该返回 200
		assert.Equal(t, http.StatusOK, w.Code, "POST 请求应该返回 200")

		// 验证 CORS 响应头（应该返回请求的 Origin）
		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"), "应该设置请求的 Origin")
	})

	t.Run("无 Origin 请求头", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/test", nil)
		// 不设置 Origin

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 正常请求应该返回 200
		assert.Equal(t, http.StatusOK, w.Code, "无 Origin 的请求应该返回 200")

		// 验证 CORS 响应头（Origin 为空字符串）
		assert.Equal(t, "", w.Header().Get("Access-Control-Allow-Origin"), "Origin 应该是空字符串")
	})

	t.Run("不同的 HTTP 方法", func(t *testing.T) {
		methods := []string{"PUT", "DELETE"}

		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				req, _ := http.NewRequest(method, "/test", nil)
				req.Header.Set("Origin", "http://localhost:8080")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// 正常请求应该返回 200
				assert.Equal(t, http.StatusOK, w.Code, method+" 请求应该返回 200")

				// 验证 CORS 响应头
				assert.Equal(t, "http://localhost:8080", w.Header().Get("Access-Control-Allow-Origin"), "应该设置允许的 Origin")
			})
		}
	})
}
