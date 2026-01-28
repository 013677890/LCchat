package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestUserRateLimiter_GetLimiter 测试获取和创建限流器
func TestUserRateLimiter_GetLimiter(t *testing.T) {
	limiter := NewUserRateLimiter(10, 5)

	// 测试创建新用户的限流器
	l1 := limiter.GetLimiter("user1")
	assert.NotNil(t, l1, "限流器不应该为 nil")
	assert.Equal(t, 1, limiter.GetLimiterCount(), "应该有 1 个限流器")

	// 测试获取已存在的限流器
	l2 := limiter.GetLimiter("user1")
	assert.Same(t, l1, l2, "应该返回同一个限流器实例")
	assert.Equal(t, 1, limiter.GetLimiterCount(), "仍然应该只有 1 个限流器")

	// 测试创建多个用户的限流器
	limiter.GetLimiter("user2")
	limiter.GetLimiter("user3")
	assert.Equal(t, 3, limiter.GetLimiterCount(), "应该有 3 个限流器")
}

// TestUserRateLimiter_CleanupInactiveLimiters 测试清理不活跃的限流器
func TestUserRateLimiter_CleanupInactiveLimiters(t *testing.T) {
	limiter := NewUserRateLimiter(10, 5)

	// 创建 3 个用户的限流器
	limiter.GetLimiter("user1")
	limiter.GetLimiter("user2")
	limiter.GetLimiter("user3")
	assert.Equal(t, 3, limiter.GetLimiterCount(), "应该有 3 个限流器")

	// 清理不活跃的限流器
	// 注意：新创建的限流器令牌桶是满的，所以会被清理
	limiter.CleanupInactiveLimiters(30 * time.Minute)
	assert.Equal(t, 0, limiter.GetLimiterCount(), "所有限流器应该被清理")
}

// TestUserRateLimiter_Allow 测试限流器允许请求
func TestUserRateLimiter_Allow(t *testing.T) {
	limiter := NewUserRateLimiter(100, 10) // 每秒 100 个请求，桶容量 10

	// 获取用户限流器
	userLimiter := limiter.GetLimiter("user1")

	// 连续发送 10 个请求（正好等于桶容量）
	for i := 0; i < 10; i++ {
		assert.True(t, userLimiter.Allow(), "第 %d 个请求应该被允许", i+1)
	}

	// 第 11 个请求应该被拒绝
	// 注意：由于令牌产生速率是 100/s，几乎是瞬时的，但桶容量是 10
	// 所以理论上可以超过 10 个，但为了测试限流效果，我们设置一个低速率
}

// TestUserRateLimiter_AllowLowRate 测试低速率限流
func TestUserRateLimiter_AllowLowRate(t *testing.T) {
	limiter := NewUserRateLimiter(1, 1) // 每秒 1 个请求，桶容量 1

	// 获取用户限流器
	userLimiter := limiter.GetLimiter("user1")

	// 第 1 个请求应该被允许
	assert.True(t, userLimiter.Allow(), "第 1 个请求应该被允许")

	// 第 2 个请求应该被拒绝（因为令牌桶已空）
	assert.False(t, userLimiter.Allow(), "第 2 个请求应该被拒绝")
}

// TestUserRateLimitMiddlewareWithConfig 测试限流中间件
func TestUserRateLimitMiddlewareWithConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建限流器：每秒 2 个请求，桶容量 2
	middleware := UserRateLimitMiddlewareWithConfig(2, 2)

	// 创建一个空的 Handler
	dummyHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	// 测试用例
	tests := []struct {
		name           string
		userUUID       string
		requestCount   int
		firstResponse  int
		lastResponse   int
	}{
		{
			name:           "正常限流（前 2 个请求成功，第 3 个被限流）",
			userUUID:       "user1",
			requestCount:   3,
			firstResponse:  http.StatusOK,
			lastResponse:   http.StatusTooManyRequests,
		},
		{
			name:           "不同用户互不影响",
			userUUID:       "user2",
			requestCount:   2,
			firstResponse:  http.StatusOK,
			lastResponse:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < tt.requestCount; i++ {
				req, _ := http.NewRequest("GET", "/test", nil)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = req
				c.Set("user_uuid", tt.userUUID) // 设置用户 UUID 到 gin.Context

				// 手动调用中间件和 Handler
				middleware(c)
				if !c.IsAborted() {
					dummyHandler(c)
				}

				expectedStatus := tt.firstResponse
				if i == tt.requestCount-1 && tt.requestCount > 2 {
					expectedStatus = tt.lastResponse
				}

				assert.Equal(t, expectedStatus, w.Code,
					"第 %d 个请求的响应状态码不匹配，期望 %d，实际 %d",
					i+1, expectedStatus, w.Code)
			}
		})
	}
}

// TestUserRateLimitMiddlewareWithConfig_NoUserUUID 测试无用户 UUID 的情况
func TestUserRateLimitMiddlewareWithConfig_NoUserUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := UserRateLimitMiddlewareWithConfig(2, 2)

	// 创建一个空的 Handler
	dummyHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	// 设置路由
	router := gin.New()
	router.Use(middleware)
	router.GET("/test", dummyHandler)

	// 发送不带用户 UUID 的请求
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该返回 401 未认证
	assert.Equal(t, http.StatusUnauthorized, w.Code, "应该返回 401 未认证")
}

// BenchmarkUserRateLimiter_GetLimiter 基准测试
func BenchmarkUserRateLimiter_GetLimiter(b *testing.B) {
	limiter := NewUserRateLimiter(1000, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.GetLimiter("user1")
	}
}

// BenchmarkUserRateLimiter_Allow 基准测试
func BenchmarkUserRateLimiter_Allow(b *testing.B) {
	limiter := NewUserRateLimiter(100000, 10000)
	userLimiter := limiter.GetLimiter("user1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userLimiter.Allow()
	}
}
