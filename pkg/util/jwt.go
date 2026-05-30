package util

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
)

// JWT 配置常量
const (
	AccessExpire  = 2 * time.Hour      // Access Token 过期时间
	RefreshExpire = 7 * 24 * time.Hour // Refresh Token 过期时间

	// devJWTSecret 仅用于本地开发/测试的回退密钥。
	// 该值已公开在源码中，任何人都能用它伪造任意用户的 Token，
	// 因此生产环境必须通过环境变量 JWT_SECRET 覆盖。
	devJWTSecret = "your-secret-key-change-in-production"
)

// jwtSecret 为 JWT 签名/校验密钥，在包初始化时从环境变量 JWT_SECRET 读取。
// 注意：auth（签发）与 gateway、connect（校验）必须配置【相同】的 JWT_SECRET，
// 否则签发方与校验方密钥不一致会导致所有 Token 校验失败。
var jwtSecret = loadJWTSecret()

// loadJWTSecret 从环境变量 JWT_SECRET 读取密钥；未设置时回退到不安全的开发默认值并打印告警。
func loadJWTSecret() []byte {
	if secret := strings.TrimSpace(os.Getenv("JWT_SECRET")); secret != "" {
		return []byte(secret)
	}
	logger.Warn(context.Background(), "JWT_SECRET 未设置，使用开发默认值")
	return []byte(devJWTSecret)
}

// CustomClaims 自定义 JWT Claims
type CustomClaims struct {
	UserUUID string `json:"user_uuid"` // 用户唯一标识
	DeviceID string `json:"device_id"` // 设备 ID（用于多端登录管理）
	jwt.RegisteredClaims
}

// GenerateToken 生成 Access Token
// userUUID: 用户唯一标识
// deviceID: 设备唯一标识
// 返回: token 字符串和可能的错误
func GenerateToken(userUUID, deviceID string) (string, error) {
	// 设置过期时间
	now := time.Now()
	claims := CustomClaims{
		UserUUID: userUUID,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "ChatServer-Gateway", // 签发者
		},
	}

	// 使用 HS256 算法签名
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ParseToken 解析并验证 Token
// tokenString: JWT token 字符串
// 返回: 解析后的 Claims 和可能的错误
func ParseToken(tokenString string) (*CustomClaims, error) {
	// 解析 token
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	// 提取 claims
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
