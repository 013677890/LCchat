package router

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	v1 "github.com/013677890/LCchat-Backend/apps/gateway/internal/router/v1"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"time"
)

const gatewayDefaultRequestTimeout = 2 * time.Second

// gatewayRequestTimeouts 按接口语义显式配置预算。
// 第一批先把 gateway 现有接口都列出来，范围先收敛在 1-3s：
//   - 1s: 轻量读、探活类
//   - 2s: 常规写 / 常规聚合
//   - 3s: 搜索、批量、发送、上传等相对更重的接口
//
// 未命中的新接口仍会回退到 gatewayDefaultRequestTimeout，避免漏配后完全失去保护。
var gatewayRequestTimeouts = map[string]time.Duration{
	"/health":  0,
	"/metrics": 0,
	// public user
	// 认证链路依赖 bcrypt、邮件发送和数据库事务，预算需要比普通读接口更宽松。
	"/api/v1/public/user/login":            3 * time.Second,
	"/api/v1/public/user/login-by-code":    3 * time.Second,
	"/api/v1/public/user/register":         5 * time.Second,
	"/api/v1/public/user/send-verify-code": 5 * time.Second,
	"/api/v1/public/user/reset-password":   5 * time.Second,
	"/api/v1/public/user/refresh-token":    2 * time.Second,
	"/api/v1/public/user/verify-code":      2 * time.Second,
	"/api/v1/public/user/parse-qrcode":     1 * time.Second,
	// auth user
	// 更新个人资料需要经过 gRPC、事务和展示字段事件，预算要比读取更宽。
	"/api/v1/auth/user/profile":                 5 * time.Second,
	"/api/v1/auth/user/profile/:userUuid":       2 * time.Second,
	"/api/v1/auth/user/search":                  3 * time.Second,
	"/api/v1/auth/user/avatar":                  3 * time.Second,
	"/api/v1/auth/user/qrcode":                  1 * time.Second,
	"/api/v1/auth/user/batch-profile":           3 * time.Second,
	"/api/v1/auth/user/devices":                 1 * time.Second,
	"/api/v1/auth/user/devices/:deviceId":       2 * time.Second,
	"/api/v1/auth/user/online-status/:userUuid": 1 * time.Second,
	"/api/v1/auth/user/batch-online-status":     2 * time.Second,
	"/api/v1/auth/user/change-password":         5 * time.Second,
	"/api/v1/auth/user/change-email":            3 * time.Second,
	"/api/v1/auth/user/delete-account":          5 * time.Second,
	"/api/v1/auth/user/logout":                  1 * time.Second,
	// auth friend
	"/api/v1/auth/friend/apply":        2 * time.Second,
	"/api/v1/auth/friend/apply-list":   1 * time.Second,
	"/api/v1/auth/friend/apply/sent":   1 * time.Second,
	"/api/v1/auth/friend/apply/handle": 2 * time.Second,
	"/api/v1/auth/friend/apply/unread": 1 * time.Second,
	"/api/v1/auth/friend/apply/read":   1 * time.Second,
	"/api/v1/auth/friend/list":         2 * time.Second,
	"/api/v1/auth/friend/sync":         2 * time.Second,
	"/api/v1/auth/friend/delete":       2 * time.Second,
	"/api/v1/auth/friend/remark":       2 * time.Second,
	"/api/v1/auth/friend/tag":          2 * time.Second,
	"/api/v1/auth/friend/check":        1 * time.Second,
	"/api/v1/auth/friend/relation":     1 * time.Second,
	// auth blacklist
	"/api/v1/auth/blacklist":           2 * time.Second,
	"/api/v1/auth/blacklist/check":     1 * time.Second,
	"/api/v1/auth/blacklist/:userUuid": 2 * time.Second,
	// auth messages
	"/api/v1/auth/messages/send":       3 * time.Second,
	"/api/v1/auth/messages/pull":       2 * time.Second,
	"/api/v1/auth/messages/get-by-ids": 2 * time.Second,
	"/api/v1/auth/messages/recall":     2 * time.Second,
	// auth conversations
	"/api/v1/auth/conversations":           2 * time.Second,
	"/api/v1/auth/conversations/mark-read": 2 * time.Second,
	"/api/v1/auth/conversations/:convId":   2 * time.Second,
	"/api/v1/auth/conversations/settings":  2 * time.Second,
	// auth groups
	"/api/v1/auth/groups":                                          2 * time.Second,
	"/api/v1/auth/groups/search":                                   3 * time.Second,
	"/api/v1/auth/groups/:groupUuid":                               2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/notice":                        2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/apply":                         2 * time.Second,
	"/api/v1/auth/groups/join-applications":                        1 * time.Second,
	"/api/v1/auth/groups/:groupUuid/my-join-application":           1 * time.Second,
	"/api/v1/auth/groups/:groupUuid/join-requests":                 2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/join-requests/pending-count":   1 * time.Second,
	"/api/v1/auth/groups/:groupUuid/join-requests/reviewed":        2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/join-requests/:applyId/review": 2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/transfer-owner":                2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/leave":                         2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/my-nickname":                   2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/mute-setting":                  2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/members":                       2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/members/search":                3 * time.Second,
	"/api/v1/auth/groups/:groupUuid/members/:userUuid":             2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/members/:userUuid/nickname":    2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/members/:userUuid/mute":        2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/members/:userUuid/role":        2 * time.Second,
	"/api/v1/auth/groups/:groupUuid/member-ids":                    1 * time.Second,
}

