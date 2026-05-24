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
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
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

type msgGRPCAddress string
type msgMetricsAddress string
type msgGroupGRPCAddress string
type msgRelationGRPCAddress string
type msgGroupGRPCConn struct{ *grpc.ClientConn }
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
func provideLogger(cfg config.LoggerConfig) (*zap.Logger, error) { return logger.Build(cfg) }

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

func provideMsgConfig() message.Config { return message.DefaultConfig() }

func provideMsgGroupGRPCAddress() msgGroupGRPCAddress {
	addr := os.Getenv("GROUP_GRPC_ADDR")
	if addr == "" {
		addr = ":9095"
	}
	return msgGroupGRPCAddress(addr)
}

func provideMsgRelationGRPCAddress() msgRelationGRPCAddress {
	addr := os.Getenv("RELATION_GRPC_ADDR")
	if addr == "" {
		addr = ":9093"
	}
	return msgRelationGRPCAddress(addr)
}

func provideMsgGroupGRPCConn(_ *zap.Logger, addr msgGroupGRPCAddress) (msgGroupGRPCConn, error) {
	// msg 访问 group-service 当前只会命中 GroupService，
	// 因此重试策略也只声明这一组实际会调用的方法，避免把无关 service 塞进同一份配置。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry:   grpcx.DefaultClientRetryConfig("group.GroupService"),
	})
	if err != nil {
		return msgGroupGRPCConn{}, fmt.Errorf("msg 创建 group-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
	}
	return msgGroupGRPCConn{ClientConn: conn}, nil
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

func provideMsgGroupClient(conn msgGroupGRPCConn) *groupcli.Client {
	return groupcli.NewClient(conn.ClientConn)
}

func provideMsgPermissionChecker(relationConn msgRelationGRPCConn, groupConn msgGroupGRPCConn) usecase.PermissionChecker {
	return usercli.NewPermissionChecker(relationConn.ClientConn, groupConn.ClientConn)
}

func provideMsgService(repo message.Repository, cfg message.Config, gc *groupcli.Client) *message.Service {
	svc := message.NewService(repo, cfg)
	if gc != nil {
		svc.SetGroupRoleQuerier(gc)
	}
	return svc
}

func provideMsgGRPCAddress() msgGRPCAddress {
	addr := os.Getenv("MSG_GRPC_ADDR")
	if addr == "" {
		addr = ":9092"
	}
	return msgGRPCAddress(addr)
}

func provideMsgMetricsAddress() msgMetricsAddress {
	addr := os.Getenv("MSG_METRICS_ADDR")
	if addr == "" {
		addr = ":9192"
	}
	return msgMetricsAddress(addr)
}

func provideMsgGRPCShutdownTimeout() msgGRPCShutdownTimeout {
	return msgGRPCShutdownTimeout(10 * time.Second)
}

func provideMsgRegistration(msgHandler *handler.MsgHandler) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		msgpb.RegisterMsgServiceServer(s, msgHandler)
	}
}

func provideMsgGRPCServer(register grpcx.RegistrationFunc, addr msgGRPCAddress) (*grpcx.BuiltServer, error) {
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
		EnableReflection: grpcx.EnableDevelopmentReflection(),
	}
	return grpcx.NewServer(opts, register)
}

func provideMsgGRPCListener(addr msgGRPCAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

func provideMsgMetricsServer(addr msgMetricsAddress, built *grpcx.BuiltServer) *http.Server {
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics)
}

var _ = conversation.NewRepository
var _ = usecase.NewSendMessageWorkflow

var msgInfraProviderSet = wire.NewSet(
	provideLoggerConfig,
	provideAsyncConfig,
	provideMySQLConfig,
	provideRedisConfig,
	provideLogger,
	provideAsyncPool,
	provideAsyncReleaseTimeout,
	provideMySQLDB,
	provideRedisClient,
	provideMsgConfig,
	provideMsgGroupGRPCAddress,
	provideMsgRelationGRPCAddress,
	provideMsgGroupGRPCConn,
	provideMsgRelationGRPCConn,
	provideMsgGroupClient,
	provideMsgPermissionChecker,
	provideMsgService,
	provideMsgGRPCAddress,
	provideMsgMetricsAddress,
	provideMsgGRPCShutdownTimeout,
	provideMsgRegistration,
	provideMsgGRPCServer,
	provideMsgGRPCListener,
	provideMsgMetricsServer,
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
