package middleware

import (
	"ChatServer/apps/gateway/internal/utils"
	"ChatServer/pkg/logger"
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// JWTAuthMiddleware JWT 认证中间件
// 从请求头中提取 Token 并验证，验证通过后将用户信息存入 Context
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()

		// 1. 从 Header 中获取 Authorization
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn(ctx, "missing authorization header",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未提供认证信息",
			})
			c.Abort()
			return
		}

		// 2. 验证格式: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn(ctx, "invalid authorization header format",
				zap.String("header", authHeader),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证格式错误",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 3. 解析并验证 Token
		claims, err := utils.ParseToken(tokenString)
		if err != nil {
			logger.Warn(ctx, "token validation failed",
				zap.String("error", err.Error()),
				zap.String("ip", c.ClientIP()),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "Token 无效或已过期",
			})
			c.Abort()
			return
		}

		// 4. 将用户信息存入 Context，供后续 Handler 使用
		c.Set("user_uuid", claims.UserUUID)
		c.Set("device_id", claims.DeviceID)

		logger.Info(ctx, "user authenticated",
			zap.String("user_uuid", claims.UserUUID),
			zap.String("device_id", claims.DeviceID),
			zap.String("path", c.Request.URL.Path),
		)

		c.Next()
	}
}

// GetUserUUID 从 Context 中获取当前登录用户的 UUID
func GetUserUUID(c *gin.Context) (string, bool) {
	userUUID, exists := c.Get("user_uuid")
	if !exists {
		return "", false
	}
	uuid, ok := userUUID.(string)
	return uuid, ok
}

// GetDeviceID 从 Context 中获取当前设备 ID
func GetDeviceID(c *gin.Context) (string, bool) {
	deviceID, exists := c.Get("device_id")
	if !exists {
		return "", false
	}
	id, ok := deviceID.(string)
	return id, ok
}
