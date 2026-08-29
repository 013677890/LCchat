package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/authcli"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/service"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
	"github.com/google/wire"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Wire 类型别名，避免同类型参数误绑定
type relationGRPCAddress string
type relationMetricsAddress string
type relationAuthGRPCAddress string
type relationAuthGRPCConn struct{ *grpc.ClientConn }
type relationAsyncReleaseTimeout time.Duration
type relationGRPCShutdownTimeout time.Duration

const relationGRPCDefaultTimeout = 300 * time.Millisecond

func provideRelationLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }
func provideRelationAsyncConfig() config.AsyncConfig   { return config.DefaultAsyncConfig() }
func provideRelationMySQLConfig() config.MySQLConfig   { return config.DefaultMySQLConfig() }
func provideRelationKafkaConfig() config.KafkaConfig   { return config.DefaultKafkaConfig() }

func provideRelationRedisConfig() config.RedisConfig {
	cfg := config.DefaultRedisConfig()
	cfg.ReadTimeout = 50 * time.Millisecond
	cfg.WriteTimeout = 50 * time.Millisecond
	// relation 的权限事实位于 MySQL；Redis 最多做 2 次短重试（共 3 次尝试），
	// 避免 Redis 故障把 300ms gRPC 请求预算全部吃满，最终失败交给 Kafka DEL 补偿。
	cfg.MaxRetries = 2
	cfg.MinRetryBackoff = 10 * time.Millisecond
	cfg.MaxRetryBackoff = 50 * time.Millisecond
	return cfg
}

func provideRelationLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

func provideRelationAsyncPool(_ *zap.Logger, cfg config.AsyncConfig) (*ants.Pool, error) {
	return async.Build(cfg)
}

func provideRelationAsyncReleaseTimeout(cfg config.AsyncConfig) relationAsyncReleaseTimeout {
	return relationAsyncReleaseTimeout(cfg.ReleaseTimeout)
}

func provideRelationMySQLDB(log *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	return mysql.Build(log, cfg)
}

// relation-service 允许 Redis 缺失后降级运行。
func provideRelationRedisClient(log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Warn(context.Background(), "Redis 初始化失败，将降级到 MySQL-Only 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	client.AddHook(redisretry.NewWriteFailureHook("relation-service.redis-write"))
	return client, nil
}

// 缓存失效补偿仅在 Redis 客户端启用时工作；MySQL-Only 模式没有待清理缓存。
func provideRelationRedisRetryProducer(redisClient *goredis.Client, cfg config.KafkaConfig) *kafka.Producer {
	if redisClient == nil {
		return nil
	}
	return kafka.NewProducer(cfg.Brokers, cfg.RelationRedisRetryTopic)
}

// parseRelationPoolWorkers 严格解析 relation-service 旁路消费 Pool 的 Reader 数。
// 空值统一默认 3；显式非法值带环境变量名返回，阻止错误配置进入运行态。
func parseRelationPoolWorkers(envName string) (int, error) {
	workers, err := kafka.ParsePoolWorkers(os.Getenv(envName))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", envName, err)
	}
	return workers, nil
}

// Consumer 的失败重试由通用 Kafka 消费器负责，超过预算后写入 dead_events。
func provideRelationRedisRetryConsumer(
	redisClient *goredis.Client,
	cfg config.KafkaConfig,
	log *zap.Logger,
	db *gorm.DB,
) (*redisretry.RedisRetryConsumer, error) {
	workers, err := parseRelationPoolWorkers("KAFKA_RELATION_REDIS_RETRY_CONSUMER_CONCURRENCY")
	if err != nil {
		return nil, err
	}
	if redisClient == nil {
		return nil, nil
	}
	return redisretry.NewRedisRetryConsumer(
		cfg.Brokers,
		cfg.RelationRedisRetryTopic,
		cfg.RelationRedisRetryGroupID,
		workers,
		redisClient,
		outbox.NewDeadLetterSink(db, "relation-service:redis-invalidation"),
		kafka.NewZapLoggerAdapter(log),
	)
}

func provideRelationGRPCAddress() relationGRPCAddress {
	addr := os.Getenv("RELATION_GRPC_ADDR")
	if addr == "" {
		addr = ":9093"
	}
	return relationGRPCAddress(addr)
}

func provideRelationMetricsAddress() relationMetricsAddress {
	addr := os.Getenv("RELATION_METRICS_ADDR")
	if addr == "" {
		addr = ":9193"
	}
	return relationMetricsAddress(addr)
}

func provideRelationAuthGRPCAddress() relationAuthGRPCAddress {
	addr := os.Getenv("AUTH_GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	return relationAuthGRPCAddress(addr)
}

// provideRelationAuthGRPCConn 创建 relation → auth 的内部查询连接。
// relation 只调用 BatchCheckAccountStatus 做好友申请前的账号存在性检查：
// 该方法为只读幂等查询，允许配置式重试；其余 auth 方法一律不重试。
// x-internal-caller 仅对该 full method 注入，caller 名与 auth 服务端白名单保持一致。
func provideRelationAuthGRPCConn(_ *zap.Logger, addr relationAuthGRPCAddress) (relationAuthGRPCConn, error) {
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry: grpcx.DefaultClientRetryConfig(
			"/auth.InternalAuthService/BatchCheckAccountStatus",
		),
		InternalCaller: &grpcx.InternalCallerClientConfig{
			Caller:  "relation-service",
			Methods: []string{"/auth.InternalAuthService/BatchCheckAccountStatus"},
		},
	})
	if err != nil {
		return relationAuthGRPCConn{}, fmt.Errorf("relation 创建 auth-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
	}
	return relationAuthGRPCConn{ClientConn: conn}, nil
}

