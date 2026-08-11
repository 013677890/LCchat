package grpcx

import "google.golang.org/grpc"

// clientUnaryInterceptorSet 列出客户端拦截器的固定角色。
// 字段顺序就是从调用方到实际 gRPC invoker 的执行顺序。
type clientUnaryInterceptorSet struct {
	metadata       grpc.UnaryClientInterceptor
	internalCaller grpc.UnaryClientInterceptor
	timeout        grpc.UnaryClientInterceptor
	logging        grpc.UnaryClientInterceptor
	observers      []grpc.UnaryClientInterceptor
	extra          []grpc.UnaryClientInterceptor
}

// buildClientUnaryInterceptors 把 ClientOptions 转换为一条完整的出站治理链。
// 它只负责选择拦截器，固定顺序集中交给 orderClientUnaryInterceptors 维护。
func buildClientUnaryInterceptors(opts ClientOptions) []grpc.UnaryClientInterceptor {
	set := clientUnaryInterceptorSet{
		metadata: MetadataUnaryClientInterceptor(),
		extra:    opts.ExtraUnaryInterceptors,
	}
	if opts.InternalCaller != nil {
		set.internalCaller = InternalCallerUnaryClientInterceptor(*opts.InternalCaller)
	}
	if opts.Timeout != nil {
		set.timeout = ClientTimeoutUnaryInterceptor(*opts.Timeout)
	}
	if opts.Logging != nil {
		set.logging = LoggingUnaryClientInterceptor(*opts.Logging)
	} else {
		set.logging = LoggingUnaryClientInterceptor()
	}
	for _, observer := range opts.Observers {
		set.observers = append(set.observers, ObserveUnaryClientInterceptor(observer))
	}
	return orderClientUnaryInterceptors(set)
}

// orderClientUnaryInterceptors 返回从调用方到实际 invoker 的执行顺序。
// internalCaller 和 timeout 是可选角色，因此 nil 会被跳过；metadata 和 logging
// 始终存在，确保每条仓库内 gRPC 连接都具有上下文传播与边界日志。
func orderClientUnaryInterceptors(set clientUnaryInterceptorSet) []grpc.UnaryClientInterceptor {
	interceptors := make([]grpc.UnaryClientInterceptor, 0, 4+len(set.observers)+len(set.extra))
	for _, interceptor := range []grpc.UnaryClientInterceptor{
		set.metadata,
		set.internalCaller,
		set.timeout,
		set.logging,
	} {
		if interceptor != nil {
			interceptors = append(interceptors, interceptor)
		}
	}
	// Observer 先于调用方扩展执行，使熔断等扩展产生的最终结果也能被观测。
	interceptors = append(interceptors, set.observers...)
	// Extra 放在最内层，接收到的是已经补齐 metadata 和 deadline 的 context。
	interceptors = append(interceptors, set.extra...)
	return interceptors
}
