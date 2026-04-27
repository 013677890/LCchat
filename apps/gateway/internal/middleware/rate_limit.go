package middleware

import (
	"github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// ==================== Redis 令牌桶 Lua 脚本 ====================

// luaTokenBucketRedis Redis 令牌桶 Lua 脚本
// 功能：原子性地更新令牌桶并判断是否允许通过
// 参数：
//
//	KEYS[1]: 限流 key (如: rate:limit:ip:{ip})
//	ARGV[1]: 当前时间戳 (毫秒)
//	ARGV[2]: 令牌桶容量
//	ARGV[3]: 每秒产生的令牌数 (乘以1000转换为毫秒精度)
//	ARGV[4]: 每次请求消耗的令牌数
//
// 返回值：
//   - 1: 允许通过
//   - 0: 不允许通过 (令牌不足)
//
// 注意：时间戳使用毫秒级精度以提高计算准确性
const luaTokenBucketRedis = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local rate = tonumber(ARGV[3]) -- 每秒产生的令牌数
local requested = tonumber(ARGV[4])

-- 获取当前状态
local info = redis.call('HMGET', key, 'tokens', 'last_time')
local current_tokens = tonumber(info[1])
local last_time = tonumber(info[2])

-- 初始化
if current_tokens == nil then
    current_tokens = capacity
end
if last_time == nil then
    last_time = now
end

-- 计算时间差 (毫秒)
local time_diff = math.max(0, now - last_time)

-- 计算补充令牌: (时间差ms * 速率) / 1000
-- 比如: 100ms * 10r/s / 1000 = 1 个令牌
local new_tokens = math.floor((time_diff * rate) / 1000)

-- 更新令牌数
if new_tokens > 0 then
    current_tokens = math.min(capacity, current_tokens + new_tokens)
    last_time = now -- 只有产生了新令牌或者消耗了令牌才更新时间，防止精度丢失
end

-- 判断是否允许通过
local allowed = 0
if current_tokens >= requested then
    current_tokens = current_tokens - requested
    allowed = 1
end

-- 更新 Redis
redis.call('HMSET', key, 'tokens', current_tokens, 'last_time', last_time)

-- 设置过期时间：桶填满所需时间 * 2，至少 60 秒
local fill_time = math.ceil(capacity / rate)
local ttl = math.max(60, fill_time * 2)
redis.call('EXPIRE', key, ttl)

return allowed
`

// ==================== Redis 限流器 ====================

// RedisRateLimiter 基于 Redis 的 IP 级别限流器
type RedisRateLimiter struct {
	redisClient *redis.Client
	rate        float64 // 每秒产生的令牌数
	burst       int     // 令牌桶容量
	mu          *sync.RWMutex
	failOpen    bool // 降级标志：true 表示 Redis 不可用，降级放行
}

// NewRedisRateLimiter 创建 Redis 限流器
// rate: 每秒产生的令牌数 (如: 10.0 表示每秒10个令牌)
// burst: 令牌桶容量 (如: 20 表示桶最多20个令牌)
func NewRedisRateLimiter(rate float64, burst int) *RedisRateLimiter {
	return &RedisRateLimiter{
		rate:     rate,
		burst:    burst,
		mu:       &sync.RWMutex{},
		failOpen: false, // 初始不降级
	}
}

// RedisSetClient 设置 Redis 客户端
// 使用延迟初始化避免循环依赖
func (r *RedisRateLimiter) RedisSetClient(redisClient *redis.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.redisClient = redisClient
}

// Allow 检查是否允许请求通过
// key: Redis 限流 key (如: rate:limit:ip:{ip})
// 返回值：
//   - bool: true 表示允许通过，false 表示被限流
//   - error: 错误信息，Redis 不可用时降级返回 nil
func (r *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	// 使用 RLock 读取 client，减少锁竞争
	r.mu.RLock()
	client := r.redisClient
	r.mu.RUnlock()

	if client == nil {
		// Redis 客户端未初始化，降级放行
		return true, nil
	}

	// 计算令牌桶参数
	now := time.Now().UnixMilli() // 当前时间戳（毫秒）

	// 【修正点】直接传 rate 给 Lua 脚本，由 Lua 内部除以 1000 计算毫秒精度
	// KEYS[1]: key
	// ARGV[1]: now (当前时间戳，毫秒)
	// ARGV[2]: r.burst (桶容量)
	// ARGV[3]: r.rate (每秒产生的令牌数，不要乘 1000)
	// ARGV[4]: 1 (每次请求消耗的令牌数)

	// 优化：给 Redis 操作加一个独立的短超时（50ms），防止 Redis 响应慢拖死网关
	redisCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	cmd := client.Eval(redisCtx, luaTokenBucketRedis, []string{key}, now, r.burst, r.rate, 1)
	result, err := cmd.Result()

	if err != nil {
		// 检查是否为 Redis 连接错误或超时
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			// 超时或取消，记录错误并降级放行
			logger.Warn(ctx, "Redis 限流检查超时，降级放行",
				logger.String("key", key),
				logger.ErrorField("error", err),
			)
			return true, nil
		}

		// 其他 Redis 错误
		logger.Error(ctx, "Redis 限流检查失败，降级放行",
			logger.String("key", key),
			logger.ErrorField("error", err),
		)
		return true, nil
	}

	// 检查 Lua 脚本返回值
	// 返回 1 表示允许通过，0 表示被限流
	allowed, ok := result.(int64)
	if !ok {
		// 类型断言失败，降级放行
		logger.Warn(ctx, "Redis 限流返回值类型错误，降级放行",
			logger.String("key", key),
			logger.Any("result", result),
		)
		return true, nil
	}

	return allowed == 1, nil
}

// CheckBlacklist 检查 IP 是否在黑名单中
// blacklistKey: Redis 黑名单 Set 的 key (如: gateway:blacklist:ips)
// ip: 要检查的 IP 地址
// 返回值：
//   - bool: true 表示在黑名单中，false 表示不在
//   - error: 错误信息，Redis 不可用时降级返回 nil
func CheckBlacklist(ctx context.Context, blacklistKey, ip string) (bool, error) {
	// 获取 Redis 客户端
	client := pkgredis.Client()
	if client == nil {
		// Redis 客户端未初始化，降级放行（不在黑名单）
		return false, nil
	}

	// 检查 IP 是否在黑名单 Set 中
	// 使用 SISMEMBER 命令
	cmd := client.SIsMember(ctx, blacklistKey, ip)
	exists, err := cmd.Result()
	if err != nil {
		// 检查是否为 Redis 连接错误
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			// 超时或取消，记录错误并降级放行
			logger.Warn(ctx, "Redis 黑名单检查超时，降级放行",
				logger.String("ip", ip),
				logger.ErrorField("error", err),
			)
			return false, nil
		}

		// 其他 Redis 错误
		logger.Error(ctx, "Redis 黑名单检查失败，降级放行",
			logger.String("ip", ip),
			logger.ErrorField("error", err),
		)
		return false, nil
	}

	return exists, nil
}

// ==================== Redis 限流中间件 ====================

// 全局 Redis 限流器实例
var globalRedisLimiter *RedisRateLimiter

// InitRedisRateLimiter 初始化全局 Redis 限流器
// rate: 每秒产生的令牌数
// burst: 令牌桶容量
// redisClient: Redis 客户端实例
func InitRedisRateLimiter(rate float64, burst int, redisClient *redis.Client) {
	globalRedisLimiter = NewRedisRateLimiter(rate, burst)

	// 设置 Redis 客户端
	globalRedisLimiter.RedisSetClient(redisClient)

	logger.Info(context.Background(), "Redis 限流器初始化完成",
		logger.Float64("rate", rate),
		logger.Int("burst", burst),
	)
}

// ==================== Redis IP 限流中间件 ====================

// IPRateLimitMiddleware 基于 Redis 的 IP 级别限流中间件
// 支持黑名单检查、令牌桶限流、降级策略
// 参数：
//   - blacklistKey: 黑名单 Redis Set 的 key (如: gateway:blacklist:ips)
//   - rate: 每秒产生的令牌数
//   - burst: 令牌桶容量
//
// 使用示例：
//
//	router.Use(IPRateLimitMiddleware("gateway:blacklist:ips", 10, 20))
func IPRateLimitMiddleware(blacklistKey string, rate float64, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c

		// 1. 获取客户端 IP
		ip, exists := GetClientIPSafe(c)
		if !exists || ip == "" {
			// 无法获取 IP，放行请求（记录警告）
			logger.Warn(ctx, "无法获取客户端 IP，跳过限流检查",
				logger.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		// 2. 检查 IP 黑名单
		inBlacklist, err := CheckBlacklist(ctx, blacklistKey, ip)
		if err != nil {
			// Redis 错误，已经降级放行了，记录日志即可
			// 继续后续流程
		} else if inBlacklist {
			// IP 在黑名单中，直接拒绝
			logger.Warn(ctx, "IP 在黑名单中，拒绝访问",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
			)

			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "访问被禁止，请联系管理员",
			})
			c.Abort()
			return
		}

		// 3. 执行 IP 限流检查
		if globalRedisLimiter == nil {
			// 限流器未初始化，放行请求
			logger.Warn(ctx, "Redis 限流器未初始化，跳过限流检查",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		// 构造限流 key: rate:limit:ip:{ip}
		rateLimitKey := rediskey.GatewayIPRateLimitKey(ip)

		// 检查是否允许通过
		allowed, err := globalRedisLimiter.Allow(ctx, rateLimitKey)
		if err != nil {
			// Redis 错误，已经降级放行了（返回 true）
			// 继续后续流程
			logger.Warn(ctx, "Redis 限流检查异常，降级放行",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
				logger.ErrorField("error", err),
			)
		} else if !allowed {
			// 被限流
			logger.Warn(ctx, "IP 请求被限流",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
			)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    10005,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		// 4. 通过检查，继续处理请求
		c.Next()
	}
}

// ==================== 用户限流中间件 ====================

// UserRateLimitMiddleware 基于用户 UUID 的限流中间件
// 用于对已认证用户进行限流，需要在 JWT 认证中间件之后使用
// 参数：
//   - rate: 每秒产生的令牌数
//   - burst: 令牌桶容量
//
// 使用示例：
//
//	// 在路由中使用（需要在 JWTAuthMiddleware 之后）
//	api.Use(JWTAuthMiddleware())
//	api.Use(UserRateLimitMiddleware(100, 200))
func UserRateLimitMiddleware(rate float64, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c

		// 1. 获取用户 UUID
		userUUID, exists := GetUserUUID(c)
		if !exists || userUUID == "" {
			// 无法获取用户 UUID，可能是未认证请求，放行
			// 注意：这个中间件应该在 JWTAuthMiddleware 之后使用
			logger.Warn(ctx, "无法获取用户 UUID，跳过用户限流检查",
				logger.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		// 2. 检查全局限流器是否初始化
		if globalRedisLimiter == nil {
			// 限流器未初始化，放行请求
			logger.Warn(ctx, "Redis 限流器未初始化，跳过用户限流检查",
				logger.String("user_uuid", userUUID),
				logger.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		// 3. 构造用户限流 key: gateway:rate:limit:user:{user_uuid}
		rateLimitKey := rediskey.GatewayUserRateLimitKey(userUUID)

		// 4. 检查是否允许通过
		allowed, err := globalRedisLimiter.Allow(ctx, rateLimitKey)
		if err != nil {
			// Redis 错误，已经降级放行了（返回 true）
			logger.Warn(ctx, "Redis 用户限流检查异常，降级放行",
				logger.String("user_uuid", userUUID),
				logger.String("path", c.Request.URL.Path),
				logger.ErrorField("error", err),
			)
		} else if !allowed {
			// 用户被限流
			logger.Warn(ctx, "用户请求被限流",
				logger.String("user_uuid", userUUID),
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
			)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    10005,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		// 5. 通过检查，继续处理请求
		c.Next()
	}
}

// UserRateLimitMiddlewareWithConfig 可配置的用户限流中间件
// 允许为不同的路由组设置不同的限流参数
// 参数：
//   - rate: 每秒产生的令牌数
//   - burst: 令牌桶容量
//
// 使用示例：
//
//	// 为敏感接口设置更严格的限流
//	api.POST("/sensitive", UserRateLimitMiddlewareWithConfig(10, 20), handler)
func UserRateLimitMiddlewareWithConfig(rate float64, burst int) gin.HandlerFunc {
	// 创建独立的限流器实例
	limiter := NewRedisRateLimiter(rate, burst)

	// 使用 sync.Once 懒加载 Redis Client（只执行一次，避免每次请求都加锁）
	var once sync.Once

	return func(c *gin.Context) {
		ctx := c

		// 懒加载 Redis Client，只执行一次
		once.Do(func() {
			if client := pkgredis.Client(); client != nil {
				limiter.RedisSetClient(client)
			}
		})

		// 1. 获取用户 UUID
		userUUID, exists := GetUserUUID(c)
		if !exists || userUUID == "" {
			// 无法获取用户 UUID，可能是未认证请求，放行
			logger.Warn(ctx, "无法获取用户 UUID，跳过用户限流检查",
				logger.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		// 2. 构造用户限流 key: gateway:rate:limit:user:{user_uuid}
		rateLimitKey := rediskey.GatewayUserRateLimitKey(userUUID)

		// 3. 检查是否允许通过
		allowed, err := limiter.Allow(ctx, rateLimitKey)
		if err != nil {
			// Redis 错误，已经降级放行了（返回 true）
			logger.Warn(ctx, "Redis 用户限流检查异常，降级放行",
				logger.String("user_uuid", userUUID),
				logger.String("path", c.Request.URL.Path),
				logger.ErrorField("error", err),
			)
		} else if !allowed {
			// 用户被限流
			logger.Warn(ctx, "用户请求被限流",
				logger.String("user_uuid", userUUID),
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
			)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    10005,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		// 4. 通过检查，继续处理请求
		c.Next()
	}
}

// ==================== IP 限流中间件（可配置） ====================

// IPRateLimitMiddlewareWithConfig 可配置的 Redis IP 限流中间件
// 允许为不同的路由组设置不同的限流参数
// 参数：
//   - blacklistKey: 黑名单 Redis Set 的 key
//   - rate: 每秒产生的令牌数
//   - burst: 令牌桶容量
//
// 使用示例：
//
//	api.GET("/sensitive", IPRateLimitMiddlewareWithConfig("gateway:blacklist:ips", 5, 10), handler)
func IPRateLimitMiddlewareWithConfig(blacklistKey string, rate float64, burst int) gin.HandlerFunc {
	// 创建独立的限流器实例
	limiter := NewRedisRateLimiter(rate, burst)

	// 2. 使用 sync.Once 懒加载 Redis Client（只执行一次，避免每次请求都加锁）
	var once sync.Once

	return func(c *gin.Context) {
		ctx := c

		// 懒加载 Redis Client，只执行一次
		once.Do(func() {
			if client := pkgredis.Client(); client != nil {
				limiter.RedisSetClient(client)
			}
		})

		// 1. 获取客户端 IP
		ip, exists := GetClientIPSafe(c)
		if !exists || ip == "" {
			// 无法获取 IP，放行请求（记录警告）
			logger.Warn(ctx, "无法获取客户端 IP，跳过限流检查",
				logger.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		// 2. 检查 IP 黑名单
		inBlacklist, err := CheckBlacklist(ctx, blacklistKey, ip)
		if err != nil {
			// Redis 错误，已经降级放行了，记录日志即可
			// 继续后续流程
		} else if inBlacklist {
			// IP 在黑名单中，直接拒绝
			logger.Warn(ctx, "IP 在黑名单中，拒绝访问",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
			)

			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "访问被禁止，请联系管理员",
			})
			c.Abort()
			return
		}

		// 3. 执行 IP 限流检查（Redis Client 已在初始化时设置）
		// limiter.RedisSetClient(pkgredis.Client())

		// 构造限流 key: rate:limit:ip:{ip}
		rateLimitKey := rediskey.GatewayIPRateLimitKey(ip)

		// 检查是否允许通过
		allowed, err := limiter.Allow(ctx, rateLimitKey)
		if err != nil {
			// Redis 错误，已经降级放行了（返回 true）
			// 继续后续流程
			logger.Warn(ctx, "Redis 限流检查异常，降级放行",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
				logger.ErrorField("error", err),
			)
		} else if !allowed {
			// 被限流
			logger.Warn(ctx, "IP 请求被限流",
				logger.String("ip", ip),
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
			)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    10005,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		// 4. 通过检查，继续处理请求
		c.Next()
	}
}
