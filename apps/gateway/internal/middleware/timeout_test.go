package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutMiddlewareWithPathUsesFullPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(TimeoutMiddlewareWithPath(map[string]time.Duration{
		"/profile/:userUuid": 120 * time.Millisecond,
	}, time.Second))

	var remaining time.Duration
	r.GET("/profile/:userUuid", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		require.True(t, ok)
		remaining = time.Until(deadline)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/profile/123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Greater(t, remaining, time.Duration(0))
	assert.LessOrEqual(t, remaining, 300*time.Millisecond)
}

func TestNewRequestTimeoutContextKeepsShorterParentDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer parentCancel()

	ctx, cancel, effectiveTimeout := newRequestTimeoutContext(parent, time.Second)
	defer cancel()

	parentDeadline, parentOK := parent.Deadline()
	childDeadline, childOK := ctx.Deadline()
	require.True(t, parentOK)
	require.True(t, childOK)

	assert.WithinDuration(t, parentDeadline, childDeadline, 5*time.Millisecond)
	assert.Greater(t, effectiveTimeout, time.Duration(0))
	assert.LessOrEqual(t, effectiveTimeout, 80*time.Millisecond)
}