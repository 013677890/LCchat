package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/gin-gonic/gin"
)

func TestTraceLoggerSetsTraceAndSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(TraceLogger())
	router.GET("/ping", func(c *gin.Context) {
		ctx := c.Request.Context()
		if got := ctxmeta.TraceID(ctx); got != "trace-1" {
			t.Fatalf("request context trace_id = %q", got)
		}
		if got := ctxmeta.ParentSpanID(ctx); got != "span-parent" {
			t.Fatalf("request context parent_span_id = %q", got)
		}
		if got := ctxmeta.SpanID(ctx); got == "" {
			t.Fatal("request context span_id is empty")
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(ctxmeta.HeaderRequestID, " trace-1 ")
	req.Header.Set(ctxmeta.HeaderParentSpanID, " span-parent ")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d", resp.Code)
	}
	if got := resp.Header().Get(ctxmeta.HeaderRequestID); got != "trace-1" {
		t.Fatalf("response trace header = %q", got)
	}
	if got := resp.Header().Get(ctxmeta.HeaderSpanID); got == "" {
		t.Fatal("response span header is empty")
	}
	if got := resp.Header().Get(ctxmeta.HeaderParentSpanID); got != "span-parent" {
		t.Fatalf("response parent span header = %q", got)
	}
}
