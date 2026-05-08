package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/groupcli"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usecase"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usercli"
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
	"gorm.io/gorm"
)

type grpcAddress string
type metricsAddress string
type msgUserGRPCAddress string
type msgRelationGRPCAddress string
type msgUserGRPCConn struct{ *grpc.ClientConn }
type msgRelationGRPCConn struct{ *grpc.ClientConn }
type msgAsyncReleaseTimeout time.Duration
type msgGRPCShutdownTimeout time.Duration

const (
	msgGRPCDefaultTimeout = 500 * time.Millisecond
	msgSendMessageTimeout = 900 * time.Millisecond
)

func provideLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }
func provideAsyncConfig() config.AsyncConfig   { return config.DefaultAsyncConfig() }
func provideMySQLConfig() config.MySQLConfig   { return config.DefaultMySQLConfig() }

func provideRedisConfig() config.RedisConfig {
	cfg := config.DefaultRedisConfig()
	cfg.ReadTimeout = 50 * time.Millisecond
	cfg.WriteTimeout = 50 * time.Millisecond
	return cfg
}

func provideKafkaConfig() config.KafkaConfig                     { return config.DefaultKafkaConfig() }
func provideLogger(cfg config.LoggerConfig) (*zap.Logger, error) { return logger.Build(cfg) }
func provideAsyncPool(_ *zap.Logger, cfg config.AsyncConfig) (*ants.Pool, error) {
	return async.Build(cfg)
}
func provideAsyncReleaseTimeout(cfg config.AsyncConfig) msgAsyncReleaseTimeout {
	return msgAsyncReleaseTimeout(cfg.ReleaseTimeout)
}
func provideMySQLDB(_ *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) { return mysql.Build(cfg) }
func provideRedisClient(_ *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	return pkgredis.Build(cfg)
}
func provideKafkaProducer(cfg config.KafkaConfig) *kafka.Producer {
	return kafka.NewProducer(cfg.Brokers, cfg.MsgPushTopic)
}
func provideMsgProducer(cfg config.KafkaConfig, producer *kafka.Producer) *mq.Producer {
	return mq.NewProducer(producer, cfg.MsgPushTopic)
}
func provideMsgConfig() message.Config { return message.DefaultConfig() }

func provideMsgUserGRPCAddress() msgUserGRPCAddress {
	addr := os.Getenv("USER_GRPC_ADDR")
	if addr == "" {
		addr = ":9094"
	}
	return msgUserGRPCAddress(addr)
}

func provideMsgRelationGRPCAddress() msgRelationGRPCAddress {
	addr := os.Getenv("RELATION_GRPC_ADDR")
	if addr == "" {
		addr = ":9093"
	}
	return msgRelationGRPCAddress(addr)
}

func provideMsgUserGRPCConn(_ *zap.Logger, addr msgUserGRPCAddress) (msgUserGRPCConn, error) {
	// msg 访问 user-service 只会落到 GroupService，
	// 因此重试策略也只声明这一组实际会调用的方法，避免把无关 service 塞进同一份配置。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry:   grpcx.DefaultClientRetryConfig("user.GroupService"),
	})
	if err != nil {
		return msgUserGRPCConn{}, fmt.Errorf("msg 创建 user-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
	}
	return msgUserGRPCConn{ClientConn: conn}, nil
}

func provideMsgRelationGRPCConn(_ *zap.Logger, addr msgRelationGRPCAddress) (msgRelationGRPCConn, error) {
	// relation 侧只会校验好友与黑名单，
	// 这里显式列出两个 service，保证重试配置与实际调用面完全一致。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry: grpcx.DefaultClientRetryConfig(
			"relation.FriendService",
			"relation.BlacklistService",
		),
	})
	if err != nil {
		return msgRelationGRPCConn{}, fmt.Errorf("msg 创建 relation-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
	}
	return msgRelationGRPCConn{ClientConn: conn}, nil
}

func provideMsgGroupClient(conn msgUserGRPCConn) *groupcli.Client {
	return groupcli.NewClient(conn.ClientConn)
}
func provideMsgPermissionChecker(relationConn msgRelationGRPCConn, userConn msgUserGRPCConn) usecase.PermissionChecker {
	return usercli.NewPermissionChecker(relationConn.ClientConn, userConn.ClientConn)
}

func provideMsgService(repo message.Repository, cfg message.Config, gc *groupcli.Client) *message.Service {
	svc := message.NewService(repo, cfg)
	if gc != nil {
		svc.SetGroupRoleQuerier(gc)
	}
	return svc
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
		addr = ":9192"
	}
	return metricsAddress(addr)
}

func provideGRPCShutdownTimeout() msgGRPCShutdownTimeout {
	return msgGRPCShutdownTimeout(10 * time.Second)
}

func provideMsgRegistration(msgHandler *handler.MsgHandler) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		msgpb.RegisterMsgServiceServer(s, msgHandler)
	}
}

func provideMsgGRPCServer(register grpcx.RegistrationFunc, addr grpcAddress) (*grpcx.BuiltServer, error) {
	opts := grpcx.ServerOptions{
		Address:   string(addr),
		Namespace: "msg",
		Timeout: &grpcx.TimeoutConfig{
			DefaultTimeout: msgGRPCDefaultTimeout,
			MethodTimeouts: map[string]time.Duration{
				"/msg.MsgService/SendMessage": msgSendMessageTimeout,
			},
		},
		EnableHealth:     true,
		EnableReflection: true,
	}
	return grpcx.NewServer(opts, register)
}

func provideMsgGRPCListener(addr grpcAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

func provideMetricsServer(addr metricsAddress, built *grpcx.BuiltServer) *http.Server {
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics)
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
	provideMsgUserGRPCAddress,
	provideMsgRelationGRPCAddress,
	provideMsgUserGRPCConn,
	provideMsgRelationGRPCConn,
	provideMsgGroupClient,
	provideMsgPermissionChecker,
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

var msgHandlerProviderSet = wire.NewSet(handler.NewMsgHandler)

var msgAppProviderSet = wire.NewSet(
	msgInfraProviderSet,
	msgDomainProviderSet,
	msgHandlerProviderSet,
	NewMsgApp,
)
