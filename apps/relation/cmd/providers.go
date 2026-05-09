package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/service"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
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

func provideRelationMySQLDB(_ *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	return mysql.Build(cfg)
}

// relation-service 允许 Redis 缺失后降级运行。
func provideRelationRedisClient(log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Warn(context.Background(), "Redis 初始化失败，将降级到 MySQL-Only 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	return client, nil
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

func provideRelationGRPCShutdownTimeout() relationGRPCShutdownTimeout {
	return relationGRPCShutdownTimeout(10 * time.Second)
}

// provideRelationAccountDeletedConsumer 构造 relation-service 的 account.deleted 消费者。
func provideRelationAccountDeletedConsumer(
	cfg config.KafkaConfig,
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
	db *gorm.DB,
) *consumer.AccountDeletedConsumer {
	groupID := cfg.ConsumerConfig.GroupID + "-relation-account-deleted"
	if cfg.ConsumerConfig.GroupID == "" {
		groupID = "relation-account-deleted-group"
	}
	return consumer.NewAccountDeletedConsumer(cfg.Brokers, cfg.AccountDeletedTopic, groupID, friendRepo, applyRepo, db)
}

func provideRelationMetricsServer(addr relationMetricsAddress, built *grpcx.BuiltServer) *http.Server {
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics)
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

func provideRelationGRPCServer(register grpcx.RegistrationFunc, addr relationGRPCAddress) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Address:          string(addr),
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
	provideRelationGRPCAddress,
	provideRelationMetricsAddress,
	provideRelationGRPCShutdownTimeout,
	provideRelationMetricsServer,
	provideRelationRegistration,
	provideRelationGRPCServer,
	provideRelationGRPCListener,
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
