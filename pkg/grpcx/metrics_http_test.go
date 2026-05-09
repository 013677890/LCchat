package grpcx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsHTTPServerRegistersMetricsRoute(t *testing.T) {
	metrics := NewMetrics()
	metrics.requestTotal.WithLabelValues("/user.UserService/GetProfile", "OK").Inc()

	server := NewMetricsHTTPServer(":9190", metrics)
	require.NotNil(t, server)
	require.NotNil(t, server.Handler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp := httptest.NewRecorder()

	server.Handler.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "/user.UserService/GetProfile")
	assert.Contains(t, resp.Body.String(), "grpc_request_total")
}

func TestNewMetricsHTTPServerHandlesNilMetrics(t *testing.T) {
	server := NewMetricsHTTPServer(":9190", nil)
	require.NotNil(t, server)
	require.NotNil(t, server.Handler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp := httptest.NewRecorder()

	server.Handler.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}
