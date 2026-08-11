package grpcx

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// MetricsConfig Prometheus 指标拦截器配置。
type MetricsConfig struct {
	// Namespace 指标名前缀，用于区分不同服务。
	// 例如 "user" 会生成 user_grpc_request_total、user_grpc_request_duration_seconds 等。
	// 为空时不加前缀（不建议，多服务会冲突）。
	Namespace string
	// Buckets 延迟直方图分桶（单位秒），为空时使用默认分桶。
	Buckets []float64
}

// DefaultMetricsConfig 返回默认指标配置。
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Namespace: "",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	}
}

// Metrics 封装 Prometheus 指标收集器和 HTTP handler。
// 每个服务应创建一个独立的 Metrics 实例。
type Metrics struct {
	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestInFlight *prometheus.GaugeVec
	registry        *prometheus.Registry
}

// NewMetrics 创建独立的 Prometheus 指标实例。
// 使用独立 Registry，避免多服务 init() 冲突。
func NewMetrics(cfgs ...MetricsConfig) *Metrics {
	cfg := DefaultMetricsConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	buckets := cfg.Buckets
	if len(buckets) == 0 {
		buckets = DefaultMetricsConfig().Buckets
	}

	m := &Metrics{
		requestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Name:      "grpc_request_total",
				Help:      "Total number of gRPC requests",
			},
			[]string{"method", "code"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.Namespace,
				Name:      "grpc_request_duration_seconds",
				Help:      "gRPC request latency in seconds",
				Buckets:   buckets,
			},
			[]string{"method"},
		),
		requestInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cfg.Namespace,
				Name:      "grpc_request_in_flight",
				Help:      "Current number of gRPC requests in flight",
			},
			[]string{"method"},
		),
		registry: prometheus.NewRegistry(),
	}

	m.registry.MustRegister(m.requestTotal, m.requestDuration, m.requestInFlight)
	// 同时注册到默认 Registry 以保持与现有 /metrics 端点的兼容性。
	// 如果已存在同名指标（如多次创建），再次注册不会 panic，因为 Register 只返回错误。
	prometheus.Register(m.requestTotal)
	prometheus.Register(m.requestDuration)
	prometheus.Register(m.requestInFlight)

	return m
}

// UnaryInterceptor 返回指标采集拦截器。
func (m *Metrics) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		m.requestInFlight.WithLabelValues(info.FullMethod).Inc()
		defer m.requestInFlight.WithLabelValues(info.FullMethod).Dec()

		start := time.Now()
		resp, err = handler(ctx, req)
		duration := time.Since(start).Seconds()

		m.requestDuration.WithLabelValues(info.FullMethod).Observe(duration)

		code := status.Code(err).String()
		m.requestTotal.WithLabelValues(info.FullMethod, code).Inc()

		return resp, err
	}
}

// Handler 返回 Prometheus HTTP handler，用于暴露 /metrics 端点。
// 使用独立 Registry 的 handler，仅包含本服务的指标。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
