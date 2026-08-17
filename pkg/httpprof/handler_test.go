package httpprof

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandlerServesPprofIndex(t *testing.T) {
	resp := httptest.NewRecorder()

	Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "Types of profiles available")
}

func TestRegisterServesPprofGoroutineProfile(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)
	resp := httptest.NewRecorder()

	mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil))

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "goroutine profile")
}
