package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// IsPlatformPath 判断是否为平台级接口路径。
func IsPlatformPath(path string) bool {
	switch path {
	case "/health", "/metrics":
		return true
	default:
		return false
	}
}

// ShouldBypassGlobalGuards 判断是否应跳过全局保护型中间件。
func ShouldBypassGlobalGuards(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.Method == http.MethodOptions {
		return true
	}
	return IsPlatformPath(c.Request.URL.Path)
}
