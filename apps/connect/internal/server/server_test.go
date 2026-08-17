package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistersPprofRoutes(t *testing.T) {
	server := New(DefaultConfig(), nil, nil)
	require.NotNil(t, server.httpServer)

	resp := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "Types of profiles available")
}
