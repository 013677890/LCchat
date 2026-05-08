package middleware

import (
	"net/http"
	"strings"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware JWT 认证中间件
// 从请求头中提取 Token 并验证，验证通过后将用户信息存入 Context
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从 Header 中获取 Authorization
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 客户端请求错误,属于正常业务流程,不记录日志
			result.FailWithStatusMessage(c, nil, "未提供认证信息", consts.CodeUnauthorized, http.StatusUnauthorized)
			c.Abort()
			return
		}

		// 2. 验证格式: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			// 客户端请求格式错误,属于正常业务流程,不记录日志
			result.FailWithStatusMessage(c, nil, "认证格式错误", consts.CodeUnauthorized, http.StatusUnauthorized)
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 3. 解析并验证 Token
		claims, err := util.ParseToken(tokenString)
		if err != nil {
			// Token 无效或过期,属于正常业务流程,不记录日志
			result.FailWithStatusMessage(c, nil, "Token 无效或已过期", consts.CodeInvalidToken, http.StatusUnauthorized)
			c.Abort()
			return
		}

		// 4. 将用户信息存入 Context，供后续 Handler 使用
		ctxmeta.SetUserUUID(c, claims.UserUUID)
		ctxmeta.SetDeviceID(c, claims.DeviceID)
		updateDeviceActive(claims.UserUUID, claims.DeviceID)

		c.Next()
	}
}

// GetUserUUID 从 Context 中获取当前登录用户的 UUID
func GetUserUUID(c *gin.Context) (string, bool) {
	userUUID := ctxmeta.UserUUIDFromGin(c)
	return userUUID, userUUID != ""
}

// GetDeviceID 从 Context 中获取当前设备 ID
func GetDeviceID(c *gin.Context) (string, bool) {
	deviceID := ctxmeta.DeviceIDFromGin(c)
	return deviceID, deviceID != ""
}
