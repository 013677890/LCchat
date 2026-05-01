package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	"github.com/013677890/LCchat-Backend/apps/user/mq"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/gorm"
)

// 这两个别名用于避免 Wire 在同为 string 的地址参数之间发生误绑定。
type userGRPCAddress string

type userMetricsAddress string

type userAsyncReleaseTimeout time.Duration

type userGRPCShutdownTimeout time.Duration

const userGRPCDefaultTimeout = 300 * time.Millisecond

func provideUserLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }
func provideUserAsyncConfig() config.AsyncConfig   { return config.DefaultAsyncConfig() }
func provideUserMySQLConfig() config.MySQLConfig   { return config.DefaultMySQLConfig() }
func provideUserKafkaConfig() config.KafkaConfig   { return config.DefaultKafkaConfig() }

func provideUserRedisConfig() config.RedisConfig {
	cfg := config.DefaultRedisConfig()
	cfg.ReadTimeout = 50 * time.Millisecond
	cfg.WriteTimeout = 50 * time.Millisecond
	return cfg
}

func provideUserLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

func provideUserAsyncPool(_ *zap.Logger, cfg config.AsyncConfig) (*ants.Pool, error) {
	return async.Build(cfg)
}

func provideUserAsyncReleaseTimeout(cfg config.AsyncConfig) userAsyncReleaseTimeout {
	return userAsyncReleaseTimeout(cfg.ReleaseTimeout)
}

func provideUserMySQLDB(_ *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	return mysql.Build(cfg)
}

// user-service 允许 Redis 缺失后降级运行，因此这里返回 nil 而不是中断整个依赖图。
func provideUserRedisClient(log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Warn(context.Background(), "Redis 初始化失败，将降级到 MySQL-Only 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	return client, nil
}

// Redis 重试链路建立在 Redis 可用前提之上，因此降级模式下不再启动 Kafka producer。
func provideUserKafkaProducer(redisClient *goredis.Client, cfg config.KafkaConfig) *kafka.Producer {
	if redisClient == nil {
		return nil
	}
	return kafka.NewProducer(cfg.Brokers, cfg.RedisRetryTopic)
}

// Consumer 只在这里构造，真正启动放到 UserApp.Run，避免 provider 隐式拉起后台 goroutine。
func provideUserRedisRetryConsumer(redisClient *goredis.Client, producer *kafka.Producer, cfg config.KafkaConfig, log *zap.Logger) *mq.RedisRetryConsumer {
	if redisClient == nil || producer == nil {
		return nil
	}
	zapLogger := kafka.NewZapLoggerAdapter(log)
	return mq.NewRedisRetryConsumer(
		cfg.Brokers,
		cfg.RedisRetryTopic,
		cfg.ConsumerConfig.GroupID,
		redisClient,
		producer,
		zapLogger,
	)
}

func provideUserGRPCAddress() userGRPCAddress {
	addr := os.Getenv("USER_GRPC_ADDR")
	if addr == "" {
		addr = ":9094"
	}
	return userGRPCAddress(addr)
}

func provideUserMetricsAddress() userMetricsAddress {
	addr := os.Getenv("USER_METRICS_ADDR")
	if addr == "" {
		addr = ":9194"
	}
	return userMetricsAddress(addr)
}

func provideUserGRPCShutdownTimeout() userGRPCShutdownTimeout {
	return userGRPCShutdownTimeout(10 * time.Second)
}

func provideUserMetricsServer(addr userMetricsAddress, built *grpcx.BuiltServer) *http.Server {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", built.Metrics.Handler())
	return &http.Server{Addr: string(addr), Handler: metricsMux}
}

func userInternalMethodWhitelist() map[string][]string {
	return map[string][]string{
		"/user.InternalProfileService/CreateProfile":         {"auth-service"},
		"/user.InternalProfileService/BatchGetUserCard":      {"gateway", "relation-service", "group-service"},
		"/user.InternalProfileService/BatchGetPublicProfile": {"gateway", "relation-service", "group-service"},
	}
}

// RegistrationFunc 把“注册哪些 gRPC 服务”从“如何创建 gRPC Server”中拆出来，
// 这样 Wire 只负责拼装依赖，运行时仍可由 App 统一编排。
func provideUserRegistration(
	userHandler *handler.UserHandler,
	internalProfileHandler *handler.InternalProfileHandler,
	groupHandler *handler.GroupHandler,
) grpcx.RegistrationFunc {
	return func(s *grpc.Server, hs healthgrpc.HealthServer) {
		userpb.RegisterUserServiceServer(s, userHandler)
		userpb.RegisterInternalProfileServiceServer(s, internalProfileHandler)
		userpb.RegisterGroupServiceServer(s, groupHandler)
		if hs != nil {
			if setter, ok := hs.(interface {
				SetServingStatus(string, healthgrpc.HealthCheckResponse_ServingStatus)
			}); ok {
				setter.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
			}
		}
	}
}

func provideUserGRPCServer(register grpcx.RegistrationFunc, addr userGRPCAddress) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Address:          string(addr),
		Namespace:        "user",
		Timeout:          &grpcx.TimeoutConfig{DefaultTimeout: userGRPCDefaultTimeout},
		EnableHealth:     true,
		EnableReflection: true,
		ExtraUnaryInterceptors: []grpc.UnaryServerInterceptor{
			grpcx.InternalCallerInterceptor(userInternalMethodWhitelist()),
		},
	}, register)
}

func provideUserGRPCListener(addr userGRPCAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

var userInfraProviderSet = wire.NewSet(
	provideUserLoggerConfig,
	provideUserAsyncConfig,
	provideUserMySQLConfig,
	provideUserRedisConfig,
	provideUserKafkaConfig,
	provideUserLogger,
	provideUserAsyncPool,
	provideUserAsyncReleaseTimeout,
	provideUserMySQLDB,
	provideUserRedisClient,
	provideUserKafkaProducer,
	provideUserRedisRetryConsumer,
	provideUserGRPCAddress,
	provideUserMetricsAddress,
	provideUserGRPCShutdownTimeout,
	provideUserMetricsServer,
	provideUserRegistration,
	provideUserGRPCServer,
	provideUserGRPCListener,
)

var userRepositoryProviderSet = wire.NewSet(
	repository.NewAuthRepository,
	repository.NewUserRepository,
	repository.NewFriendRepository,
	repository.NewApplyRepository,
	repository.NewBlacklistRepository,
	repository.NewDeviceRepository,
	repository.NewGroupRepository,
)

var userServiceProviderSet = wire.NewSet(
	service.NewAuthService,
	service.NewUserService,
	service.NewInternalProfileService,
	service.NewFriendService,
	service.NewBlacklistService,
	service.NewDeviceService,
	service.NewGroupService,
)

var userHandlerProviderSet = wire.NewSet(
	handler.NewAuthHandler,
	handler.NewUserHandler,
	handler.NewInternalProfileHandler,
	handler.NewFriendHandler,
	handler.NewBlacklistHandler,
	handler.NewDeviceHandler,
	handler.NewGroupHandler,
)

var userAppProviderSet = wire.NewSet(
	userInfraProviderSet,
	userRepositoryProviderSet,
	userServiceProviderSet,
	userHandlerProviderSet,
	NewUserApp,
)
