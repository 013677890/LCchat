package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/auth/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/013677890/LCchat-Backend/pkg/presence"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
	"github.com/google/wire"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// 这些别名用于避免 Wire 在同类型参数之间发生误绑定。
type authGRPCAddress string
type authMetricsAddress string
type authGRPCShutdownTimeout time.Duration

// 认证主链路包含 bcrypt、事务与验证码等重操作，服务端兜底预算需要显著高于轻量查询。
const authGRPCDefaultTimeout = 3 * time.Second

func provideAuthLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }
func provideAuthMySQLConfig() config.MySQLConfig   { return config.DefaultMySQLConfig() }
func provideAuthKafkaConfig() config.KafkaConfig   { return config.DefaultKafkaConfig() }

func provideAuthRedisConfig() config.RedisConfig {
	cfg := config.DefaultRedisConfig()
	cfg.ReadTimeout = 50 * time.Millisecond
	cfg.WriteTimeout = 50 * time.Millisecond
	return cfg
}

func provideAuthLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

func provideAuthMySQLDB(_ *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	return mysql.Build(cfg)
}

// auth-service 允许 Redis 缺失后降级运行。
func provideAuthRedisClient(log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Warn(context.Background(), "Redis 初始化失败，将降级到 MySQL-Only 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	return client, nil
}

// provideAuthPresenceRepository 构造 presence 路由读取仓储（在线状态事实源）。
// Redis 降级为 nil 时仓储会返回空路由，在线状态整体按离线降级。
func provideAuthPresenceRepository(redisClient *goredis.Client) presence.Repository {
	return presence.NewRedisRepository(redisClient, presenceOnlineWindowFromEnv())
}

// presenceOnlineWindowFromEnv 读取在线判定窗口（PRESENCE_ONLINE_WINDOW_SECONDS，单位秒）。
// 心跳无条件刷新路由后，presence 新鲜度≈客户端心跳周期（约 30s），
// 默认窗口 120s 表示连续丢约 4 个心跳判离线；解析失败时告警并回退默认值。
func presenceOnlineWindowFromEnv() time.Duration {
	v := strings.TrimSpace(os.Getenv("PRESENCE_ONLINE_WINDOW_SECONDS"))
	if v == "" {
		return presence.DefaultOnlineWindow
	}

	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		logger.Warn(context.Background(), "PRESENCE_ONLINE_WINDOW_SECONDS 非法，使用默认值",
			logger.String("raw", v),
			logger.String("fallback", presence.DefaultOnlineWindow.String()),
		)
		return presence.DefaultOnlineWindow
	}
	return time.Duration(seconds) * time.Second
}

// 缓存失效补偿建立在 Redis 可用前提之上，因此降级模式下不再启动 Kafka producer。
func provideAuthKafkaProducer(redisClient *goredis.Client, cfg config.KafkaConfig) *kafka.Producer {
	if redisClient == nil {
		return nil
	}
	return kafka.NewProducer(cfg.Brokers, cfg.AuthRedisRetryTopic)
}

// Consumer 只在这里构造，真正启动放到 AuthApp.Run，避免 provider 隐式拉起后台 goroutine。
func provideAuthRedisRetryConsumer(redisClient *goredis.Client, cfg config.KafkaConfig, log *zap.Logger, db *gorm.DB) *redisretry.RedisRetryConsumer {
	if redisClient == nil {
		return nil
	}
	return redisretry.NewRedisRetryConsumer(
		cfg.Brokers,
		cfg.AuthRedisRetryTopic,
		cfg.AuthRedisRetryGroupID,
		redisClient,
		outbox.NewDeadLetterSink(db, "auth-service:redis-invalidation"),
		kafka.NewZapLoggerAdapter(log),
	)
}

// provideAuthProfileDisplayChangedConsumer 构造 auth-service 的 profile_display_changed 消费者。
func provideAuthProfileDisplayChangedConsumer(
	cfg config.KafkaConfig,
	internalAuthSvc service.InternalAuthService,
	db *gorm.DB,
) *consumer.ProfileDisplayChangedConsumer {
	groupID := cfg.ConsumerConfig.GroupID + "-auth-profile-display-changed"
	if cfg.ConsumerConfig.GroupID == "" {
		groupID = "auth-profile-display-changed-group"
	}
	return consumer.NewProfileDisplayChangedConsumer(cfg.Brokers, cfg.ProfileDisplayChangedTopic, groupID, internalAuthSvc, db)
}

