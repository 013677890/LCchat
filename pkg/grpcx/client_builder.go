package grpcx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultClientLoadBalancingPolicy = "round_robin"

// ClientOptions 定义统一的 gRPC 客户端构造参数。
//
// 设计目标：
//   - 让 metadata / timeout / logging / internal-caller / retry 的装配顺序固定；
//   - 让各服务只保留“这条连接连谁、需要哪些策略”的最小差异；
//   - 业务方若有额外需求（如 gateway 的熔断器），通过 ExtraUnaryInterceptors 注入。
type ClientOptions struct {
	Address string

	// Credentials 允许调用方覆盖传输凭证；未显式指定时默认走 insecure，
	// 与当前仓库内部服务直连的现状保持一致。
	Credentials credentials.TransportCredentials

	Timeout        *ClientTimeoutConfig
	Logging        *ClientLoggingConfig
	InternalCaller *InternalCallerClientConfig
	Retry          *ClientRetryConfig
	Observers      []ClientCallObserver

	// LoadBalancingPolicy 写入 gRPC service config。
	// 默认使用 round_robin，让 Docker/Kubernetes DNS 返回多个后端地址时能分摊请求。
	LoadBalancingPolicy string

	ExtraUnaryInterceptors []grpc.UnaryClientInterceptor

	// MaxRecvMsgSize / MaxSendMsgSize 作用于默认 CallOptions。
	MaxRecvMsgSize int
	MaxSendMsgSize int
}

// ClientRetryConfig 定义 gRPC ServiceConfig 中的重试策略。
//
// 注意：ServiceNames 必须是精确的 protobuf service 名称（如 group.GroupService），
// 不能偷懒写成某个固定模板，否则不同服务会共用错误的重试规则。
type ClientRetryConfig struct {
	ServiceNames         []string
	Timeout              time.Duration
	WaitForReady         bool
	MaxAttempts          int
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	BackoffMultiplier    float64
	RetryableStatusCodes []string
}

// DefaultClientRetryConfig 返回仓库统一的默认重试骨架。
// 调用方拿到的是一份可修改副本，可在不同连接上覆写 attempts / timeout / codes。
func DefaultClientRetryConfig(serviceNames ...string) *ClientRetryConfig {
	filtered := make([]string, 0, len(serviceNames))
	for _, serviceName := range serviceNames {
		serviceName = strings.TrimSpace(serviceName)
		if serviceName == "" {
			continue
		}
		filtered = append(filtered, serviceName)
	}

	return &ClientRetryConfig{
		ServiceNames:         filtered,
		Timeout:              2 * time.Second,
		WaitForReady:         true,
		MaxAttempts:          5,
		InitialBackoff:       100 * time.Millisecond,
		MaxBackoff:           time.Second,
		BackoffMultiplier:    2,
		RetryableStatusCodes: []string{"UNAVAILABLE", "DEADLINE_EXCEEDED", "UNKNOWN"},
	}
}

// NewClient 用统一装配顺序创建 gRPC ClientConn。
func NewClient(opts ClientOptions) (*grpc.ClientConn, error) {
	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return nil, fmt.Errorf("grpc client address is empty")
	}

	transportCredentials := opts.Credentials
	if transportCredentials == nil {
		transportCredentials = insecure.NewCredentials()
	}

	interceptors := []grpc.UnaryClientInterceptor{MetadataUnaryClientInterceptor()}
	if opts.InternalCaller != nil {
		interceptors = append(interceptors, InternalCallerUnaryClientInterceptor(*opts.InternalCaller))
	}
	if opts.Timeout != nil {
		interceptors = append(interceptors, ClientTimeoutUnaryInterceptor(*opts.Timeout))
	}
	if opts.Logging != nil {
		interceptors = append(interceptors, LoggingUnaryClientInterceptor(*opts.Logging))
	} else {
		interceptors = append(interceptors, LoggingUnaryClientInterceptor())
	}
	for _, observer := range opts.Observers {
		interceptors = append(interceptors, ObserveUnaryClientInterceptor(observer))
	}
	interceptors = append(interceptors, opts.ExtraUnaryInterceptors...)

	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(transportCredentials)}
	if serviceConfigJSON, err := buildClientServiceConfig(opts.Retry, opts.LoadBalancingPolicy); err != nil {
		return nil, err
	} else if serviceConfigJSON != "" {
		dialOptions = append(dialOptions, grpc.WithDefaultServiceConfig(serviceConfigJSON))
	}

	callOptions := buildClientCallOptions(opts)
	if len(callOptions) > 0 {
		dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(callOptions...))
	}
	if len(interceptors) > 0 {
		dialOptions = append(dialOptions, grpc.WithChainUnaryInterceptor(interceptors...))
	}

	return grpc.NewClient(address, dialOptions...)
}

