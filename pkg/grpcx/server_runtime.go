package grpcx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/grpc"
)

// NewListener 创建 gRPC TCP 监听器。
// 监听动作与 NewServer 分离，保证依赖注入阶段可以独立构建 Server 而不占用端口。
func NewListener(address string) (net.Listener, error) {
	if address == "" {
		return nil, errors.New("grpc listen address is empty")
	}
	return net.Listen("tcp", address)
}

// Serve 在 listener 上阻塞运行 server。
//
// Serve 不监听 context，也不主动停止 server；服务的关闭时机统一由上层 App
// 调用 GracefulStop 控制，避免运行路径和关闭路径同时争抢生命周期所有权。
func Serve(server *grpc.Server, listener net.Listener) error {
	if server == nil {
		return errors.New("grpc server is nil")
	}
	if listener == nil {
		return errors.New("grpc listener is nil")
	}

	logger.Info(context.Background(), "gRPC 服务启动", logger.String("addr", listener.Addr().String()))
	if err := server.Serve(listener); err != nil {
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("grpc serve failed: %w", err)
	}
	return nil
}

// GracefulStop 负责优雅停止 gRPC Server。
// timeout 是 gRPC 自身的停服预算，ctx 则服从上层 App 的整体关闭预算；任一先到期
// 都会退化为强制 Stop，避免某个长请求永久阻塞整个进程退出。
func GracefulStop(ctx context.Context, server *grpc.Server, timeout time.Duration) error {
	if server == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	stopDone := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopDone)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-stopDone:
		logger.Info(ctx, "gRPC 服务已优雅停止")
		return nil
	case <-ctx.Done():
		logger.Warn(ctx, "gRPC 优雅停机上下文已取消，执行强制停止", logger.ErrorField("error", ctx.Err()))
		server.Stop()
		return ctx.Err()
	case <-timer.C:
		logger.Warn(ctx, "gRPC 优雅停机超时，执行强制停止", logger.Duration("timeout", timeout))
		server.Stop()
		return context.DeadlineExceeded
	}
}
