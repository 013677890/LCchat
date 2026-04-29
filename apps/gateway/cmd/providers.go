package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/router"
	v1 "github.com/013677890/LCchat-Backend/apps/gateway/internal/router/v1"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// 使用别名类型避免 Wire 在多个 string / *grpc.ClientConn / *gobreaker.CircuitBreaker 之间误绑定。
type gatewayUserServiceAddr string

type gatewayMsgServiceAddr string

type gatewayHTTPAddr string

type gatewayAsyncReleaseTimeout time.Duration

type gatewayUserBreaker struct{ value *gobreaker.CircuitBreaker }

type gatewayMsgBreaker struct{ value *gobreaker.CircuitBreaker }

type gatewayUserConn struct{ value *grpc.ClientConn }

type gatewayMsgConn struct{ value *grpc.ClientConn }

// provideGatewayBaseContext 提供启动期基础上下文。
// 这个上下文不绑定具体请求，只用于进程启动、降级告警、后台组件初始化等场景。
func provideGatewayBaseContext() context.Context {
	return ctxmeta.WithTraceID(context.Background(), "0")
}

// provideGatewayLoggerConfig 提供日志默认配置。
func provideGatewayLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }

// provideGatewayRedisConfig 提供 Redis 默认配置。
func provideGatewayRedisConfig() config.RedisConfig { return config.DefaultRedisConfig() }

// provideGatewayAsyncConfig 提供异步协程池默认配置。
func provideGatewayAsyncConfig() config.AsyncConfig { return config.DefaultAsyncConfig() }

// provideGatewayMinIOConfig 提供对象存储默认配置。
func provideGatewayMinIOConfig() config.MinIOConfig { return config.DefaultMinIOConfig() }

// provideGatewayDeviceActiveConfig 提供设备活跃同步配置（仅配置，不触发全局副作用）。
func provideGatewayDeviceActiveConfig() config.DeviceActiveConfig {
	return config.DefaultDeviceActiveConfig()
}

// provideGatewayLogger 构建 logger（不注册全局，全局注册在 GatewayApp.Run）。
func provideGatewayLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

