package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/user/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
	"github.com/google/wire"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
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

// 缓存失效补偿建立在 Redis 可用前提之上，因此降级模式下不再启动 Kafka producer。
func provideUserKafkaProducer(redisClient *goredis.Client, cfg config.KafkaConfig) *kafka.Producer {
	if redisClient == nil {
		return nil
	}
	return kafka.NewProducer(cfg.Brokers, cfg.UserRedisRetryTopic)
}

// parseUserPoolWorkers 严格解析 user-service 旁路消费 Pool 的 Reader 数。
// 空值统一默认 3；显式非法值带环境变量名返回，阻止错误配置进入运行态。
func parseUserPoolWorkers(envName string) (int, error) {
	workers, err := kafka.ParsePoolWorkers(os.Getenv(envName))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", envName, err)
	}
	return workers, nil
}

// Consumer 只在这里构造，真正启动放到 UserApp.Run，避免 provider 隐式拉起后台 goroutine。
func provideUserRedisRetryConsumer(redisClient *goredis.Client, cfg config.KafkaConfig, log *zap.Logger, db *gorm.DB) (*redisretry.RedisRetryConsumer, error) {
	workers, err := parseUserPoolWorkers("KAFKA_USER_REDIS_RETRY_CONSUMER_CONCURRENCY")
	if err != nil {
		return nil, err
	}
	if redisClient == nil {
		return nil, nil
	}
	zapLogger := kafka.NewZapLoggerAdapter(log)
	return redisretry.NewRedisRetryConsumer(
		cfg.Brokers,
		cfg.UserRedisRetryTopic,
		cfg.UserRedisRetryGroupID,
		workers,
		redisClient,
		outbox.NewDeadLetterSink(db, "user-service:redis-invalidation"),
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

// provideUserGRPCMethodTimeouts 为 user-service gRPC 方法提供更细粒度的超时预算。
// 资料更新需要写主表并触发展示字段事件，比只读查询更重，因此单独放宽。
func provideUserGRPCMethodTimeouts() map[string]time.Duration {
	return map[string]time.Duration{
		"/user.UserService/UpdateProfile": 2 * time.Second,
	}
}

// provideUserUserCreatedConsumer 构造 user-service 的 user_created 消费者。
func provideUserUserCreatedConsumer(cfg config.KafkaConfig, internalProfileSvc service.InternalProfileService, db *gorm.DB) (*consumer.UserCreatedConsumer, error) {
	workers, err := parseUserPoolWorkers("KAFKA_USER_CREATED_CONSUMER_CONCURRENCY")
	if err != nil {
		return nil, err
	}
	groupID := cfg.ConsumerConfig.GroupID + "-user-created"
	if cfg.ConsumerConfig.GroupID == "" {
		groupID = "user-created-group"
	}
	return consumer.NewUserCreatedConsumer(cfg.Brokers, cfg.UserCreatedTopic, groupID, workers, internalProfileSvc, db)
}

// provideUserAccountDeletedConsumer 构造 user-service 的 account.deleted 消费者。
func provideUserAccountDeletedConsumer(cfg config.KafkaConfig, userRepo repository.IUserRepository, db *gorm.DB) (*consumer.AccountDeletedConsumer, error) {
	workers, err := parseUserPoolWorkers("KAFKA_USER_ACCOUNT_DELETED_CONSUMER_CONCURRENCY")
	if err != nil {
		return nil, err
	}
	groupID := cfg.ConsumerConfig.GroupID + "-user-account-deleted"
	if cfg.ConsumerConfig.GroupID == "" {
		groupID = "user-account-deleted-group"
	}
	return consumer.NewAccountDeletedConsumer(cfg.Brokers, cfg.AccountDeletedTopic, groupID, workers, userRepo, db)
}

// provideUserMetricsServer 注册 user 的 gRPC 与旁路 Kafka Pool 指标并创建 HTTP 服务。
// 指标注册失败会中止初始化，避免 Pool 已隔离失败却无法从 /metrics 观测。
func provideUserMetricsServer(addr userMetricsAddress, built *grpcx.BuiltServer) (*http.Server, error) {
	if err := built.Metrics.RegisterCollector(kafka.IsolatedPoolFailureCollector()); err != nil {
		return nil, fmt.Errorf("注册 user Kafka 旁路 Pool 指标失败: %w", err)
	}
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics), nil
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
) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		userpb.RegisterUserServiceServer(s, userHandler)
		userpb.RegisterInternalProfileServiceServer(s, internalProfileHandler)
	}
}

func provideUserGRPCServer(register grpcx.RegistrationFunc) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Namespace:        "user",
		Timeout:          &grpcx.TimeoutConfig{DefaultTimeout: userGRPCDefaultTimeout, MethodTimeouts: provideUserGRPCMethodTimeouts()},
		EnableHealth:     true,
		EnableReflection: grpcx.EnableDevelopmentReflection(),
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
	repository.NewUserRepository,
)

var userServiceProviderSet = wire.NewSet(
	service.NewProfileUserService,
	service.NewInternalProfileService,
)

var userHandlerProviderSet = wire.NewSet(
	handler.NewUserHandler,
	handler.NewInternalProfileHandler,
)

var userAppProviderSet = wire.NewSet(
	userInfraProviderSet,
	userRepositoryProviderSet,
	userServiceProviderSet,
	provideUserUserCreatedConsumer,
	provideUserAccountDeletedConsumer,
	userHandlerProviderSet,
	NewUserApp,
)
