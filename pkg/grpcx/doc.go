// Package grpcx 提供仓库统一的 gRPC 客户端、服务端和双端共享能力。
// 包保持单一导入路径，并通过文件名前缀表达实现边界：
//
//   - client_*.go：连接构建、出站拦截器和客户端策略；
//   - server_*.go：服务构建、入站拦截器和服务生命周期；
//   - shared_*.go：客户端和服务端确实共用的协议常量或算法。
//
// 客户端建议按 client_builder.go、client_chain.go、具体策略文件的顺序阅读；
// 服务端建议按 server_builder.go、server_chain.go、server_runtime.go 的顺序阅读。
// 对外入口分别是 NewClient 和 NewServer。Serve 只阻塞处理请求，上层 App
// 负责在关闭阶段调用 GracefulStop，因此包内不会自行监听进程退出信号。
//
// # Unary 拦截器顺序
//
// 服务端从最外层到业务 handler 的固定顺序为：Recovery、Metadata、Timeout、
// Validate、RateLimit、Metrics、ErrorNormalize、Logging、Extra。
//
// 客户端从调用方到实际 invoker 的固定顺序为：Metadata、可选 InternalCaller、
// 可选 Timeout、Logging、Observers、Extra。
//
// 新增单端能力时应放进对应前缀文件；只有两端都直接使用、并且不拥有单端策略的
// 最小实现才能进入 shared_*.go，避免通过共享文件重新形成隐式耦合。
package grpcx
