package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	connectgrpc "github.com/013677890/LCchat-Backend/apps/connect/internal/grpc"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	connectserver "github.com/013677890/LCchat-Backend/apps/connect/internal/server"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/svc"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	googlegrpc "google.golang.org/grpc"
)

// ConnectApp 统一管理 connect 服务的运行与资源释放。
type ConnectApp struct {
	logger            *zap.Logger
	httpServer        *connectserver.Server
	grpcServer        *connectgrpc.Server
	connManager       *manager.ConnectionManager
	connectService    *svc.ConnectService
	userGRPCConn      *googlegrpc.ClientConn
	deviceActiveConfig config.DeviceActiveConfig
}

// NewConnectApp 只做资源聚合与合法性校验，不在构造阶段启动阻塞逻辑。
func NewConnectApp(
	log *zap.Logger,
	httpServer *connectserver.Server,
	grpcServer *connectgrpc.Server,
	connManager *manager.ConnectionManager,
	connectService *svc.ConnectService,
	userGRPCConn *googlegrpc.ClientConn,
	deviceActiveConfig config.DeviceActiveConfig,
) (*ConnectApp, error) {
	if log == nil {
		return nil, errors.New("logger 未初始化")
	}
	if httpServer == nil {
		return nil, errors.New("http server 未初始化")
	}
	if grpcServer == nil {
		return nil, errors.New("grpc server 未初始化")
	}
	if connManager == nil {
		return nil, errors.New("connection manager 未初始化")
	}
	if connectService == nil {
		return nil, errors.New("connect service 未初始化")
	}

	return &ConnectApp{
		logger:             log,
		httpServer:         httpServer,
		grpcServer:         grpcServer,
		connManager:        connManager,
		connectService:     connectService,
		userGRPCConn:       userGRPCConn,
		deviceActiveConfig: deviceActiveConfig,
	}, nil
}

// Run 负责启动 connect 服务的长生命周期组件。
func (a *ConnectApp) Run(ctx context.Context) error {
	initConnectGlobals(a.logger, a.connectService.RedisClient(), a.deviceActiveConfig)
	if err := a.connectService.InitActiveSyncer(a.deviceActiveConfig); err != nil {
		return fmt.Errorf("初始化设备活跃同步器失败: %w", err)
	}

	errCh := make(chan error, 2)

	go func() {
		logger.Info(ctx, "Connect HTTP 服务启动中", logger.String("addr", a.httpServer.Addr()))
		if err := a.httpServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("运行 HTTP 服务失败: %w", err)
		}
	}()

	go func() {
		logger.Info(ctx, "Connect gRPC 服务启动中", logger.String("addr", a.grpcServer.Addr()))
		if err := a.grpcServer.Start(); err != nil {
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
		a.grpcServer.Stop()
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
	}
	if a.userGRPCConn != nil {
		if err := a.userGRPCConn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 user-service gRPC 连接失败: %w", err))
		}
	}
	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}

func initConnectGlobals(log *zap.Logger, redis *goredis.Client, cfg config.DeviceActiveConfig) {
	logger.ReplaceGlobal(log)
	if redis != nil {
		pkgredis.ReplaceGlobal(redis)
	}
	deviceactive.SetOnlineWindow(cfg.OnlineWindow)
}
