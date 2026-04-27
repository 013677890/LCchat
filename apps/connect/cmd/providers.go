package main

import (
	"context"
	"os"

	connectgrpc "github.com/013677890/LCchat-Backend/apps/connect/internal/grpc"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	connectserver "github.com/013677890/LCchat-Backend/apps/connect/internal/server"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/svc"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type connectUserGRPCAddress string

type connectGRPCAddress string

func provideConnectLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }

func provideConnectRedisConfig() config.RedisConfig { return config.DefaultRedisConfig() }

func provideConnectDeviceActiveConfig() config.DeviceActiveConfig {
	return config.DefaultDeviceActiveConfig()
}

func provideConnectLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

// connect 允许 Redis 缺失时降级运行，因此这里返回 nil 而不是中断整个依赖图。
func provideConnectRedisClient(log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Warn(context.Background(), "Connect 服务 Redis 初始化失败，降级为无 Redis 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	return client, nil
}

func provideConnectUserGRPCAddress() connectUserGRPCAddress {
	addr := os.Getenv("USER_GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	return connectUserGRPCAddress(addr)
}

func provideConnectGRPCAddress() connectGRPCAddress {
	addr := os.Getenv("CONNECT_GRPC_ADDR")
	if addr == "" {
		addr = ":9091"
	}
	return connectGRPCAddress(addr)
}

// user-service gRPC 连接失败时允许降级运行，因此这里返回 nil 连接和 nil 错误。
func provideUserGRPCConn(log *zap.Logger, addr connectUserGRPCAddress) (*googlegrpc.ClientConn, error) {
	conn, err := googlegrpc.NewClient(string(addr), googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Warn(context.Background(), "user-service gRPC 连接创建失败，降级为无设备状态同步模式",
			logger.String("addr", string(addr)),
			logger.ErrorField("error", err),
		)
		_ = log
		return nil, nil
	}
	return conn, nil
}

func provideUserDeviceClient(conn *googlegrpc.ClientConn) userpb.DeviceServiceClient {
	if conn == nil {
		return nil
	}
	return userpb.NewDeviceServiceClient(conn)
}

func provideConnectManager() *manager.ConnectionManager { return manager.NewConnectionManager() }

func provideConnectHTTPServer(wsHandler *handler.WSHandler, connManager *manager.ConnectionManager) *connectserver.Server {
	return connectserver.New(connectserver.DefaultConfig(), wsHandler, connManager)
}

func provideConnectGRPCServer(addr connectGRPCAddress, connManager *manager.ConnectionManager) *connectgrpc.Server {
	return connectgrpc.NewServer(string(addr), connManager)
}

var connectProviderSet = wire.NewSet(
	provideConnectLoggerConfig,
	provideConnectRedisConfig,
	provideConnectDeviceActiveConfig,
	provideConnectLogger,
	provideConnectRedisClient,
	provideConnectUserGRPCAddress,
	provideConnectGRPCAddress,
	provideUserGRPCConn,
	provideUserDeviceClient,
	provideConnectManager,
	svc.NewConnectService,
	handler.NewWSHandler,
	provideConnectHTTPServer,
	provideConnectGRPCServer,
	NewConnectApp,
)