func gatewayTimeoutMiddleware() gin.HandlerFunc {
	return middleware.TimeoutMiddlewareWithPath(gatewayRequestTimeouts, gatewayDefaultRequestTimeout)
}

// InitRouter 初始化路由
// authHandler: 认证处理器（依赖注入）
// userHandler: 用户信息处理器（依赖注入）
// friendHandler: 好友处理器（依赖注入）
// blacklistHandler: 黑名单处理器（依赖注入）
// deviceHandler: 设备处理器（依赖注入）
// msgHandler: 消息处理器（依赖注入）
func InitRouter(authHandler *v1.AuthHandler, userHandler *v1.UserHandler, friendHandler *v1.FriendHandler, blacklistHandler *v1.BlacklistHandler, deviceHandler *v1.DeviceHandler, msgHandler *v1.MsgHandler, groupHandlers ...*v1.GroupHandler) *gin.Engine {
	var groupHandler *v1.GroupHandler
	if len(groupHandlers) > 0 {
		groupHandler = groupHandlers[0]
	}
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
	// 全局请求级超时控制：探活/指标接口跳过，其余路由统一纳入 timeout budget。
	// 放在日志/监控中间件之后，确保超时兜底写回的最终响应能被完整观测到；
	// 同时放在业务 handler 之前，保证后续所有下游调用都能继承同一个请求级 deadline。
	r.Use(gatewayTimeoutMiddleware())
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
				friend.POST("/check", friendHandler.CheckIsFriend)
				friend.POST("/relation", friendHandler.GetRelationStatus)
			}
			blacklist := auth.Group("/blacklist")
			{
				blacklist.POST("", blacklistHandler.AddBlacklist)
				blacklist.GET("", blacklistHandler.GetBlacklistList)
				blacklist.DELETE("/:userUuid", blacklistHandler.RemoveBlacklist)
				// 兼容现有客户端与测试约定，暂时继续对外暴露 /check。
				// 后续若要收敛越权风险，需要改成仅允许判断“当前登录用户 -> target”的关系。
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
			if groupHandler != nil {
				groups := auth.Group("/groups")
				{
					groups.POST("", groupHandler.CreateGroup)
					groups.GET("", groupHandler.GetGroupList)
					// /search 必须放在 /:groupUuid 之前，避免被动态路由吞掉。
					groups.GET("/search", groupHandler.SearchGroups)
					groups.GET("/join-applications", groupHandler.ListMyJoinGroupApplications)
					groups.GET("/:groupUuid", groupHandler.GetGroupInfo)
					groups.PATCH("/:groupUuid", groupHandler.UpdateGroupInfo)
					groups.PUT("/:groupUuid/notice", groupHandler.UpdateGroupNotice)
					groups.POST("/:groupUuid/apply", groupHandler.ApplyJoinGroup)
					groups.DELETE("/:groupUuid/apply", groupHandler.CancelJoinGroupApplication)
					groups.GET("/:groupUuid/my-join-application", groupHandler.GetMyJoinGroupApplication)
					groups.GET("/:groupUuid/join-requests", groupHandler.ListJoinRequests)
					groups.GET("/:groupUuid/join-requests/pending-count", groupHandler.GetJoinRequestPendingCount)
					groups.GET("/:groupUuid/join-requests/reviewed", groupHandler.ListReviewedJoinRequests)
					groups.POST("/:groupUuid/join-requests/:applyId/review", groupHandler.ReviewJoinGroup)
					groups.POST("/:groupUuid/transfer-owner", groupHandler.TransferGroupOwner)
					groups.POST("/:groupUuid/leave", groupHandler.LeaveGroup)
					groups.PATCH("/:groupUuid/my-nickname", groupHandler.UpdateMyGroupNickname)
					groups.PATCH("/:groupUuid/mute-setting", groupHandler.UpdateGroupMuteSetting)
					groups.DELETE("/:groupUuid", groupHandler.DismissGroup)
					groups.POST("/:groupUuid/members", groupHandler.AddMember)
					groups.GET("/:groupUuid/members/search", groupHandler.SearchGroupMembers)
					groups.GET("/:groupUuid/members", groupHandler.GetMemberList)
					groups.DELETE("/:groupUuid/members/:userUuid", groupHandler.RemoveMember)
					groups.PATCH("/:groupUuid/members/:userUuid/nickname", groupHandler.UpdateGroupMemberNickname)
					groups.PATCH("/:groupUuid/members/:userUuid/mute", groupHandler.MuteGroupMember)
					groups.PATCH("/:groupUuid/members/:userUuid/role", groupHandler.UpdateMemberRole)
					groups.GET("/:groupUuid/member-ids", groupHandler.GetGroupMemberIDs)
				}
			}
		}
	}
	return r
}