func provideAuthGRPCAddress() authGRPCAddress {
	addr := os.Getenv("AUTH_GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	return authGRPCAddress(addr)
}

func provideAuthMetricsAddress() authMetricsAddress {
	addr := os.Getenv("AUTH_METRICS_ADDR")
	if addr == "" {
		addr = ":9190"
	}
	return authMetricsAddress(addr)
}

func provideAuthGRPCShutdownTimeout() authGRPCShutdownTimeout {
	return authGRPCShutdownTimeout(10 * time.Second)
}

func provideAuthMetricsServer(addr authMetricsAddress, built *grpcx.BuiltServer) *http.Server {
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics)
}

func authInternalMethodWhitelist() map[string][]string {
	return map[string][]string{
		"/auth.InternalAuthService/FindAccountByEmail":      {"gateway", "relation-service"},
		"/auth.InternalAuthService/FindAccountByTelephone":  {"gateway"},
		"/auth.InternalAuthService/UpdateLoginDisplay":      {"user-service"},
		"/auth.InternalAuthService/BatchCheckAccountStatus": {"relation-service", "group-service"},
	}
}

// provideAuthRegistration 注册 auth-service 暴露的全部 gRPC 服务。
func provideAuthRegistration(
	authHandler *handler.AuthHandler,
	deviceHandler *handler.DeviceHandler,
	accountHandler *handler.AccountHandler,
	internalAuthHandler *handler.InternalAuthHandler,
) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		authpb.RegisterAuthServiceServer(s, authHandler)
		authpb.RegisterDeviceServiceServer(s, deviceHandler)
		authpb.RegisterAccountServiceServer(s, accountHandler)
		authpb.RegisterInternalAuthServiceServer(s, internalAuthHandler)
	}
}

// provideAuthGRPCServer 构建 auth-service 的 gRPC Server。
func provideAuthGRPCServer(register grpcx.RegistrationFunc, addr authGRPCAddress) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Address:          string(addr),
		Namespace:        "auth",
		Timeout:          &grpcx.TimeoutConfig{DefaultTimeout: authGRPCDefaultTimeout},
		EnableHealth:     true,
		EnableReflection: grpcx.EnableDevelopmentReflection(),
		ExtraUnaryInterceptors: []grpc.UnaryServerInterceptor{
			grpcx.InternalCallerInterceptor(authInternalMethodWhitelist()),
		},
	}, register)
}

func provideAuthGRPCListener(addr authGRPCAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

var authInfraProviderSet = wire.NewSet(
	provideAuthLoggerConfig,
	provideAuthMySQLConfig,
	provideAuthRedisConfig,
	provideAuthKafkaConfig,
	provideAuthLogger,
	provideAuthMySQLDB,
	provideAuthRedisClient,
	provideAuthKafkaProducer,
	provideAuthRedisRetryConsumer,
	provideAuthGRPCAddress,
	provideAuthMetricsAddress,
	provideAuthGRPCShutdownTimeout,
	provideAuthMetricsServer,
	provideAuthRegistration,
	provideAuthGRPCServer,
	provideAuthGRPCListener,
)

var authRepositoryProviderSet = wire.NewSet(
	repository.NewAuthRepository,
	repository.NewDeviceRepository,
	provideAuthPresenceRepository,
)

var authServiceProviderSet = wire.NewSet(
	service.NewAuthService,
	service.NewDeviceService,
	service.NewAccountService,
	service.NewInternalAuthService,
)

var authHandlerProviderSet = wire.NewSet(
	handler.NewAuthHandler,
	handler.NewDeviceHandler,
	handler.NewAccountHandler,
	handler.NewInternalAuthHandler,
)

var authAppProviderSet = wire.NewSet(
	authInfraProviderSet,
	authRepositoryProviderSet,
	authServiceProviderSet,
	provideAuthProfileDisplayChangedConsumer,
	authHandlerProviderSet,
	NewAuthApp,
)
