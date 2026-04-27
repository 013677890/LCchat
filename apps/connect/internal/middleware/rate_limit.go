package middleware

import (
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const (
	// wsHandshakeMaxEntriesDefault 限流桶最大 IP 数默认值。
	// 超过该值后会执行随机淘汰，防止恶意随机 IP 导致内存膨胀。
	wsHandshakeMaxEntriesDefault = 50000
	// wsHandshakeEvictBatch 随机淘汰批次大小上限。
	wsHandshakeEvictBatch = 256
)

// WSHandshakeRateLimitConfig 定义 /ws 握手限流配置。
type WSHandshakeRateLimitConfig struct {
	// Rate 表示每秒允许的握手请求数。
	Rate float64
	// Burst 表示瞬时突发容量。
	Burst int
	// BucketTTL 表示 IP 桶的空闲回收时间，避免内存无限增长。
	BucketTTL time.Duration
	// CleanupInterval 表示清理周期。
	CleanupInterval time.Duration
	// MaxEntries 表示可保留的 IP 桶上限，防止内存无限增长。
	MaxEntries int
}

// DefaultWSHandshakeRateLimitConfig 返回默认握手限流参数。
// 环境变量：
// - CONNECT_WS_HANDSHAKE_RATE: 每秒握手数（默认 5）
// - CONNECT_WS_HANDSHAKE_BURST: 突发容量（默认 20）
// - CONNECT_WS_HANDSHAKE_MAX_ENTRIES: 最大 IP 桶数量（默认 50000）
func DefaultWSHandshakeRateLimitConfig() WSHandshakeRateLimitConfig {
	return WSHandshakeRateLimitConfig{
		Rate:            parseFloatEnv("CONNECT_WS_HANDSHAKE_RATE", 5),
		Burst:           parseIntEnv("CONNECT_WS_HANDSHAKE_BURST", 20),
		BucketTTL:       10 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		MaxEntries:      parseIntEnv("CONNECT_WS_HANDSHAKE_MAX_ENTRIES", wsHandshakeMaxEntriesDefault),
	}
}

type ipLimiterEntry struct {
	limiter *rate.Limiter
	// lastSeenUnixNano 使用原子值，降低全局锁占用时间。
	lastSeenUnixNano atomic.Int64
}

type handshakeLimiter struct {
	cfg     WSHandshakeRateLimitConfig
	mu      sync.Mutex
	entries map[string]*ipLimiterEntry
}

func newHandshakeLimiter(cfg WSHandshakeRateLimitConfig) *handshakeLimiter {
	l := &handshakeLimiter{
		cfg:     cfg,
		entries: make(map[string]*ipLimiterEntry),
	}
	l.startCleanupLoop()
	return l
}

func (l *handshakeLimiter) allow(ip string, now time.Time) bool {
	entry := l.getOrCreateEntry(ip, now)
	if entry == nil {
		// 容量保护触发时采用降级放行，避免误伤合法握手请求。
		return true
	}
	entry.lastSeenUnixNano.Store(now.UnixNano())

	// 只在 entry 级别执行令牌检查，不再持有全局 map 锁。
	return entry.limiter.AllowN(now, 1)
}

func (l *handshakeLimiter) getOrCreateEntry(ip string, now time.Time) *ipLimiterEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, ok := l.entries[ip]; ok {
		return entry
	}

	if l.cfg.MaxEntries > 0 && len(l.entries) >= l.cfg.MaxEntries {
		l.evictRandomLocked(l.evictCount())
	}
	if l.cfg.MaxEntries > 0 && len(l.entries) >= l.cfg.MaxEntries {
		return nil
	}

	entry := &ipLimiterEntry{
		limiter: rate.NewLimiter(rate.Limit(l.cfg.Rate), l.cfg.Burst),
	}
	entry.lastSeenUnixNano.Store(now.UnixNano())
	l.entries[ip] = entry
	return entry
}

func (l *handshakeLimiter) startCleanupLoop() {
	ticker := time.NewTicker(l.cfg.CleanupInterval)
	go func() {
		defer ticker.Stop()
		for now := range ticker.C {
			l.cleanupExpired(now)
		}
	}()
}

func (l *handshakeLimiter) cleanupExpired(now time.Time) {
	expireBefore := now.Add(-l.cfg.BucketTTL).UnixNano()

	l.mu.Lock()
	for ip, entry := range l.entries {
		if entry.lastSeenUnixNano.Load() < expireBefore {
			delete(l.entries, ip)
		}
	}
	l.mu.Unlock()
}

func (l *handshakeLimiter) evictRandomLocked(n int) {
	if n <= 0 {
		n = 1
	}
	removed := 0
	for ip := range l.entries {
		delete(l.entries, ip)
		removed++
		if removed >= n {
			return
		}
	}
}

func (l *handshakeLimiter) evictCount() int {
	n := l.cfg.MaxEntries / 100
	if n < 1 {
		n = 1
	}
	if n > wsHandshakeEvictBatch {
		n = wsHandshakeEvictBatch
	}
	return n
}

// WSHandshakeRateLimitMiddleware 仅用于 /ws 握手请求限流。
// 注意：它只限制“建连频率”，不干预 WebSocket 长连接内的消息收发。
func WSHandshakeRateLimitMiddleware(cfg WSHandshakeRateLimitConfig) gin.HandlerFunc {
	if cfg.Rate <= 0 || cfg.Burst <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	if cfg.BucketTTL <= 0 {
		cfg.BucketTTL = 10 * time.Minute
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 1 * time.Minute
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = wsHandshakeMaxEntriesDefault
	}

	limiter := newHandshakeLimiter(cfg)
	return func(c *gin.Context) {
		ip := ctxmeta.ClientIPFromGin(c)
		if ip == "" {
			ip = c.ClientIP()
		}
		if ip == "" {
			c.Next()
			return
		}

		now := time.Now()
		if limiter.allow(ip, now) {
			c.Next()
			return
		}

		logCtx := ctxmeta.BuildContextFromGin(c)
		logger.Warn(logCtx, "WebSocket 握手请求被限流",
			logger.String("ip", ip),
			logger.String("path", c.Request.URL.Path),
			logger.String("method", c.Request.Method),
		)

		c.Set("business_code", consts.CodeTooManyRequests)
		c.JSON(http.StatusTooManyRequests, result.Response{
			Code:      consts.CodeTooManyRequests,
			Message:   consts.GetMessage(consts.CodeTooManyRequests),
			Data:      nil,
			TraceId:   ctxmeta.TraceIDFromGin(c),
			Timestamp: now.Unix(),
		})
		c.Abort()
	}
}

func parseFloatEnv(key string, fallback float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseIntEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
