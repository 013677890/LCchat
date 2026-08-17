package main

import (
	"context"
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
	"net/http"
	"os"
	"time"
)

// 使用别名类型避免 Wire 在多个 string / *grpc.ClientConn / *gobreaker.CircuitBreaker 之间误绑定。
type gatewayAuthServiceAddr string
type gatewayUserServiceAddr string
type gatewayRelationServiceAddr string
type gatewayMsgServiceAddr string
type gatewayGroupServiceAddr string
type gatewayHTTPAddr string
type gatewayAsyncReleaseTimeout time.Duration
type gatewayAuthBreaker struct{ value *gobreaker.CircuitBreaker }
type gatewayUserBreaker struct{ value *gobreaker.CircuitBreaker }
type gatewayRelationBreaker struct{ value *gobreaker.CircuitBreaker }
type gatewayMsgBreaker struct{ value *gobreaker.CircuitBreaker }
type gatewayGroupBreaker struct{ value *gobreaker.CircuitBreaker }
type gatewayAuthConn struct{ value *grpc.ClientConn }
type gatewayUserConn struct{ value *grpc.ClientConn }
type gatewayRelationConn struct{ value *grpc.ClientConn }
type gatewayMsgConn struct{ value *grpc.ClientConn }
type gatewayGroupConn struct{ value *grpc.ClientConn }

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
	const (
		maxAttempts = 5
		retryDelay  = 1500 * time.Millisecond
	)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		client, err := pkgminio.Build(cfg)
		if err == nil {
			// Gateway 启动早于对象存储完全就绪时，重试后的成功也要明确记录下来，便于排查冷启动问题。
			log.Info("MinIO 初始化成功",
				zap.String("endpoint", cfg.Endpoint),
				zap.String("bucket", cfg.BucketName),
				zap.Bool("use_ssl", cfg.UseSSL),
				zap.Int("attempt", attempt),
			)
			return client, nil
		}

		lastErr = err
		if attempt < maxAttempts {
			log.Warn("初始化 MinIO 失败，稍后重试",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", maxAttempts),
				zap.Duration("retry_delay", retryDelay),
				zap.Error(err),
			)
			time.Sleep(retryDelay)
			continue
		}
	}

	log.Error("初始化 MinIO 失败，相关上传能力将降级不可用",
		zap.Int("attempts", maxAttempts),
		zap.Error(lastErr),
	)
	_ = ctx
	return nil, nil
}

// provideGatewayAuthServiceAddr 提供 auth-service 地址。
func provideGatewayAuthServiceAddr() gatewayAuthServiceAddr {
	addr := os.Getenv("AUTH_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9090"
	}
	return gatewayAuthServiceAddr(addr)
}

// provideGatewayUserServiceAddr 提供 user-service 地址。
func provideGatewayUserServiceAddr() gatewayUserServiceAddr {
	addr := os.Getenv("USER_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9094"
	}
	return gatewayUserServiceAddr(addr)
}

