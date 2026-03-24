package router

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	v1 "github.com/013677890/LCchat-Backend/apps/gateway/internal/router/v1"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InitRouter 初始化路由
// authHandler: 认证处理器（依赖注入）
// userHandler: 用户信息处理器（依赖注入）
// friendHandler: 好友处理器（依赖注入）
// blacklistHandler: 黑名单处理器（依赖注入）
// deviceHandler: 设备处理器（依赖注入）
// msgHandler: 消息处理器（依赖注入）
func InitRouter(authHandler *v1.AuthHandler, userHandler *v1.UserHandler, friendHandler *v1.FriendHandler, blacklistHandler *v1.BlacklistHandler, deviceHandler *v1.DeviceHandler, msgHandler *v1.MsgHandler) *gin.Engine {
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

	// ==================== 全局 IP 限流中间件 ====================
	// 参数说明：
	//   - blacklistKey: gateway:blacklist:ips (黑名单 Redis Set 的 key)
	//   - rate: 10.0 (每秒10个令牌)
	//   - burst: 20 (令牌桶容量，允许突发请求)
	// 功能：
	//   1. 检查 IP 是否在黑名单中，在则返回 403
	//   2. 执行令牌桶限流，超过则返回 429
	//   3. Redis 不可用时降级放行（Fail-Open），不影响服务可用性
	r.Use(middleware.IPRateLimitMiddleware(rediskey.GatewayIPBlacklistKey(), 10.0, 20))

	// 健康检查（无需认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// Prometheus 指标暴露接口
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API 路由组
	api := r.Group("/api/v1")
	{
		// 公开接口（不需要认证）
		public := api.Group("/public")
		{
			user := public.Group("/user")
			{
				user.POST("/login", authHandler.Login)
				user.POST("/login-by-code", authHandler.LoginByCode)
				user.POST("/register", authHandler.Register)
				user.POST("/send-verify-code", authHandler.SendVerifyCode)
				user.POST("/reset-password", authHandler.ResetPassword)
				user.POST("/refresh-token", authHandler.RefreshToken)
				user.POST("/verify-code", authHandler.VerifyCode)
				user.POST("/parse-qrcode", userHandler.ParseQRCode)
			}
		}

		// 需要认证的接口
		auth := api.Group("/auth")
		auth.Use(middleware.JWTAuthMiddleware()) // JWT 认证中间件（必须在前）

		// ==================== 用户级别限流中间件 ====================
		// 只对已认证的用户进行限流
		// 参数说明：
		//   - rate: 100.0 (每秒100个令牌，对正常用户比较宽松)
		//   - burst: 200 (令牌桶容量200)
		auth.Use(middleware.UserRateLimitMiddleware(100.0, 200))
		{
			user := auth.Group("/user")
			{
				user.GET("/profile", userHandler.GetProfile)
				user.PUT("/profile", userHandler.UpdateProfile)
				user.GET("/profile/:userUuid", userHandler.GetOtherProfile)
				user.GET("/search", userHandler.SearchUser)
				user.POST("/avatar", userHandler.UploadAvatar)
				user.GET("/qrcode", userHandler.GetQRCode)
				user.POST("/batch-profile", userHandler.BatchGetProfile)
				user.GET("/devices", deviceHandler.GetDeviceList)
				user.DELETE("/devices/:deviceId", deviceHandler.KickDevice)
				user.GET("/online-status/:userUuid", deviceHandler.GetOnlineStatus)
				user.POST("/batch-online-status", deviceHandler.BatchGetOnlineStatus)

				// 敏感操作使用更严格的限流
				user.POST("/change-password",
					middleware.UserRateLimitMiddlewareWithConfig(2.0, 5),
					userHandler.ChangePassword)
				user.POST("/change-email",
					middleware.UserRateLimitMiddlewareWithConfig(2.0, 5),
					userHandler.ChangeEmail)
				user.POST("/delete-account",
					middleware.UserRateLimitMiddlewareWithConfig(2.0, 5),
					userHandler.DeleteAccount)

				user.POST("/logout", authHandler.Logout)
			}
			friend := auth.Group("/friend")
			{
				friend.POST("/apply", friendHandler.SendFriendApply)
				friend.GET("/apply-list", friendHandler.GetFriendApplyList)
				friend.GET("/apply/sent", friendHandler.GetSentApplyList)
				friend.POST("/apply/handle", friendHandler.HandleFriendApply)
				friend.GET("/apply/unread", friendHandler.GetUnreadApplyCount)
				friend.POST("/apply/read", friendHandler.MarkApplyAsRead)
				friend.GET("/list", friendHandler.GetFriendList)
				friend.POST("/sync", friendHandler.SyncFriendList)
				friend.POST("/delete", friendHandler.DeleteFriend)
				friend.POST("/remark", friendHandler.SetFriendRemark)
				friend.POST("/tag", friendHandler.SetFriendTag)
				friend.GET("/tags", friendHandler.GetTagList)
				friend.POST("/check", friendHandler.CheckIsFriend)
				friend.POST("/relation", friendHandler.GetRelationStatus)
			}
			blacklist := auth.Group("/blacklist")
			{
				blacklist.POST("", blacklistHandler.AddBlacklist)
				blacklist.GET("", blacklistHandler.GetBlacklistList)
				blacklist.DELETE("/:userUuid", blacklistHandler.RemoveBlacklist)
				blacklist.POST("/check", blacklistHandler.CheckIsBlacklist)
			}
			messages := auth.Group("/messages")
			{
				messages.POST("/send", msgHandler.SendMessage)
				messages.GET("/pull", msgHandler.PullMessages)
				messages.POST("/get-by-ids", msgHandler.GetMessagesByIds)
				messages.POST("/recall", msgHandler.RecallMessage)
			}
			conversations := auth.Group("/conversations")
			{
				conversations.GET("", msgHandler.GetConversations)
				conversations.POST("/mark-read", msgHandler.MarkRead)
				conversations.DELETE("/:convId", msgHandler.DeleteConversation)
				conversations.PATCH("/settings", msgHandler.UpdateConversationSettings)
			}
		}
	}

	return r
}
