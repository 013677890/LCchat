package router

import (
	"ChatServer/apps/gateway/internal/middleware"
	"ChatServer/apps/gateway/internal/utils"
	"ChatServer/pkg/logger"
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// InitRouter 初始化路由
func InitRouter() *gin.Engine {
	r := gin.New()

	// 基础中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 跨域中间件
	r.Use(middleware.CorsMiddleware())

	// 健康检查（无需认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API 路由组
	api := r.Group("/api/v1")
	{
		// 公开接口（不需要认证）
		public := api.Group("/public")
		{
			// 示例：登录接口
			public.POST("/login", handleLogin)
			// 示例：刷新 Token 接口
			public.POST("/refresh", handleRefresh)
		}

		// 需要认证的接口
		auth := api.Group("/auth")
		auth.Use(middleware.JWTAuthMiddleware()) // 应用 JWT 认证中间件
		{
			// 示例：获取用户信息
			auth.GET("/user/info", handleGetUserInfo)
			// 示例：退出登录
			auth.POST("/logout", handleLogout)
		}
	}

	return r
}

// handleLogin 登录接口示例
func handleLogin(c *gin.Context) {
	ctx := context.Background()

	// TODO: 实际业务逻辑：验证用户名密码，查询数据库等
	// 这里只是示例
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		DeviceID string `json:"device_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn(ctx, "login request bind failed", zap.String("error", err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误",
		})
		return
	}

	// 模拟用户 UUID（实际应从数据库查询）
	userUUID := "mock-user-uuid-12345"

	// 生成 Token
	accessToken, err := utils.GenerateToken(userUUID, req.DeviceID)
	if err != nil {
		logger.Error(ctx, "generate access token failed", zap.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "生成 Token 失败",
		})
		return
	}

	refreshToken, err := utils.GenerateRefreshToken(userUUID, req.DeviceID)
	if err != nil {
		logger.Error(ctx, "generate refresh token failed", zap.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "生成 Refresh Token 失败",
		})
		return
	}

	logger.Info(ctx, "user login success",
		zap.String("user_uuid", userUUID),
		zap.String("device_id", req.DeviceID),
		zap.String("ip", c.ClientIP()),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "登录成功",
		"data": gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"user_uuid":     userUUID,
		},
	})
}

// handleRefresh 刷新 Token 接口示例
func handleRefresh(c *gin.Context) {
	ctx := context.Background()

	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn(ctx, "refresh request bind failed", zap.String("error", err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误",
		})
		return
	}

	// 刷新 Access Token
	newAccessToken, err := utils.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		logger.Warn(ctx, "refresh token failed", zap.String("error", err.Error()))
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Refresh Token 无效或已过期",
		})
		return
	}

	logger.Info(ctx, "token refreshed successfully", zap.String("ip", c.ClientIP()))

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "刷新成功",
		"data": gin.H{
			"access_token": newAccessToken,
		},
	})
}

// handleGetUserInfo 获取用户信息接口示例（需要认证）
func handleGetUserInfo(c *gin.Context) {
	ctx := context.Background()

	// 从中间件中获取用户信息
	userUUID, _ := middleware.GetUserUUID(c)
	deviceID, _ := middleware.GetDeviceID(c)

	logger.Info(ctx, "get user info",
		zap.String("user_uuid", userUUID),
		zap.String("device_id", deviceID),
	)

	// TODO: 从数据库查询用户详细信息
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"user_uuid": userUUID,
			"device_id": deviceID,
			"nickname":  "示例用户", // 模拟数据
		},
	})
}

// handleLogout 退出登录接口示例（需要认证）
func handleLogout(c *gin.Context) {
	ctx := context.Background()

	userUUID, _ := middleware.GetUserUUID(c)
	deviceID, _ := middleware.GetDeviceID(c)

	// TODO: 将 Token 加入黑名单（Redis）或从设备会话表中删除
	logger.Info(ctx, "user logout",
		zap.String("user_uuid", userUUID),
		zap.String("device_id", deviceID),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "退出成功",
	})
}
