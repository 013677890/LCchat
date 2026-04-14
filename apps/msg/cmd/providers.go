package main

import (
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usecase"
	"github.com/013677890/LCchat-Backend/apps/msg/mq"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
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

type grpcAddress string

type metricsAddress string

type msgAsyncReleaseTimeout time.Duration

type msgGRPCShutdownTimeout time.Duration

func provideLoggerConfig() config.LoggerConfig {
	return config.DefaultLoggerConfig()
}

func provideAsyncConfig() config.AsyncConfig {
	return config.DefaultAsyncConfig()
}

func provideMySQLConfig() config.MySQLConfig {
	return config.DefaultMySQLConfig()
}

func provideRedisConfig() config.RedisConfig {
	cfg := config.DefaultRedisConfig()
	cfg.ReadTimeout = 50 * time.Millisecond
	cfg.WriteTimeout = 50 * time.Millisecond
	return cfg
}

func provideKafkaConfig() config.KafkaConfig {
	return config.DefaultKafkaConfig()
}

func provideLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

func provideAsyncPool(_ *zap.Logger, cfg config.AsyncConfig) (*ants.Pool, error) {
	return async.Build(cfg)
}

func provideAsyncReleaseTimeout(cfg config.AsyncConfig) msgAsyncReleaseTimeout {
	return msgAsyncReleaseTimeout(cfg.ReleaseTimeout)
}

func provideMySQLDB(_ *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	return mysql.Build(cfg)
}

func provideRedisClient(_ *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	return pkgredis.Build(cfg)
}

func provideKafkaProducer(cfg config.KafkaConfig) *kafka.Producer {
	return kafka.NewProducer(cfg.Brokers, cfg.MsgPushTopic)
}

func provideMsgProducer(cfg config.KafkaConfig, producer *kafka.Producer) *mq.Producer {
	return mq.NewProducer(producer, cfg.MsgPushTopic)
}

func provideMsgConfig() message.Config {
	return message.DefaultConfig()
}

func provideMsgService(repo message.Repository, cfg message.Config) *message.Service {
	return message.NewService(repo, cfg)
}

func provideGRPCAddress() grpcAddress {
	addr := os.Getenv("MSG_GRPC_ADDR")
	if addr == "" {
		addr = ":9092"
	}
	return grpcAddress(addr)
}

func provideMetricsAddress() metricsAddress {
	addr := os.Getenv("MSG_METRICS_ADDR")
	if addr == "" {
		addr = ":9093"
	}
	return metricsAddress(addr)
}

func provideGRPCShutdownTimeout() msgGRPCShutdownTimeout {
	return msgGRPCShutdownTimeout(10 * time.Second)
}

func provideMsgRegistration(msgHandler *handler.MsgHandler) grpcx.RegistrationFunc {
	return func(s *grpc.Server, hs healthgrpc.HealthServer) {
		msgpb.RegisterMsgServiceServer(s, msgHandler)
		if hs != nil {
			if setter, ok := hs.(interface {
				SetServingStatus(service string, status healthgrpc.HealthCheckResponse_ServingStatus)
			}); ok {
				setter.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
			}
		}
	}
}

func provideMsgGRPCServer(register grpcx.RegistrationFunc, addr grpcAddress) (*grpcx.BuiltServer, error) {
	opts := grpcx.ServerOptions{
		Address:          string(addr),
		Namespace:        "msg",
		EnableHealth:     true,
		EnableReflection: true,
	}
	return grpcx.NewServer(opts, register)
}

func provideMsgGRPCListener(addr grpcAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

func provideMetricsServer(addr metricsAddress, built *grpcx.BuiltServer) *http.Server {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", built.Metrics.Handler())
	return &http.Server{
		Addr:    string(addr),
		Handler: metricsMux,
	}
}

var _ = conversation.NewRepository
var _ = usecase.NewSendMessageWorkflow

var msgInfraProviderSet = wire.NewSet(
	provideLoggerConfig,
	provideAsyncConfig,
	provideMySQLConfig,
	provideRedisConfig,
	provideKafkaConfig,
	provideLogger,
	provideAsyncPool,
	provideAsyncReleaseTimeout,
	provideMySQLDB,
	provideRedisClient,
	provideKafkaProducer,
	provideMsgProducer,
	provideMsgConfig,
	provideMsgService,
	provideGRPCAddress,
	provideMetricsAddress,
	provideGRPCShutdownTimeout,
	provideMsgRegistration,
	provideMsgGRPCServer,
	provideMsgGRPCListener,
	provideMetricsServer,
)

var msgDomainProviderSet = wire.NewSet(
	message.NewRepository,
	conversation.NewRepository,
	conversation.NewService,
	usecase.NewSendMessageWorkflow,
	usecase.NewRecallMessageWorkflow,
	usecase.NewMarkReadWorkflow,
)

var msgHandlerProviderSet = wire.NewSet(
	handler.NewMsgHandler,
)

var msgAppProviderSet = wire.NewSet(
	msgInfraProviderSet,
	msgDomainProviderSet,
	msgHandlerProviderSet,
	NewMsgApp,
)
