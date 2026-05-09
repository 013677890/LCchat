package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus 指标定义

// httpRequestsTotal 计数器：记录所有 HTTP 请求总数
// 标签：
//   - method: HTTP 方法 (GET, POST, PUT, DELETE 等)
//   - path: 请求路径 (/api/v1/login 等)
//   - status: HTTP 状态码 (200, 500)
var httpRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "gateway_http_requests_total",
		Help: "Total number of HTTP requests processed by the gateway",
	},
	[]string{"method", "path", "status"},
)

// httpBusinessCodeTotal 计数器：记录业务状态码分布
// 标签：
//   - method: HTTP 方法
//   - path: 请求路径
//   - business_code: 业务状态码 (0=成功, 10001=参数错误, 11003=密码错误 等)
var httpBusinessCodeTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "gateway_http_business_code_total",
		Help: "Total number of HTTP requests by business code",
	},
	[]string{"method", "path", "business_code"},
)

// httpRequestDuration 直方图：记录请求耗时分布
// 标签：
//   - method: HTTP 方法
//   - path: 请求路径
//
// 自动计算的百分位数：
//   - P50, P90, P95, P99 等可以通过 Bucket 配置
var httpRequestDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "gateway_http_request_duration_seconds",
		Help:    "HTTP request latency distributions in seconds",
		Buckets: prometheus.DefBuckets, // 默认桶: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
	},
	[]string{"method", "path"},
)

// httpRequestSize 直方图：记录请求体大小分布
var httpRequestSize = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "gateway_http_request_size_bytes",
		Help:    "HTTP request size distribution in bytes",
		Buckets: []float64{100, 1000, 10000, 100000, 1000000}, // 100B, 1KB, 10KB, 100KB, 1MB
	},
	[]string{"method", "path"},
)

// httpResponseSize 直方图：记录响应体大小分布
var httpResponseSize = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "gateway_http_response_size_bytes",
		Help:    "HTTP response size distribution in bytes",
		Buckets: []float64{100, 1000, 10000, 100000, 1000000},
	},
	[]string{"method", "path"},
)

// httpRequestsInProgress 仪表：当前正在处理的请求数
var httpRequestsInProgress = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "gateway_http_requests_in_progress",
		Help: "Number of HTTP requests currently being processed",
	},
	[]string{"method"},
)

// PrometheusMiddleware Prometheus 监控中间件
// 自动记录所有 HTTP 请求的指标
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		method := c.Request.Method

		// 记录当前正在处理的请求数 (+1)
		httpRequestsInProgress.WithLabelValues(method).Inc()
		defer func() {
			// 请求结束，减 1
			httpRequestsInProgress.WithLabelValues(method).Dec()
		}()

		// 处理请求
		c.Next()

		// 请求完成后，计算耗时
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		// 获取请求和响应大小
		requestSize := float64(c.Request.ContentLength)
		responseSize := float64(c.Writer.Size())

		// 获取业务状态码（从响应封装中设置的值）
		businessCode := -1
		if code, exists := c.Get("business_code"); exists {
			switch v := code.(type) {
			case int:
				businessCode = v
			case int32:
				businessCode = int(v)
			case int64:
				businessCode = int(v)
			}
		}

		// 记录指标
		// 1. 请求总数 +1（按 HTTP 状态码统计）
		httpRequestsTotal.WithLabelValues(method, path, status).Inc()

		// 2. 业务状态码统计（如果存在）
		if businessCode >= 0 {
			httpBusinessCodeTotal.WithLabelValues(method, path, strconv.Itoa(businessCode)).Inc()
		}

		// 3. 记录耗时
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)

		// 4. 记录请求大小（如果有）
		if requestSize > 0 {
			httpRequestSize.WithLabelValues(method, path).Observe(requestSize)
		}

		// 5. 记录响应大小（如果有）
		if responseSize > 0 {
			httpResponseSize.WithLabelValues(method, path).Observe(responseSize)
		}
	}
}

// GetHTTPRequestsTotal 获取 HTTP 请求总数指标（可用于监控面板）
func GetHTTPRequestsTotal() *prometheus.CounterVec {
	return httpRequestsTotal
}

// GetHTTPBusinessCodeTotal 获取业务状态码统计指标
func GetHTTPBusinessCodeTotal() *prometheus.CounterVec {
	return httpBusinessCodeTotal
}

// GetHTTPRequestDuration 获取 HTTP 请求耗时指标
func GetHTTPRequestDuration() *prometheus.HistogramVec {
	return httpRequestDuration
}