// provideGatewayRelationServiceAddr 提供 relation-service 地址。
// 这里将好友/黑名单两个子域统一指向同一个 relation-service 入口。
func provideGatewayRelationServiceAddr() gatewayRelationServiceAddr {
	addr := os.Getenv("RELATION_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9093"
	}
	return gatewayRelationServiceAddr(addr)
}
func provideGatewayMsgServiceAddr() gatewayMsgServiceAddr {
	addr := os.Getenv("MSG_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9092"
	}
	return gatewayMsgServiceAddr(addr)
}
func provideGatewayGroupServiceAddr() gatewayGroupServiceAddr {
	addr := os.Getenv("GROUP_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9095"
	}
	return gatewayGroupServiceAddr(addr)
}
func provideGatewayHTTPAddr() gatewayHTTPAddr {
	addr := os.Getenv("GATEWAY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return gatewayHTTPAddr(addr)
}

// provideGatewayAuthBreaker 创建 auth-service 熔断器。
func provideGatewayAuthBreaker(ctx context.Context) gatewayAuthBreaker {
	breaker := pb.CreateCircuitBreaker("auth-service")
	logger.Info(ctx, "认证服务熔断器创建成功", logger.String("name", "auth-service"))
	return gatewayAuthBreaker{value: breaker}
}

// provideGatewayUserBreaker 创建 user-service 熔断器。
func provideGatewayUserBreaker(ctx context.Context) gatewayUserBreaker {
	breaker := pb.CreateCircuitBreaker("user-service")
	logger.Info(ctx, "用户服务熔断器创建成功", logger.String("name", "user-service"))
	return gatewayUserBreaker{value: breaker}
}

// provideGatewayRelationBreaker 创建 relation-service 熔断器。
func provideGatewayRelationBreaker(ctx context.Context) gatewayRelationBreaker {
	breaker := pb.CreateCircuitBreaker("relation-service")
	logger.Info(ctx, "关系服务熔断器创建成功", logger.String("name", "relation-service"))
	return gatewayRelationBreaker{value: breaker}
}

// provideGatewayMsgBreaker 创建 msg-service 熔断器。
func provideGatewayMsgBreaker(ctx context.Context) gatewayMsgBreaker {
	breaker := pb.CreateCircuitBreaker("msg-service")
	logger.Info(ctx, "消息服务熔断器创建成功", logger.String("name", "msg-service"))
	return gatewayMsgBreaker{value: breaker}
}

// provideGatewayGroupBreaker 创建 group-service 熔断器。
func provideGatewayGroupBreaker(ctx context.Context) gatewayGroupBreaker {
	breaker := pb.CreateCircuitBreaker("group-service")
	logger.Info(ctx, "群组服务熔断器创建成功", logger.String("name", "group-service"))
	return gatewayGroupBreaker{value: breaker}
}

// provideGatewayAuthConn 建立到 auth-service 的 gRPC 连接。
// 认证、账号安全、设备会话与内部账号查询都依赖该连接，因此不允许降级为 nil。
func provideGatewayAuthConn(ctx context.Context, addr gatewayAuthServiceAddr, breaker gatewayAuthBreaker) (gatewayAuthConn, error) {
	conn, err := pb.CreateAuthServiceConnection(string(addr), breaker.value)
	if err != nil {
		return gatewayAuthConn{}, err
	}
	logger.Info(ctx, "认证服务 gRPC 连接创建成功", logger.String("address", string(addr)))
	return gatewayAuthConn{value: conn}, nil
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

// provideGatewayRelationConn 建立到 relation-service 的 gRPC 连接。
// 好友/黑名单路由已经切分出去，因此这里也视为 gateway 的必需依赖。
func provideGatewayRelationConn(ctx context.Context, addr gatewayRelationServiceAddr, breaker gatewayRelationBreaker) (gatewayRelationConn, error) {
	conn, err := pb.CreateRelationServiceConnection(string(addr), breaker.value)
	if err != nil {
		return gatewayRelationConn{}, err
	}
	logger.Info(ctx, "关系服务 gRPC 连接创建成功", logger.String("address", string(addr)))
	return gatewayRelationConn{value: conn}, nil
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

// provideGatewayGroupConn 建立到 group-service 的 gRPC 连接。
// 群组 HTTP 接口已经在 gateway 暴露，因此这里作为必需依赖处理。
func provideGatewayGroupConn(ctx context.Context, addr gatewayGroupServiceAddr, breaker gatewayGroupBreaker) (gatewayGroupConn, error) {
	conn, err := pb.CreateGroupServiceConnection(string(addr), breaker.value)
	if err != nil {
		return gatewayGroupConn{}, err
	}
	logger.Info(ctx, "群组服务 gRPC 连接创建成功", logger.String("address", string(addr)))
	return gatewayGroupConn{value: conn}, nil
}

// provideGatewayUserClient 聚合 gateway 对多个下游服务的客户端。
// 当前拆分策略：
// - auth/account/internalAuth/device 走 auth-service；
// - profile/search/qrcode 走 user-service；
// - friend/blacklist 走 relation-service。
func provideGatewayUserClient(
	authConn gatewayAuthConn,
	userConn gatewayUserConn,
	relationConn gatewayRelationConn,
) pb.UserServiceClient {
	return pb.NewUserServiceClient(
		authConn.value,
		userConn.value,
		relationConn.value,
		relationConn.value,
		authConn.value,
	)
}

// provideGatewayMsgClient 构造消息服务客户端。
func provideGatewayMsgClient(conn gatewayMsgConn) pb.MsgServiceClient {
	return pb.NewMsgServiceClient(conn.value)
}

// provideGatewayGroupClient 构造群组服务客户端。
func provideGatewayGroupClient(conn gatewayGroupConn) pb.GroupServiceClient {
	return pb.NewGroupServiceClient(conn.value)
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
	groupHandler *v1.GroupHandler,
) http.Handler {
	return router.InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, msgHandler, groupHandler)
}

// provideGatewayHTTPServer 构造 HTTP Server。
// 注意这里只构造 server，不真正启动监听，保持“构造”和“运行”解耦。
// WriteTimeout 要覆盖标准 CPU pprof 默认的 30 秒采样时间。
func provideGatewayHTTPServer(addr gatewayHTTPAddr, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:           string(addr),
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   35 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

var gatewayInfraProviderSet = wire.NewSet(
	provideGatewayBaseContext,
	provideGatewayLoggerConfig,
	provideGatewayRedisConfig,
	provideGatewayAsyncConfig,
	provideGatewayMinIOConfig,
	provideGatewayLogger,
	provideGatewayRedisClient,
	provideGatewayAsyncPool,
	provideGatewayAsyncReleaseTimeout,
	provideGatewayMinIOClient,
	provideGatewayAuthServiceAddr,
	provideGatewayUserServiceAddr,
	provideGatewayRelationServiceAddr,
	provideGatewayMsgServiceAddr,
	provideGatewayGroupServiceAddr,
	provideGatewayHTTPAddr,
	provideGatewayAuthBreaker,
	provideGatewayUserBreaker,
	provideGatewayRelationBreaker,
	provideGatewayMsgBreaker,
	provideGatewayGroupBreaker,
	provideGatewayAuthConn,
	provideGatewayUserConn,
	provideGatewayRelationConn,
	provideGatewayMsgConn,
	provideGatewayGroupConn,
	provideGatewayUserClient,
	provideGatewayMsgClient,
	provideGatewayGroupClient,
)
var gatewayServiceProviderSet = wire.NewSet(
	service.NewAuthService,
	service.NewUserService,
	service.NewFriendService,
	service.NewBlacklistService,
	service.NewDeviceService,
	service.NewMsgService,
	service.NewGroupService,
)
var gatewayHandlerProviderSet = wire.NewSet(
	v1.NewAuthHandler,
	v1.NewUserHandler,
	v1.NewFriendHandler,
	v1.NewBlacklistHandler,
	v1.NewDeviceHandler,
	v1.NewMsgHandler,
	v1.NewGroupHandler,
)
var gatewayAppProviderSet = wire.NewSet(
	gatewayInfraProviderSet,
	gatewayServiceProviderSet,
	gatewayHandlerProviderSet,
	provideGatewayRouter,
	provideGatewayHTTPServer,
	NewGatewayApp,
)
