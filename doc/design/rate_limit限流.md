# 限流中间件使用说明

## 概述

ChatServer Gateway 提供了两种级别的限流中间件：
1. **IP 级别限流** - 基于客户端 IP 地址进行限流
2. **用户级别限流** - 基于已认证用户的 UUID 进行限流

两种限流器都使用 Redis + Lua 脚本实现令牌桶算法，支持降级策略（Redis 不可用时放行请求）。

## 技术原理

### 令牌桶算法
- **容量（burst）**: 令牌桶的最大容量，允许突发流量
- **速率（rate）**: 每秒产生的令牌数量
- **消耗**: 每次请求消耗 1 个令牌
- **补充**: 按照设定的速率自动补充令牌

### Redis Key 设计
- IP 限流: `rate:limit:ip:{ip}`
- 用户限流: `rate:limit:user:{user_uuid}`

## IP 级别限流

### 1. 全局 IP 限流 - IPRateLimitMiddleware

适用于所有路由的全局 IP 限流，支持 IP 黑名单检查。

```go
// 在 router 初始化时应用
r.Use(middleware.IPRateLimitMiddleware("gateway:blacklist:ips", 10.0, 20))
```

**参数说明**:
- `blacklistKey`: Redis Set 的 key，存储黑名单 IP（如: `gateway:blacklist:ips`）
- `rate`: 每秒产生的令牌数（如: 10.0 表示每秒 10 个请求）
- `burst`: 令牌桶容量（如: 20 表示最多积累 20 个令牌，允许短时间突发）

**功能**:
1. 检查 IP 是否在黑名单中 → 返回 403 Forbidden
2. 执行令牌桶限流 → 超限返回 429 Too Many Requests
3. Redis 不可用时降级放行（Fail-Open）

### 2. 可配置 IP 限流 - IPRateLimitMiddlewareWithConfig

为特定路由或路由组配置不同的限流参数。

```go
// 为敏感接口设置更严格的限流
api.GET("/sensitive", 
    middleware.IPRateLimitMiddlewareWithConfig("gateway:blacklist:ips", 5.0, 10), 
    handler)

// 为高频接口设置宽松的限流
api.GET("/high-traffic", 
    middleware.IPRateLimitMiddlewareWithConfig("gateway:blacklist:ips", 100.0, 200), 
    handler)
```

**特点**:
- 创建独立的限流器实例
- 不同路由可设置不同的限流参数
- 懒加载 Redis 客户端，避免初始化顺序问题

## 用户级别限流

### 1. 全局用户限流 - UserRateLimitMiddleware

适用于所有已认证用户的全局限流。

```go
// 在认证路由组中应用
auth := api.Group("/auth")
auth.Use(middleware.JWTAuthMiddleware())       // JWT 认证中间件（必须在前）
auth.Use(middleware.UserRateLimitMiddleware(100.0, 200))  // 用户限流中间件
{
    user := auth.Group("/user")
    {
        user.GET("/profile", userHandler.GetProfile)
        user.PUT("/profile", userHandler.UpdateProfile)
    }
}
```

**参数说明**:
- `rate`: 每秒产生的令牌数（如: 100.0 表示每秒 100 个请求）
- `burst`: 令牌桶容量（如: 200 表示最多积累 200 个令牌）

**注意事项**:
- ⚠️ **必须在 `JWTAuthMiddleware` 之后使用**，因为需要从 Context 中获取 `user_uuid`
- 如果无法获取 `user_uuid`，会记录警告日志并放行请求
- Redis 不可用时降级放行

### 2. 可配置用户限流 - UserRateLimitMiddlewareWithConfig

为特定路由或用户行为配置不同的限流参数。

```go
auth := api.Group("/auth")
auth.Use(middleware.JWTAuthMiddleware())
{
    // 为消息发送设置较严格的限流（防止刷屏）
    auth.POST("/send-message", 
        middleware.UserRateLimitMiddlewareWithConfig(10.0, 20), 
        messageHandler.SendMessage)

    // 为查询接口设置宽松的限流
    auth.GET("/search", 
        middleware.UserRateLimitMiddlewareWithConfig(50.0, 100), 
        searchHandler.Search)

    // 为文件上传设置更严格的限流
    auth.POST("/upload", 
        middleware.UserRateLimitMiddlewareWithConfig(1.0, 5), 
        fileHandler.Upload)
}
```

**使用场景**:
- 消息发送: 防止用户刷屏（如: 10 次/秒）
- 文件上传: 限制上传频率（如: 1 次/秒）
- API 调用: 防止滥用（如: 50 次/秒）
- 搜索查询: 防止恶意查询（如: 20 次/秒）

## 完整使用示例

