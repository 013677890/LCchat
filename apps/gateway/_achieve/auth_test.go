package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"ChatServer/pkg/logger"
	"ChatServer/pkg/util"
)

// init 初始化 logger（测试模式，不输出日志）
func init() {
	testLogger := zap.NewNop()
	logger.ReplaceGlobal(testLogger)
}

// generateTestToken 生成测试用的 Token
func generateTestToken() string {
	token, err := util.GenerateToken("test-user-uuid", "test-device-id")
	if err != nil {
		panic(err)
	}
	return token
}

// TestJWTAuthMiddleware_NoAuthorization 测试未提供 Authorization header
func TestJWTAuthMiddleware_NoAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := JWTAuthMiddleware()

	// 创建一个空的 Handler
	dummyHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", dummyHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "应该返回 401 未认证")
	assert.Contains(t, w.Body.String(), "未提供认证信息", "响应应该包含错误消息")
}

// TestJWTAuthMiddleware_InvalidFormat 测试 Authorization header 格式错误
func TestJWTAuthMiddleware_InvalidFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := JWTAuthMiddleware()

	dummyHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", dummyHandler)

	tests := []struct {
		name   string
		header string
	}{
		{
			name:   "没有 Bearer 前缀",
			header: "invalid-token",
		},
		{
			name:   "使用错误的 Scheme",
			header: "Basic invalid-token",
		},
		{
			name:   "只有 Bearer 前缀",
			header: "Bearer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code, "应该返回 401 未认证")
			assert.Contains(t, w.Body.String(), "认证格式错误", "响应应该包含格式错误消息")
		})
	}
}

// TestJWTAuthMiddleware_InvalidToken 测试无效 Token
func TestJWTAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := JWTAuthMiddleware()

	dummyHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", dummyHandler)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "完全无效的 token",
			token: "invalid.jwt.token",
		},
		{
			name:  "签名错误的 token",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX3V1aWQiOiJ0ZXN0LXVzZXIiLCJkZXZpY2VfaWQiOiJ0ZXN0LWRldmljZSIsImV4cCI6MTY3MjUzMDQwMCwiaWF0IjoxNjcyNTI3MjAwLCJuYmYiOjE2NzI1MjcyMDAsImlzcyI6IkNoYXRTZXJ2ZXItR2F0ZXdheSJ9.invalid-signature",
		},
		{
			name:  "过期后的 token",
			token: generateExpiredToken(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code, "应该返回 401 未认证")
			assert.Contains(t, w.Body.String(), "Token 无效或已过期", "响应应该包含 Token 无效消息")
		})
	}
}

// generateExpiredToken 生成过期的 Token
func generateExpiredToken() string {
	now := time.Now().Add(-1 * time.Hour) // 1 小时前过期
	claims := &util.CustomClaims{
		UserUUID: "test-user-uuid",
		DeviceID: "test-device-id",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "ChatServer-Gateway",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(util.JWTSecret))
	if err != nil {
		panic(err)
	}
	return tokenString
}

// TestJWTAuthMiddleware_Success 测试 Token 验证成功
func TestJWTAuthMiddleware_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := JWTAuthMiddleware()

	// 验证 user_uuid 和 device_id 是否正确设置
	successHandler := func(c *gin.Context) {
		userUUID, exists := GetUserUUID(c)
		assert.True(t, exists, "user_uuid 应该存在")
		assert.Equal(t, "test-user-uuid", userUUID, "user_uuid 应该匹配")

		deviceID, exists := GetDeviceID(c)
		assert.True(t, exists, "device_id 应该存在")
		assert.Equal(t, "test-device-id", deviceID, "device_id 应该匹配")

		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", successHandler)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "应该返回 200 成功")
}

// TestGetUserUUID 测试 GetUserUUID 辅助函数
func TestGetUserUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		setup  func(c *gin.Context)
		want   string
		wantOk bool
	}{
		{
			name: "获取已设置的 user_uuid",
			setup: func(c *gin.Context) {
				c.Set("user_uuid", "test-user-123")
			},
			want:   "test-user-123",
			wantOk: true,
		},
		{
			name:   "user_uuid 不存在",
			setup:  func(c *gin.Context) {},
			want:   "",
			wantOk: false,
		},
		{
			name: "user_uuid 类型错误",
			setup: func(c *gin.Context) {
				c.Set("user_uuid", 12345)
			},
			want:   "",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			tt.setup(c)

			uuid, ok := GetUserUUID(c)
			assert.Equal(t, tt.want, uuid, "user_uuid 不匹配")
			assert.Equal(t, tt.wantOk, ok, "ok 不匹配")
		})
	}
}

// TestGetDeviceID 测试 GetDeviceID 辅助函数
func TestGetDeviceID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		setup  func(c *gin.Context)
		want   string
		wantOk bool
	}{
		{
			name: "获取已设置的 device_id",
			setup: func(c *gin.Context) {
				c.Set("device_id", "test-device-123")
			},
			want:   "test-device-123",
			wantOk: true,
		},
		{
			name:   "device_id 不存在",
			setup:  func(c *gin.Context) {},
			want:   "",
			wantOk: false,
		},
		{
			name: "device_id 类型错误",
			setup: func(c *gin.Context) {
				c.Set("device_id", 12345)
			},
			want:   "",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			tt.setup(c)

			deviceID, ok := GetDeviceID(c)
			assert.Equal(t, tt.want, deviceID, "device_id 不匹配")
			assert.Equal(t, tt.wantOk, ok, "ok 不匹配")
		})
	}
}
