package router

import (
	"ChatServer/apps/gateway/internal/middleware"
	v1 "ChatServer/apps/gateway/internal/router/v1"
	"ChatServer/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InitRouter 初始化路由
// loginHandler: 登录处理器（依赖注入）
func InitRouter(authHandler *v1.AuthHandler) *gin.Engine {
	r := gin.New()

	// 恢复中间件
	r.Use(middleware.GinRecovery(true))

	// 追踪中间件 (生成 trace_id)
	r.Use(util.TraceLogger())

	// 客户端 IP 中间件
	r.Use(middleware.ClientIPMiddleware())

	// 日志中间件
	r.Use(middleware.GinLogger())


	// Prometheus 监控中间件
	r.Use(middleware.PrometheusMiddleware())

	// 跨域中间件
	r.Use(middleware.CorsMiddleware())

	// 健康检查（无需认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// Prometheus 指标暴露接口
	// Prometheus 会定时访问这个接口来拉取监控数据
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API 路由组
	api := r.Group("/api/v1")
	{
		// 公开接口（不需要认证）
		public := api.Group("/public")
		{
			//转发给user服务的接口
			user := public.Group("/user")
			{
				user.POST("/login", authHandler.Login)
				user.POST("/register", authHandler.Register)
				user.POST("/send-verify-code", authHandler.SendVerifyCode)
			}
		}

		// 需要认证的接口
		_ = api.Group("/auth")
		//auth.Use(middleware.JWTAuthMiddleware()) // 应用 JWT 认证中间件  测试环境下不启用
		// TODO: 添加需要认证的接口
	}

	return r
}
