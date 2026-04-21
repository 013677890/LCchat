package server

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config 定义 message-push HTTP server 的运行参数。
type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// DefaultConfig 返回 message-push metrics server 默认配置。
func DefaultConfig() Config {
	addr := os.Getenv("MESSAGE_PUSH_HTTP_ADDR")
	if addr == "" {
		addr = ":8084"
	}
	return Config{
		Addr:              addr,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}

// Server 封装 message-push 的 HTTP 指标服务。
type Server struct {
	httpServer *http.Server
}

// New 构建只暴露 health/metrics 的轻量 HTTP server。
func New(cfg Config) *Server {
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)

	r := gin.New()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.Addr,
			Handler:           r,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}
}

// Start 启动 HTTP 服务。
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭 HTTP 服务。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr 返回监听地址。
func (s *Server) Addr() string {
	if s == nil || s.httpServer == nil {
		return ""
	}
	return s.httpServer.Addr
}
