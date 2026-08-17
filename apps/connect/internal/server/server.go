package server

import (
	"context"
	"fmt"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/middleware"
	"github.com/013677890/LCchat-Backend/pkg/httpprof"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// Config 定义 connect HTTP 服务的运行参数。
// 这些超时用于限制异常连接占用资源，避免慢连接拖垮服务。
type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// DefaultConfig 返回 connect 服务的默认配置。
// 端口优先读取 CONNECT_ADDR，未设置时默认监听 :8081。
func DefaultConfig() Config {
	addr := os.Getenv("CONNECT_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	return Config{
		Addr:              addr,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Server 对 http.Server 的轻量封装。
// 这里集中管理启动和优雅关闭，避免调用方直接操作底层对象。
type Server struct {
	httpServer *http.Server
}

// New 构建 Gin 路由并包装成 HTTP Server。
// 路由职责：
// - GET /health:   健康检查，返回在线连接数，供容器/探针调用。
// - GET /metrics:  暴露 Prometheus 文本格式指标（online_connections gauge）。
// - GET /ws:       WebSocket 接入入口。
// - GET /debug/pprof/: Go 运行时性能分析端点。
func New(cfg Config, wsHandler *handler.WSHandler, connManager *manager.ConnectionManager) *Server {
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)

	r := gin.New()
	r.Use(util.TraceLogger())
	r.Use(middleware.ClientIPMiddleware())
	r.Use(middleware.GinLogger())
	r.Use(middleware.RecoverMiddleware(true))
	r.Use(middleware.CORSMiddleware(middleware.DefaultCORSConfig()))

	wsRateLimitCfg := middleware.DefaultWSHandshakeRateLimitConfig()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":             "ok",
			"online_connections": connManager.Count(),
		})
	})

	r.GET("/metrics", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.String(http.StatusOK,
			fmt.Sprintf("# HELP connect_online_connections Current number of active WebSocket connections.\n"+
				"# TYPE connect_online_connections gauge\n"+
				"connect_online_connections %d\n", connManager.Count()))
	})
	profilerHandler := httpprof.Handler()
	r.Any("/debug/pprof", gin.WrapH(profilerHandler))
	r.Any("/debug/pprof/*path", gin.WrapH(profilerHandler))

	r.GET("/ws", middleware.WSHandshakeRateLimitMiddleware(wsRateLimitCfg), wsHandler.ServeWS)

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

// Start 启动 HTTP 监听。
// 正常优雅关闭时会返回 http.ErrServerClosed，调用方应将其视为正常退出。
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown 执行优雅停机。
// 调用方需要传入带超时的 ctx，以防止无限等待。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr 返回 HTTP 监听地址，便于启动日志复用。
func (s *Server) Addr() string {
	if s == nil || s.httpServer == nil {
		return ""
	}
	return s.httpServer.Addr
}
