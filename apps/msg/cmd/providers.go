package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/consumer"
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
func provideKafkaConfig() config.KafkaConfig   { return config.DefaultKafkaConfig() }

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
	// msg → group 只做发送前权限检查，配置式重试仅允许这两个只读 full method。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry: grpcx.DefaultClientRetryConfig(
			"/group.GroupService/CheckGroupMember",
			"/group.GroupService/CheckGroupSendPermission",
		),
	})
	if err != nil {
		return msgGroupGRPCConn{}, fmt.Errorf("msg 创建 group-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
	}
	return msgGroupGRPCConn{ClientConn: conn}, nil
}

func provideMsgRelationGRPCConn(_ *zap.Logger, addr msgRelationGRPCAddress) (msgRelationGRPCConn, error) {
	// msg → relation 只做好友/黑名单检查，配置式重试仅允许这两个只读 full method。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry: grpcx.DefaultClientRetryConfig(
			"/relation.FriendService/CheckIsFriend",
			"/relation.BlacklistService/CheckIsBlacklist",
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

// provideMsgGroupMembershipProjector 为 msg 创建独立的 group.cache 分区级并行消费组。
// GroupCacheGroupID 属于 group-service 的 Redis projector，二者必须使用不同 group ID，
// 否则 Kafka 会把同一事件负载均衡给两个服务，造成任一投影随机漏事件。
//
// KAFKA_MSG_GROUP_MEMBERSHIP_PROJECTOR_CONCURRENCY：未配置默认 3；显式值必须是 1～64 正整数，非法直接启动失败。
func provideMsgGroupMembershipProjector(
	cfg config.KafkaConfig,
	repo conversation.GroupMembershipProjectorRepository,
	db *gorm.DB,
) (*consumer.GroupMembershipProjector, error) {
	switch {
	case len(cfg.Brokers) == 0:
		return nil, fmt.Errorf("msg group membership projector 缺少 Kafka brokers")
	case cfg.GroupCacheTopic == "":
		return nil, fmt.Errorf("msg group membership projector 缺少 group.cache topic")
	case cfg.MsgGroupMembershipGroupID == "":
		return nil, fmt.Errorf("msg group membership projector 缺少独立 consumer group id")
	case cfg.MsgGroupMembershipGroupID == cfg.GroupCacheGroupID:
		return nil, fmt.Errorf(
			"msg group membership projector consumer group id 不能与 group cache projector 相同: %s",
			cfg.MsgGroupMembershipGroupID,
		)
	}

	workers, err := kafka.ParsePoolWorkers(os.Getenv("KAFKA_MSG_GROUP_MEMBERSHIP_PROJECTOR_CONCURRENCY"))
	if err != nil {
		return nil, fmt.Errorf("KAFKA_MSG_GROUP_MEMBERSHIP_PROJECTOR_CONCURRENCY: %w", err)
	}
	return consumer.NewGroupMembershipProjector(
		cfg.Brokers,
		cfg.GroupCacheTopic,
		cfg.MsgGroupMembershipGroupID,
		workers,
		repo,
		db,
	)
}

// provideConvService 创建会话领域服务，并注入群成员权威点查器。
// 点查只覆盖 group.cache 尚未投影到本地的短暂可见性窗口。
func provideConvService(repo conversation.Repository, gc *groupcli.Client) *conversation.Service {
	svc := conversation.NewService(repo)
	if gc != nil {
		svc.SetGroupMembershipQuerier(gc)
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

func provideMsgGRPCServer(register grpcx.RegistrationFunc) (*grpcx.BuiltServer, error) {
	opts := grpcx.ServerOptions{
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
	provideKafkaConfig,
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
	provideMsgGroupMembershipProjector,
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
	conversation.NewGroupMembershipProjectorRepository,
	provideConvService,
	usecase.NewMessageReadWorkflow,
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
