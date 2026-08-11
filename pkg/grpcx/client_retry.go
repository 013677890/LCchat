package grpcx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "google.golang.org/grpc/balancer/roundrobin"
)

// round_robin 让同一个 DNS target 解析出的多个后端实例共同承载请求。
const defaultClientLoadBalancingPolicy = "round_robin"

// ClientRetryConfig 定义 gRPC ServiceConfig 中的方法级重试策略。
//
// 原则：默认不重试；只有 Methods 中显式列出的 full method 才会写入 retryPolicy。
// Methods 必须是完整形式 `/package.Service/Method`（例如 `/group.GroupService/GetGroupInfo`），
// 禁止只填 service 名、禁止运行时按 Get/List 前缀推断、禁止静默降级为 Service 级重试。
type ClientRetryConfig struct {
	// Methods 可重试 full method 白名单。空列表表示不启用配置式重试。
	Methods []string

	// Timeout 写入 methodConfig.timeout，仅作用于 Methods 中的方法。
	// 为 0 时不写入该字段，由 ClientTimeout 拦截器与父 context 负责 deadline。
	Timeout time.Duration

	// WaitForReady 允许 RPC 等待连接恢复，而不是在短暂不可用时立即失败。
	WaitForReady bool
	// MaxAttempts 包含首次调用；gRPC 规范要求配置式重试至少为 2。
	MaxAttempts int
	// InitialBackoff、MaxBackoff 和 BackoffMultiplier 共同定义指数退避。
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	// RetryableStatusCodes 只应包含基础设施瞬态错误，业务错误不得放入。
	RetryableStatusCodes []string
}

// DefaultClientRetryConfig 返回仓库统一的默认重试骨架，并绑定到指定 full method 白名单。
// 调用方拿到的是一份可修改副本，可在不同连接上覆写 attempts / codes 等。
// fullMethods 的合法性在 NewClient / buildClientServiceConfig 时严格校验；非法配置直接报错。
func DefaultClientRetryConfig(fullMethods ...string) *ClientRetryConfig {
	methods := make([]string, 0, len(fullMethods))
	methods = append(methods, fullMethods...)
	return &ClientRetryConfig{
		Methods:              methods,
		Timeout:              0,
		WaitForReady:         true,
		MaxAttempts:          5,
		InitialBackoff:       100 * time.Millisecond,
		MaxBackoff:           time.Second,
		BackoffMultiplier:    2,
		RetryableStatusCodes: []string{"UNAVAILABLE", "DEADLINE_EXCEEDED", "UNKNOWN"},
	}
}

// normalizeClientLoadBalancingPolicy 把空配置收敛为仓库默认策略。
func normalizeClientLoadBalancingPolicy(policy string) string {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return defaultClientLoadBalancingPolicy
	}
	return policy
}

// parsedRetryMethod 是校验通过后的 full method 拆分结果。
type parsedRetryMethod struct {
	FullMethod string
	Service    string
	Method     string
}

// parseRetryFullMethod 严格解析 `/package.Service/Method`。
// service 与 method 任一为空、格式不符或重复斜杠均返回错误，禁止静默降级。
func parseRetryFullMethod(fullMethod string) (parsedRetryMethod, error) {
	fullMethod = strings.TrimSpace(fullMethod)
	if fullMethod == "" {
		return parsedRetryMethod{}, fmt.Errorf("grpc retry full method 不能为空")
	}
	if !strings.HasPrefix(fullMethod, "/") {
		return parsedRetryMethod{}, fmt.Errorf("grpc retry full method 必须以 / 开头: %q", fullMethod)
	}

	// 去掉前导 / 后必须恰好拆成 service 与 method 两段；段内禁止空白，避免静默规范化掩盖配置错误。
	body := fullMethod[1:]
	parts := strings.Split(body, "/")
	if len(parts) != 2 {
		return parsedRetryMethod{}, fmt.Errorf("grpc retry full method 必须是 /package.Service/Method: %q", fullMethod)
	}
	service := parts[0]
	method := parts[1]
	if service == "" || method == "" {
		return parsedRetryMethod{}, fmt.Errorf("grpc retry full method 的 service 与 method 均不能为空: %q", fullMethod)
	}
	if strings.TrimSpace(service) != service || strings.TrimSpace(method) != method ||
		strings.ContainsAny(service, " \t\r\n") || strings.ContainsAny(method, " \t\r\n") {
		return parsedRetryMethod{}, fmt.Errorf("grpc retry full method 不能包含空白: %q", fullMethod)
	}
	normalized := "/" + service + "/" + method
	return parsedRetryMethod{
		FullMethod: normalized,
		Service:    service,
		Method:     method,
	}, nil
}