// provideGatewayRedisClient 初始化 Redis 客户端。
// 降级策略：
// - Redis 不可用时不阻塞 gateway 启动；
// - 限流与黑名单能力降级为“尽量放行”；
// - 这样优先保证 API 可用性，而不是把整个入口服务拖死。
func provideGatewayRedisClient(ctx context.Context, log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Error(ctx, "初始化 Redis 失败，Gateway 将降级为无 Redis 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	logger.Info(ctx, "Redis 初始化成功", logger.String("addr", cfg.Addr))
	return client, nil
}

// provideGatewayAsyncPool 构建异步协程池（全局注册在 GatewayApp.Run）。
func provideGatewayAsyncPool(_ context.Context, cfg config.AsyncConfig) (*ants.Pool, error) {
	return async.Build(cfg)
}

func provideGatewayAsyncReleaseTimeout(cfg config.AsyncConfig) gatewayAsyncReleaseTimeout {
	return gatewayAsyncReleaseTimeout(cfg.ReleaseTimeout)
}

// provideGatewayMinIOClient 初始化 MinIO 客户端。
// 降级策略：
// - MinIO 失败时不阻塞 gateway；
// - 仅影响依赖对象存储的功能，例如头像上传；
// - 其他纯转发 API 保持可用。
func provideGatewayMinIOClient(ctx context.Context, log *zap.Logger, cfg config.MinIOConfig) (*pkgminio.MinIOClient, error) {
	client, err := pkgminio.Build(cfg)
	if err != nil {
		logger.Error(ctx, "初始化 MinIO 失败，相关上传能力将降级不可用", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	logger.Info(ctx, "MinIO 初始化成功",
		logger.String("endpoint", cfg.Endpoint),
		logger.String("bucket", cfg.BucketName),
		logger.Bool("use_ssl", cfg.UseSSL),
	)
	return client, nil
}

func provideGatewayUserServiceAddr() gatewayUserServiceAddr {
	addr := os.Getenv("USER_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9090"
	}
	return gatewayUserServiceAddr(addr)
}

func provideGatewayMsgServiceAddr() gatewayMsgServiceAddr {
	addr := os.Getenv("MSG_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9092"
	}
	return gatewayMsgServiceAddr(addr)
}

func provideGatewayHTTPAddr() gatewayHTTPAddr {
	addr := os.Getenv("GATEWAY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return gatewayHTTPAddr(addr)
}

// provideGatewayUserBreaker 创建 user-service 熔断器。
func provideGatewayUserBreaker(ctx context.Context) gatewayUserBreaker {
	breaker := pb.CreateCircuitBreaker("user-service")
	logger.Info(ctx, "用户服务熔断器创建成功", logger.String("name", "user-service"))
	return gatewayUserBreaker{value: breaker}
}

// provideGatewayMsgBreaker 创建 msg-service 熔断器。
func provideGatewayMsgBreaker(ctx context.Context) gatewayMsgBreaker {
	breaker := pb.CreateCircuitBreaker("msg-service")
	logger.Info(ctx, "消息服务熔断器创建成功", logger.String("name", "msg-service"))
	return gatewayMsgBreaker{value: breaker}
}

// provideGatewayUserConn 建立到 user-service 的 gRPC 连接。
// 这里不允许降级为 nil，因为 gateway 的绝大多数核心接口都依赖 user-service。
func provideGatewayUserConn(ctx context.Context, addr gatewayUserServiceAddr, breaker gatewayUserBreaker) (gatewayUserConn, error) {
	conn, err := pb.CreateUserServiceConnection(string(addr), breaker.value)
	if err != nil {
		return gatewayUserConn{}, err
	}
	logger.Info(ctx, "用户服务 gRPC 连接创建成功", logger.String("address", string(addr)))
	return gatewayUserConn{value: conn}, nil
}

// provideGatewayMsgConn 建立到 msg-service 的 gRPC 连接。
// 这里同样视为 gateway 的必需依赖，因为消息接口已经对外暴露在同一个入口服务中。
func provideGatewayMsgConn(ctx context.Context, addr gatewayMsgServiceAddr, breaker gatewayMsgBreaker) (gatewayMsgConn, error) {
	conn, err := pb.CreateMsgServiceConnection(string(addr), breaker.value)
	if err != nil {
		return gatewayMsgConn{}, err
	}
	logger.Info(ctx, "消息服务 gRPC 连接创建成功", logger.String("address", string(addr)))
	return gatewayMsgConn{value: conn}, nil
}

// provideGatewayUserClient 聚合 user-service 多个子服务客户端。
// 这里沿用原有实现：同一条连接复用给 auth/user/friend/blacklist/device 五类调用。
func provideGatewayUserClient(conn gatewayUserConn, breaker gatewayUserBreaker) pb.UserServiceClient {
	return pb.NewUserServiceClient(conn.value, conn.value, conn.value, conn.value, conn.value, breaker.value)
}

// provideGatewayMsgClient 构造消息服务客户端。
func provideGatewayMsgClient(conn gatewayMsgConn, breaker gatewayMsgBreaker) pb.MsgServiceClient {
	return pb.NewMsgServiceClient(conn.value, breaker.value)
}

// provideGatewayRouter 统一设置 Gin 模式并构造路由树。
// 这样路由装配与 HTTP Server 构造边界清晰：
// - provider 负责拿到 *gin.Engine；
// - App 只关心如何运行 *http.Server。
func provideGatewayRouter(
	authHandler *v1.AuthHandler,
	userHandler *v1.UserHandler,
	friendHandler *v1.FriendHandler,
	blacklistHandler *v1.BlacklistHandler,
	deviceHandler *v1.DeviceHandler,
	msgHandler *v1.MsgHandler,
) http.Handler {
	return router.InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, msgHandler)
}

// provideGatewayHTTPServer 构造 HTTP Server。
// 注意这里只构造 server，不真正启动监听，保持“构造”和“运行”解耦。
func provideGatewayHTTPServer(addr gatewayHTTPAddr, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:           string(addr),
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

var gatewayInfraProviderSet = wire.NewSet(
	provideGatewayBaseContext,
	provideGatewayLoggerConfig,
	provideGatewayRedisConfig,
	provideGatewayAsyncConfig,
	provideGatewayMinIOConfig,
	provideGatewayDeviceActiveConfig,
	provideGatewayLogger,
	provideGatewayRedisClient,
	provideGatewayAsyncPool,
	provideGatewayAsyncReleaseTimeout,
	provideGatewayMinIOClient,
	provideGatewayUserServiceAddr,
	provideGatewayMsgServiceAddr,
	provideGatewayHTTPAddr,
	provideGatewayUserBreaker,
	provideGatewayMsgBreaker,
	provideGatewayUserConn,
	provideGatewayMsgConn,
	provideGatewayUserClient,
	provideGatewayMsgClient,
)

var gatewayServiceProviderSet = wire.NewSet(
	service.NewAuthService,
	service.NewUserService,
	service.NewFriendService,
	service.NewBlacklistService,
	service.NewDeviceService,
	service.NewMsgService,
)

var gatewayHandlerProviderSet = wire.NewSet(
	v1.NewAuthHandler,
	v1.NewUserHandler,
	v1.NewFriendHandler,
	v1.NewBlacklistHandler,
	v1.NewDeviceHandler,
	v1.NewMsgHandler,
)

var gatewayAppProviderSet = wire.NewSet(
	gatewayInfraProviderSet,
	gatewayServiceProviderSet,
	gatewayHandlerProviderSet,
	provideGatewayRouter,
	provideGatewayHTTPServer,
	NewGatewayApp,
)
