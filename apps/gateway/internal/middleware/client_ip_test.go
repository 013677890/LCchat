package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestParseTrustedProxies(t *testing.T) {
	assert.Nil(t, parseTrustedProxies(""))
	assert.Nil(t, parseTrustedProxies("   "))
	assert.Equal(t, []string{"10.0.0.0/8"}, parseTrustedProxies("10.0.0.0/8"))
	assert.Equal(t,
		[]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.1.1"},
		parseTrustedProxies(" 10.0.0.0/8 , 172.16.0.0/12 ,, 192.168.1.1 "),
	)
}

func newClientIPContext(remoteAddr string, configure func(*gin.Engine)) *gin.Context {
	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)
	if configure != nil {
		configure(engine)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("X-Real-IP", "9.9.9.9")       // 伪造的来源 IP
	req.Header.Set("X-Forwarded-For", "8.8.8.8") // 伪造的转发链
	c.Request = req
	return c
}

func TestGetClientIP_IgnoresSpoofedHeadersWithoutTrustedProxy(t *testing.T) {
	// 未配置可信代理：伪造的 X-Real-IP / X-Forwarded-For 必须被忽略，使用直连地址。
	c := newClientIPContext("203.0.113.7:54321", func(e *gin.Engine) {
		t.Setenv("GATEWAY_TRUSTED_PROXIES", "")
		ConfigureTrustedProxies(e)
	})
	assert.Equal(t, "203.0.113.7", GetClientIP(c))
}

func TestGetClientIP_HonorsHeaderFromTrustedProxy(t *testing.T) {
	// 直连 peer 属于可信代理时，按 X-Real-IP 解析真实客户端。
	c := newClientIPContext("203.0.113.7:54321", func(e *gin.Engine) {
		t.Setenv("GATEWAY_TRUSTED_PROXIES", "203.0.113.7")
		ConfigureTrustedProxies(e)
	})
	assert.Equal(t, "9.9.9.9", GetClientIP(c))
}
