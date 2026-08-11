package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	connectserver "github.com/013677890/LCchat-Backend/apps/connect/internal/server"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/svc"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// ConnectApp 统一管理 connect 服务的运行与资源释放。
type ConnectApp struct {
	logger              *zap.Logger
	httpServer          *connectserver.Server
	grpcServer          *grpc.Server
	grpcListener        net.Listener
	grpcShutdownTimeout time.Duration
	connManager         *manager.ConnectionManager
	connectService      *svc.ConnectService
	authGRPCConn        *grpc.ClientConn
}

// NewConnectApp 只做资源聚合与合法性校验，不在构造阶段启动阻塞逻辑。
func NewConnectApp(
	log *zap.Logger,
	httpServer *connectserver.Server,
	built *grpcx.BuiltServer,
	grpcListener net.Listener,
	grpcShutdownTimeout connectGRPCShutdownTimeout,
	connManager *manager.ConnectionManager,
	connectService *svc.ConnectService,
	authGRPCConn *grpc.ClientConn,
) (*ConnectApp, error) {
	if log == nil {
		return nil, errors.New("logger 未初始化")
	}
	if httpServer == nil {
		return nil, errors.New("http server 未初始化")
	}
	if built == nil || built.Server == nil {
		return nil, errors.New("grpc server 未初始化")
	}
	if grpcListener == nil {
		return nil, errors.New("grpc listener 未初始化")
	}
	if connManager == nil {
		return nil, errors.New("connection manager 未初始化")
	}
	if connectService == nil {
		return nil, errors.New("connect service 未初始化")
	}

	return &ConnectApp{
		logger:              log,
		httpServer:          httpServer,
		grpcServer:          built.Server,
		grpcListener:        grpcListener,
		grpcShutdownTimeout: time.Duration(grpcShutdownTimeout),
		connManager:         connManager,
		connectService:      connectService,
		authGRPCConn:        authGRPCConn,
	}, nil
}

// Run 负责启动 connect 服务的长生命周期组件。
func (a *ConnectApp) Run(ctx context.Context) error {
	initConnectGlobals(a.logger, a.connectService.RedisClient())
	if os.Getenv("CONNECT_SELF_GRPC_ADDR") == "" {
		return errors.New("CONNECT_SELF_GRPC_ADDR 未配置")
	}
	// 路由 Key 写入 TTL 属于 presence 契约参数，在对外提供服务前注入。
	a.connectService.SetRouteTTL(connectRouteTTLFromEnv())

	errCh := make(chan error, 2)

	go func() {
		logger.Info(ctx, "Connect HTTP 服务启动中", logger.String("addr", a.httpServer.Addr()))
		if err := a.httpServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("运行 HTTP 服务失败: %w", err)
		}
	}()

	go func() {
		logger.Info(ctx, "Connect gRPC 服务启动中", logger.String("addr", a.grpcListener.Addr().String()))
		if err := grpcx.Serve(a.grpcServer, a.grpcListener); err != nil {
			errCh <- fmt.Errorf("运行 gRPC 服务失败: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// Shutdown 按“先停入口，再停连接与后台任务，最后收资源”的顺序关闭服务。
func (a *ConnectApp) Shutdown(ctx context.Context) error {
	var errs []error

	if a.grpcServer != nil {
		if err := grpcx.GracefulStop(ctx, a.grpcServer, a.grpcShutdownTimeout); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			errs = append(errs, fmt.Errorf("关闭 gRPC 服务失败: %w", err))
		}
	}

	if a.grpcListener != nil {
		if err := a.grpcListener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, fmt.Errorf("关闭 gRPC 监听器失败: %w", err))
		}
	}

	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("关闭 HTTP 服务失败: %w", err))
		}
	}

	if a.connManager != nil {
		a.connManager.Shutdown()
	}
	if a.connectService != nil {
		a.connectService.ShutdownStatusWorkers()
		a.connectService.RemoveRoutesByConnectAddr(ctx, os.Getenv("CONNECT_SELF_GRPC_ADDR"))
	}
	if a.authGRPCConn != nil {
		if err := a.authGRPCConn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 auth-service gRPC 连接失败: %w", err))
		}
	}

	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}

func initConnectGlobals(log *zap.Logger, redis *goredis.Client) {
	logger.ReplaceGlobal(log)
	if redis != nil {
		pkgredis.ReplaceGlobal(redis)
	}
}
