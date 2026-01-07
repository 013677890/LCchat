package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT 配置常量
const (
	// TODO: 生产环境应从配置文件或环境变量读取
	JWTSecret     = "your-secret-key-change-in-production" // JWT 签名密钥
	AccessExpire  = 2 * time.Hour                          // Access Token 过期时间
	RefreshExpire = 7 * 24 * time.Hour                     // Refresh Token 过期时间
)

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
	tokenString, err := token.SignedString([]byte(JWTSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GenerateRefreshToken 生成 Refresh Token
// userUUID: 用户唯一标识
// deviceID: 设备唯一标识
// 返回: refresh token 字符串和可能的错误
func GenerateRefreshToken(userUUID, deviceID string) (string, error) {
	now := time.Now()
	claims := CustomClaims{
		UserUUID: userUUID,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "ChatServer-Gateway",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(JWTSecret))
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
		return []byte(JWTSecret), nil
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

// RefreshAccessToken 使用 Refresh Token 刷新 Access Token
// refreshToken: refresh token 字符串
// 返回: 新的 access token 和可能的错误
func RefreshAccessToken(refreshToken string) (string, error) {
	// 解析 refresh token
	claims, err := ParseToken(refreshToken)
	if err != nil {
		return "", err
	}

	// 生成新的 access token
	return GenerateToken(claims.UserUUID, claims.DeviceID)
}

