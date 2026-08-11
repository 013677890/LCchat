package main

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	connectgrpc "github.com/013677890/LCchat-Backend/apps/connect/internal/grpc"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	connectserver "github.com/013677890/LCchat-Backend/apps/connect/internal/server"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/svc"
	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/presence"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type connectAuthGRPCAddress string

type connectGRPCAddress string

type connectGRPCShutdownTimeout time.Duration

const connectGRPCDefaultTimeout = 300 * time.Millisecond

func provideConnectLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }

func provideConnectRedisConfig() config.RedisConfig { return config.DefaultRedisConfig() }

// connectRouteTTLFromEnv 读取路由 Key 写入 TTL（CONNECT_ROUTE_TTL_SECONDS，单位秒）。
// presence 契约：心跳无条件刷新路由，TTL 只需覆盖数个心跳周期；
// 解析失败时告警并回退到 presence.DefaultRouteTTL，避免静默吞掉配置错误。
func connectRouteTTLFromEnv() time.Duration {
	v := strings.TrimSpace(os.Getenv("CONNECT_ROUTE_TTL_SECONDS"))
	if v == "" {
		return presence.DefaultRouteTTL
	}

	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		logger.Warn(context.Background(), "CONNECT_ROUTE_TTL_SECONDS 非法，使用默认值",
			logger.String("raw", v),
			logger.String("fallback", presence.DefaultRouteTTL.String()),
		)
		return presence.DefaultRouteTTL
	}
	return time.Duration(seconds) * time.Second
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

func provideConnectAuthGRPCAddress() connectAuthGRPCAddress {
	addr := os.Getenv("AUTH_GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	return connectAuthGRPCAddress(addr)
}

func provideConnectGRPCAddress() connectGRPCAddress {
	addr := os.Getenv("CONNECT_GRPC_ADDR")
	if addr == "" {
		addr = ":9091"
	}
	return connectGRPCAddress(addr)
}

func provideConnectGRPCShutdownTimeout() connectGRPCShutdownTimeout {
	return connectGRPCShutdownTimeout(10 * time.Second)
}

func connectInternalMethodWhitelist() map[string][]string {
	return map[string][]string{
		"/connect.ConnectService/PushToDevice":     {"message-push"},
		"/connect.ConnectService/PushToUser":       {"message-push"},
		"/connect.ConnectService/BroadcastToUsers": {"message-push"},
		"/connect.ConnectService/KickConnection":   {"message-push"},
	}
}

// auth-service gRPC 连接失败时允许降级运行，因此这里返回 nil 连接和 nil 错误。
func provideAuthGRPCConn(log *zap.Logger, addr connectAuthGRPCAddress) (*grpc.ClientConn, error) {
	// connect 只调用设备状态同步写接口 UpdateDeviceStatus：
	// 幂等（设备不存在按成功处理、最后写入覆盖），因此允许配置式重试。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry: grpcx.DefaultClientRetryConfig(
			"/auth.DeviceService/UpdateDeviceStatus",
		),
	})
	if err != nil {
		logger.Warn(context.Background(), "auth-service gRPC 连接创建失败，降级为无设备状态同步模式",
			logger.String("addr", string(addr)),
			logger.ErrorField("error", err),
		)
		_ = log
		return nil, nil
	}
	return conn, nil
}

func provideAuthDeviceClient(conn *grpc.ClientConn) authpb.DeviceServiceClient {
	if conn == nil {
		return nil
	}
	return authpb.NewDeviceServiceClient(conn)
}

func provideConnectManager() *manager.ConnectionManager { return manager.NewConnectionManager() }

func provideConnectHTTPServer(wsHandler *handler.WSHandler, connManager *manager.ConnectionManager) *connectserver.Server {
	return connectserver.New(connectserver.DefaultConfig(), wsHandler, connManager)
}

func provideConnectGRPCHandler(connManager *manager.ConnectionManager) *connectgrpc.Server {
	return connectgrpc.NewServer(connManager)
}

// provideConnectRegistration 只负责“注册哪些服务”，不负责决定监听与运行方式。
func provideConnectRegistration(handler *connectgrpc.Server) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		connectpb.RegisterConnectServiceServer(s, handler)
	}
}

func provideConnectGRPCServer(register grpcx.RegistrationFunc) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Namespace: "connect",
		RateLimit: &grpcx.RateLimitConfig{
			RequestsPerSecond: 5000,
			Burst:             8000,
		},
		Logging: &grpcx.LoggingConfig{
			SlowThreshold: 200 * time.Millisecond,
			IgnoreMethods: []string{"/grpc.health.v1.Health/Check"},
		},
		Timeout:          &grpcx.TimeoutConfig{DefaultTimeout: connectGRPCDefaultTimeout},
		EnableHealth:     true,
		EnableReflection: grpcx.EnableDevelopmentReflection(),
		ExtraUnaryInterceptors: []grpc.UnaryServerInterceptor{
			// connect 的 gRPC 入口只面向内部调用方开放，
			// 第二波统一后由 grpcx 统一处理 internal-caller 鉴权。
			grpcx.InternalCallerInterceptor(connectInternalMethodWhitelist()),
		},
	}, register)
}

func provideConnectGRPCListener(addr connectGRPCAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

var connectProviderSet = wire.NewSet(
	provideConnectLoggerConfig,
	provideConnectRedisConfig,
	provideConnectLogger,
	provideConnectRedisClient,
	provideConnectAuthGRPCAddress,
	provideConnectGRPCAddress,
	provideConnectGRPCShutdownTimeout,
	provideAuthGRPCConn,
	provideAuthDeviceClient,
	provideConnectManager,
	svc.NewConnectService,
	handler.NewWSHandler,
	provideConnectHTTPServer,
	provideConnectGRPCHandler,
	provideConnectRegistration,
	provideConnectGRPCServer,
	provideConnectGRPCListener,
	NewConnectApp,
)
