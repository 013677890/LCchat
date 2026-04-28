package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayTimeoutMiddlewareSkipsHealthAndMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(gatewayTimeoutMiddleware())
	r.GET("/health", func(c *gin.Context) {
		_, ok := c.Request.Context().Deadline()
		assert.False(t, ok)
		c.Status(http.StatusOK)
	})
	r.GET("/metrics", func(c *gin.Context) {
		_, ok := c.Request.Context().Deadline()
		assert.False(t, ok)
		c.Status(http.StatusOK)
	})

	for _, path := range []string{"/health", "/metrics"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}
}

func TestGatewayTimeoutMiddlewareAppliesDefaultBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(gatewayTimeoutMiddleware())
	r.GET("/api/v1/public/ping", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		require.True(t, ok)
		remaining := time.Until(deadline)
		assert.Greater(t, remaining, time.Duration(0))
		assert.LessOrEqual(t, remaining, gatewayDefaultRequestTimeout)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/ping", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestGatewayTimeoutMiddlewareConfiguredOnInitRouter(t *testing.T) {
	initRouterAuthTestLogger()

	var deadlineSeen bool
	r := buildAuthTestRouter(&fakeRouterAuthService{
		loginFn: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
			_, deadlineSeen = ctx.Deadline()
			require.Equal(t, "a", req.Account)
			require.Equal(t, "d1", deviceID)
			return &dto.LoginResponse{}, nil
		},
	})

	req := newRouterJSONRequest(t, http.MethodPost, "/api/v1/public/user/login", `{"account":"a","password":"pass123"}`)
	req.Header.Set("X-Device-ID", "d1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, deadlineSeen)
}