// normalizeRetryMethods 校验并去重 full method 列表；重复项直接拒绝。
func normalizeRetryMethods(methods []string) ([]parsedRetryMethod, error) {
	if len(methods) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(methods))
	out := make([]parsedRetryMethod, 0, len(methods))
	for _, raw := range methods {
		parsed, err := parseRetryFullMethod(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[parsed.FullMethod]; ok {
			return nil, fmt.Errorf("grpc retry full method 重复: %s", parsed.FullMethod)
		}
		seen[parsed.FullMethod] = struct{}{}
		out = append(out, parsed)
	}
	return out, nil
}

// buildClientServiceConfig 同时生成负载均衡和方法级重试配置。
// 即使 cfg 为 nil，也会保留负载均衡部分；只有白名单非空时才写入 retryPolicy，
// 防止一次“关闭重试”的配置调整意外退化为 pick_first。
func buildClientServiceConfig(cfg *ClientRetryConfig, loadBalancingPolicy string) (string, error) {
	loadBalancingPolicy = normalizeClientLoadBalancingPolicy(loadBalancingPolicy)

	var parsedMethods []parsedRetryMethod
	if cfg != nil {
		var err error
		parsedMethods, err = normalizeRetryMethods(cfg.Methods)
		if err != nil {
			return "", err
		}
	}

	type methodNameEntry struct {
		Service string `json:"service"`
		Method  string `json:"method"`
	}
	type retryPolicy struct {
		MaxAttempts          int      `json:"maxAttempts"`
		InitialBackoff       string   `json:"initialBackoff"`
		MaxBackoff           string   `json:"maxBackoff"`
		BackoffMultiplier    float64  `json:"backoffMultiplier"`
		RetryableStatusCodes []string `json:"retryableStatusCodes"`
	}
	type methodConfig struct {
		Name         []methodNameEntry `json:"name"`
		WaitForReady bool              `json:"waitForReady,omitempty"`
		Timeout      string            `json:"timeout,omitempty"`
		RetryPolicy  *retryPolicy      `json:"retryPolicy,omitempty"`
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

	if cfg != nil && len(parsedMethods) > 0 {
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

		// 每个 full method 单独一条 name 项，强制 service+method 精确匹配。
		names := make([]methodNameEntry, 0, len(parsedMethods))
		for _, method := range parsedMethods {
			names = append(names, methodNameEntry{
				Service: method.Service,
				Method:  method.Method,
			})
		}

		mc := methodConfig{
			Name:         names,
			WaitForReady: cfg.WaitForReady,
			RetryPolicy: &retryPolicy{
				MaxAttempts:          maxAttempts,
				InitialBackoff:       formatServiceConfigDuration(initialBackoff),
				MaxBackoff:           formatServiceConfigDuration(maxBackoff),
				BackoffMultiplier:    backoffMultiplier,
				RetryableStatusCodes: retryableCodes,
			},
		}

		if cfg.Timeout > 0 {
			mc.Timeout = formatServiceConfigDuration(cfg.Timeout)
		}
		payload.MethodConfig = []methodConfig{mc}
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

// formatServiceConfigDuration 使用 gRPC ServiceConfig 接受的秒数字符串格式。
func formatServiceConfigDuration(duration time.Duration) string {
	seconds := strconv.FormatFloat(duration.Seconds(), 'f', -1, 64)
	return seconds + "s"
}