func normalizeClientLoadBalancingPolicy(policy string) string {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return defaultClientLoadBalancingPolicy
	}
	return policy
}

func buildClientCallOptions(opts ClientOptions) []grpc.CallOption {
	callOptions := make([]grpc.CallOption, 0, 2)
	if opts.MaxRecvMsgSize > 0 {
		callOptions = append(callOptions, grpc.MaxCallRecvMsgSize(opts.MaxRecvMsgSize))
	}
	if opts.MaxSendMsgSize > 0 {
		callOptions = append(callOptions, grpc.MaxCallSendMsgSize(opts.MaxSendMsgSize))
	}
	return callOptions
}

func buildClientServiceConfig(cfg *ClientRetryConfig, loadBalancingPolicy string) (string, error) {
	loadBalancingPolicy = normalizeClientLoadBalancingPolicy(loadBalancingPolicy)
	serviceNames := make([]string, 0)
	if cfg != nil {
		serviceNames = make([]string, 0, len(cfg.ServiceNames))
		for _, serviceName := range cfg.ServiceNames {
			serviceName = strings.TrimSpace(serviceName)
			if serviceName == "" {
				continue
			}
			serviceNames = append(serviceNames, serviceName)
		}
	}

	type serviceNameEntry struct {
		Service string `json:"service"`
	}
	type retryPolicy struct {
		MaxAttempts          int      `json:"maxAttempts"`
		InitialBackoff       string   `json:"initialBackoff"`
		MaxBackoff           string   `json:"maxBackoff"`
		BackoffMultiplier    float64  `json:"backoffMultiplier"`
		RetryableStatusCodes []string `json:"retryableStatusCodes"`
	}
	type methodConfig struct {
		Name         []serviceNameEntry `json:"name"`
		WaitForReady bool               `json:"waitForReady"`
		Timeout      string             `json:"timeout"`
		RetryPolicy  retryPolicy        `json:"retryPolicy"`
	}
	type serviceConfig struct {
		LoadBalancingConfig []map[string]any `json:"loadBalancingConfig,omitempty"`
		MethodConfig        []methodConfig   `json:"methodConfig,omitempty"`
	}

	payload := serviceConfig{}
	if loadBalancingPolicy != "" {
		payload.LoadBalancingConfig = []map[string]any{{
			loadBalancingPolicy: map[string]any{},
		}}
	}

	if len(serviceNames) > 0 {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		maxAttempts := cfg.MaxAttempts
		if maxAttempts <= 1 {
			maxAttempts = 2
		}
		initialBackoff := cfg.InitialBackoff
		if initialBackoff <= 0 {
			initialBackoff = 100 * time.Millisecond
		}
		maxBackoff := cfg.MaxBackoff
		if maxBackoff <= 0 {
			maxBackoff = time.Second
		}
		backoffMultiplier := cfg.BackoffMultiplier
		if backoffMultiplier <= 0 {
			backoffMultiplier = 2
		}
		retryableCodes := cfg.RetryableStatusCodes
		if len(retryableCodes) == 0 {
			retryableCodes = []string{"UNAVAILABLE", "DEADLINE_EXCEEDED", "UNKNOWN"}
		}

		names := make([]serviceNameEntry, 0, len(serviceNames))
		for _, serviceName := range serviceNames {
			names = append(names, serviceNameEntry{Service: serviceName})
		}

		payload.MethodConfig = []methodConfig{{
			Name:         names,
			WaitForReady: cfg.WaitForReady,
			Timeout:      formatServiceConfigDuration(timeout),
			RetryPolicy: retryPolicy{
				MaxAttempts:          maxAttempts,
				InitialBackoff:       formatServiceConfigDuration(initialBackoff),
				MaxBackoff:           formatServiceConfigDuration(maxBackoff),
				BackoffMultiplier:    backoffMultiplier,
				RetryableStatusCodes: retryableCodes,
			},
		}}
	}

	if len(payload.LoadBalancingConfig) == 0 && len(payload.MethodConfig) == 0 {
		return "", nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal grpc client service config failed: %w", err)
	}
	return string(raw), nil
}

func formatServiceConfigDuration(d time.Duration) string {
	seconds := strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
	return seconds + "s"
}