```go
package router

import (
    "github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
    v1 "github.com/013677890/LCchat-Backend/apps/gateway/internal/router/v1"
    "github.com/gin-gonic/gin"
)

func InitRouter(authHandler *v1.AuthHandler, userHandler *v1.UserHandler) *gin.Engine {
    r := gin.New()

    // 1. 全局中间件
    r.Use(middleware.GinRecovery(true))
    r.Use(middleware.ClientIPMiddleware())
    r.Use(middleware.GinLogger())
    
    // 2. 全局 IP 限流（所有请求）
    // 每秒 10 个请求，突发容量 20
    r.Use(middleware.IPRateLimitMiddleware("gateway:blacklist:ips", 10.0, 20))

    // 3. 公开接口（无需认证）
    api := r.Group("/api/v1")
    public := api.Group("/public")
    {
        user := public.Group("/user")
        {
            // 登录接口使用更严格的 IP 限流（防止暴力破解）
            user.POST("/login", 
                middleware.IPRateLimitMiddlewareWithConfig("gateway:blacklist:ips", 5.0, 10), 
                authHandler.Login)
            
            user.POST("/register", authHandler.Register)
            user.POST("/send-verify-code", authHandler.SendVerifyCode)
        }
    }

    // 4. 需要认证的接口
    auth := api.Group("/auth")
    auth.Use(middleware.JWTAuthMiddleware())                    // JWT 认证
    auth.Use(middleware.UserRateLimitMiddleware(100.0, 200))    // 用户级别限流
    {
        user := auth.Group("/user")
        {
            // 普通查询接口使用全局用户限流（100 次/秒）
            user.GET("/profile", userHandler.GetProfile)
            user.GET("/friends", userHandler.GetFriends)
            
            // 敏感操作使用更严格的限流
            user.POST("/change-password", 
                middleware.UserRateLimitMiddlewareWithConfig(2.0, 5), 
                userHandler.ChangePassword)
            
            user.POST("/change-email", 
                middleware.UserRateLimitMiddlewareWithConfig(2.0, 5), 
                userHandler.ChangeEmail)
        }
    }

    return r
}
```

## 初始化限流器

在 `main.go` 中初始化 Redis 限流器：

```go
package main

import (
    "github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
    "github.com/013677890/LCchat-Backend/config"
)

func main() {
    // 1. 初始化 Redis
    config.InitRedis()
    redisClient := redis.Client()

    // 2. 初始化全局 Redis 限流器
    // 参数: rate=10.0 (每秒10个令牌), burst=20 (桶容量20)
    middleware.InitRedisRateLimiter(10.0, 20, redisClient)

    // 3. 初始化路由
    router := router.InitRouter(authHandler, userHandler)
    
    // 4. 启动服务
    router.Run(":8080")
}
```

## 限流参数建议

### IP 级别限流
| 场景 | Rate (次/秒) | Burst | 说明 |
|------|-------------|-------|------|
| 全局流量 | 10-20 | 20-40 | 防止单个 IP 占用过多资源 |
| 登录接口 | 3-5 | 5-10 | 防止暴力破解 |
| 注册接口 | 1-2 | 3-5 | 防止批量注册 |
| 验证码发送 | 0.5-1 | 2-3 | 防止验证码轰炸 |

### 用户级别限流
| 场景 | Rate (次/秒) | Burst | 说明 |
|------|-------------|-------|------|
| 普通 API | 50-100 | 100-200 | 正常用户操作 |
| 消息发送 | 5-10 | 10-20 | 防止刷屏 |
| 文件上传 | 1-2 | 3-5 | 限制上传频率 |
| 敏感操作 | 1-3 | 3-5 | 修改密码、邮箱等 |
| 搜索查询 | 10-20 | 20-40 | 防止恶意查询 |

## 降级策略

当 Redis 不可用时，限流器会自动降级：
1. **检测 Redis 错误** - 连接失败、超时、取消
2. **记录警告日志** - 便于故障排查
3. **放行请求** - Fail-Open 策略，保证服务可用性

```go
// Redis 不可用时的日志示例
logger.Warn(ctx, "Redis 限流检查超时，降级放行",
    logger.String("key", key),
    logger.ErrorField("error", err),
)
```

## 监控指标

建议监控以下指标（需配合 Prometheus）：
- 限流触发次数（429 响应数量）
- Redis 限流检查延迟
- Redis 降级次数
- 各接口的 QPS

## IP 黑名单管理

使用 Redis CLI 管理 IP 黑名单：

```bash
# 添加 IP 到黑名单
redis-cli SADD gateway:blacklist:ips "192.168.1.100"

# 批量添加
redis-cli SADD gateway:blacklist:ips "192.168.1.100" "192.168.1.101"

# 查看所有黑名单 IP
redis-cli SMEMBERS gateway:blacklist:ips

# 移除 IP
redis-cli SREM gateway:blacklist:ips "192.168.1.100"

# 检查 IP 是否在黑名单
redis-cli SISMEMBER gateway:blacklist:ips "192.168.1.100"
```

## 故障排查

### 1. 限流器未生效
- 检查是否调用了 `InitRedisRateLimiter`
- 检查 Redis 连接是否正常
- 查看日志是否有 "Redis 限流器未初始化" 警告

### 2. 用户限流无法获取 user_uuid
- 确认 `UserRateLimitMiddleware` 在 `JWTAuthMiddleware` 之后
- 检查 JWT Token 是否有效
- 查看日志: "无法获取用户 UUID，跳过用户限流检查"

### 3. Redis 频繁降级
- 检查 Redis 服务状态
- 检查网络连接
- 调整 Redis 超时配置（当前: 50ms）

## 性能优化

1. **使用 Lua 脚本** - 原子性操作，减少网络往返
2. **短超时** - Redis 操作超时设为 50ms，避免慢响应
3. **懒加载客户端** - 使用 `sync.Once` 避免重复初始化
4. **RWMutex** - 读多写少场景，使用读写锁减少竞争
5. **降级策略** - Redis 不可用时放行，保证服务可用性

## 注意事项

1. ⚠️ **用户限流必须在 JWT 认证之后使用**
2. ⚠️ **不要在公开接口使用用户限流**（无法获取 user_uuid）
3. ⚠️ **合理设置限流参数**，避免误杀正常用户
4. ⚠️ **监控限流触发情况**，及时调整参数
5. ⚠️ **考虑业务特点**，不同接口使用不同参数