// provideRelationAccountChecker 基于 auth 连接构造账号边界校验器。
func provideRelationAccountChecker(conn relationAuthGRPCConn) service.AccountChecker {
	return authcli.NewClient(conn.ClientConn)
}

func provideRelationGRPCShutdownTimeout() relationGRPCShutdownTimeout {
	return relationGRPCShutdownTimeout(10 * time.Second)
}

// provideRelationRealtimePushProducer 构造 relation-service 的实时提醒生产者。
func provideRelationRealtimePushProducer(cfg config.KafkaConfig) *realtimepush.Producer {
	return realtimepush.NewKafkaTopicProducer(cfg.Brokers, cfg.RealtimePushTopic)
}

// provideRelationAccountDeletedConsumer 构造 relation-service 的 account.deleted 消费者。
func provideRelationAccountDeletedConsumer(
	cfg config.KafkaConfig,
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
	db *gorm.DB,
) (*consumer.AccountDeletedConsumer, error) {
	workers, err := parseRelationPoolWorkers("KAFKA_RELATION_ACCOUNT_DELETED_CONSUMER_CONCURRENCY")
	if err != nil {
		return nil, err
	}
	groupID := cfg.ConsumerConfig.GroupID + "-relation-account-deleted"
	if cfg.ConsumerConfig.GroupID == "" {
		groupID = "relation-account-deleted-group"
	}
	return consumer.NewAccountDeletedConsumer(cfg.Brokers, cfg.AccountDeletedTopic, groupID, workers, friendRepo, applyRepo, db)
}

// provideRelationMetricsServer 注册 relation 的 gRPC 与旁路 Kafka Pool 指标并创建 HTTP 服务。
// 指标注册失败会中止初始化，避免 Pool 已隔离失败却无法从 /metrics 观测。
func provideRelationMetricsServer(addr relationMetricsAddress, built *grpcx.BuiltServer) (*http.Server, error) {
	if err := built.Metrics.RegisterCollector(kafka.IsolatedPoolFailureCollector()); err != nil {
		return nil, fmt.Errorf("注册 relation Kafka 旁路 Pool 指标失败: %w", err)
	}
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics), nil
}

func provideRelationRegistration(
	friendHandler *handler.FriendHandler,
	blacklistHandler *handler.BlacklistHandler,
) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		relationpb.RegisterFriendServiceServer(s, friendHandler)
		relationpb.RegisterBlacklistServiceServer(s, blacklistHandler)
	}
}

func provideRelationGRPCServer(register grpcx.RegistrationFunc) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Namespace:        "relation",
		Timeout:          &grpcx.TimeoutConfig{DefaultTimeout: relationGRPCDefaultTimeout},
		EnableHealth:     true,
		EnableReflection: grpcx.EnableDevelopmentReflection(),
	}, register)
}

func provideRelationGRPCListener(addr relationGRPCAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

var relationInfraProviderSet = wire.NewSet(
	provideRelationLoggerConfig,
	provideRelationAsyncConfig,
	provideRelationMySQLConfig,
	provideRelationKafkaConfig,
	provideRelationRedisConfig,
	provideRelationLogger,
	provideRelationAsyncPool,
	provideRelationAsyncReleaseTimeout,
	provideRelationMySQLDB,
	provideRelationRedisClient,
	provideRelationRedisRetryProducer,
	provideRelationRedisRetryConsumer,
	provideRelationGRPCAddress,
	provideRelationMetricsAddress,
	provideRelationAuthGRPCAddress,
	provideRelationAuthGRPCConn,
	provideRelationAccountChecker,
	provideRelationGRPCShutdownTimeout,
	provideRelationMetricsServer,
	provideRelationRegistration,
	provideRelationGRPCServer,
	provideRelationGRPCListener,
	provideRelationRealtimePushProducer,
	// 仅向 Wire 声明现有实现关系，不创建额外包装对象；服务运行时仍直接持有同一个 Producer 实例。
	wire.Bind(new(realtimepush.Publisher), new(*realtimepush.Producer)),
)

var relationRepositoryProviderSet = wire.NewSet(
	repository.NewFriendRepository,
	repository.NewApplyRepository,
	repository.NewBlacklistRepository,
)

var relationServiceProviderSet = wire.NewSet(
	service.NewFriendService,
	service.NewBlacklistService,
)

var relationHandlerProviderSet = wire.NewSet(
	handler.NewFriendHandler,
	handler.NewBlacklistHandler,
)

var relationAppProviderSet = wire.NewSet(
	relationInfraProviderSet,
	relationRepositoryProviderSet,
	provideRelationAccountDeletedConsumer,
	relationServiceProviderSet,
	relationHandlerProviderSet,
	NewRelationApp,
)
