package middleware

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// gRPCRequestsTotal gRPC 请求计数器。
// 标签：
//   - service: 服务名 (user.UserService)
//   - method: 方法名 (Login)
//   - status: 状态 (ok, error)
var gRPCRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "gateway_grpc_requests_total",
		Help: "Total number of gRPC requests",
	},
	[]string{"service", "method", "status"},
)

// gRPCRequestDuration gRPC 请求耗时。
var gRPCRequestDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "gateway_grpc_request_duration_seconds",
		Help:    "gRPC request latency distributions in seconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"service", "method"},
)

// RecordGRPCRequest 记录 gateway 出站 gRPC 请求指标。
func RecordGRPCRequest(service, method string, duration float64, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}

	gRPCRequestsTotal.WithLabelValues(service, method, status).Inc()
	gRPCRequestDuration.WithLabelValues(service, method).Observe(duration)
}

// GRPCMetricsObserver 把 gateway 的下游 gRPC 指标采集挂到 grpcx 统一 observer 上。
// 这样 facade 本身不再负责手动计时与上报，职责边界收敛到建连阶段。
func GRPCMetricsObserver() grpcx.ClientCallObserver {
	return func(_ context.Context, result grpcx.ClientCallResult) {
		RecordGRPCRequest(result.Service, result.Method, result.Cost.Seconds(), result.Err)
	}
}
