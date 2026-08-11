package grpcx

import "google.golang.org/grpc"

// serverUnaryInterceptorSet 列出服务端内置拦截器的固定角色。
// 字段顺序就是从最外层到业务 handler 的执行顺序。
type serverUnaryInterceptorSet struct {
	recovery       grpc.UnaryServerInterceptor
	metadata       grpc.UnaryServerInterceptor
	timeout        grpc.UnaryServerInterceptor
	validate       grpc.UnaryServerInterceptor
	rateLimit      grpc.UnaryServerInterceptor
	metrics        grpc.UnaryServerInterceptor
	errorNormalize grpc.UnaryServerInterceptor
	logging        grpc.UnaryServerInterceptor
}

// buildServerUnaryInterceptors 解析 ServerOptions 中的可选配置并构造内置拦截器。
// 默认值只在这里收敛，使 NewServer 保持“创建资源并注册服务”的单一职责。
func buildServerUnaryInterceptors(opts ServerOptions, metrics *Metrics) []grpc.UnaryServerInterceptor {
	rateLimitCfg := DefaultRateLimitConfig()
	if opts.RateLimit != nil {
		rateLimitCfg = *opts.RateLimit
	}

	loggingCfg := DefaultLoggingConfig()
	if opts.Logging != nil {
		loggingCfg = *opts.Logging
	}

	var timeoutCfg TimeoutConfig
	if opts.Timeout != nil {
		timeoutCfg = *opts.Timeout
	}

	return orderServerUnaryInterceptors(serverUnaryInterceptorSet{
		recovery:       RecoveryUnaryInterceptor(),
		metadata:       MetadataUnaryInterceptor(),
		timeout:        TimeoutUnaryInterceptor(timeoutCfg),
		validate:       ValidateUnaryInterceptor(),
		rateLimit:      RateLimitUnaryInterceptor(rateLimitCfg),
		metrics:        metrics.UnaryInterceptor(),
		errorNormalize: ErrorNormalizeUnaryInterceptor(),
		logging:        LoggingUnaryInterceptor(loggingCfg),
	}, opts.ExtraUnaryInterceptors)
}

// orderServerUnaryInterceptors 返回从网络入口到业务 handler 的执行顺序。
// 返回阶段按相反方向展开，因此 Metrics 能看到归一化后的状态码，Recovery 则能兜住
// 包括自定义 Extra 在内的整条链路。
func orderServerUnaryInterceptors(
	set serverUnaryInterceptorSet,
	extra []grpc.UnaryServerInterceptor,
) []grpc.UnaryServerInterceptor {
	interceptors := []grpc.UnaryServerInterceptor{
		// Recovery 必须位于最外层，任何后续拦截器 panic 都能转换为受控错误。
		set.recovery,
		// Metadata 先补齐业务上下文，Timeout 再让后续阶段共享同一个 deadline。
		set.metadata,
		// timeout 要尽早生效，让 validate、rate-limit、metrics 和业务处理
		// 看到同一个被收紧后的请求级 deadline。
		set.timeout,
		set.validate,
		set.rateLimit,
		// Metrics 包住错误归一化阶段，因此记录的是最终对外 gRPC code。
		set.metrics,
		// Logging 先接触 handler 错误并补齐日志信息，ErrorNormalize 随后脱敏出站错误。
		set.errorNormalize,
		set.logging,
	}
	return append(interceptors, extra...)
}
