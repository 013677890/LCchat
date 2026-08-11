package grpcx

import "net/http"

// NewMetricsHTTPServer 创建一个只暴露 /metrics 的 HTTP Server。
// 各服务的 provider 只需决定监听地址，不再重复手写 mux 装配。
func NewMetricsHTTPServer(address string, metrics *Metrics) *http.Server {
	mux := http.NewServeMux()
	if metrics != nil {
		mux.Handle("/metrics", metrics.Handler())
	}
	return &http.Server{Addr: address, Handler: mux}
}
