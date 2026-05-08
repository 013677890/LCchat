package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/groupcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	mpserver "github.com/013677890/LCchat-Backend/apps/message-push/internal/server"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// messagePushRouteTTL = 路由存活窗口。
// 超过该窗口未活跃的设备路由会在读取时被视为过期，不再参与推送。
type messagePushRouteTTL time.Duration

// messagePushConnectUserTimeout = 单次 PushToUser RPC 超时时间。
// 用于限制 message-push 调用 connect 节点时的最长等待时间。
type messagePushConnectUserTimeout time.Duration

type messagePushUserGRPCAddress string

// provideMessagePushLoggerConfig 提供 message-push 专用日志配置。
func provideMessagePushLoggerConfig() config.LoggerConfig {
	return config.DefaultLoggerConfig()
}

// provideMessagePushRedisConfig 提供 message-push 专用 Redis 配置。
func provideMessagePushRedisConfig() config.RedisConfig {
	return config.DefaultRedisConfig()
}

// provideMessagePushKafkaConfig 提供 message-push 专用 Kafka 配置。
func provideMessagePushKafkaConfig() config.KafkaConfig {
	return config.DefaultKafkaConfig()
}

// provideMessagePushLogger 构建服务级 zap logger。
func provideMessagePushLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

// provideMessagePushRedisClient 构建 Redis 客户端。
// 当前 logger 参数仅用于与统一 provider 签名保持一致，便于后续扩展接入日志埋点。
func provideMessagePushRedisClient(_ *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	return pkgredis.Build(cfg)
}

// provideMessagePushGroupID 提供 Kafka consumer group id。
// 优先读取环境变量，未配置时回退到仓库内默认值。
func provideMessagePushGroupID() string {
	if v := os.Getenv("KAFKA_MSG_PUSH_GROUP_ID"); v != "" {
		return v
	}
	return "message-push-consumer-group"
}

// provideMessagePushRouteTTL 提供在线路由读取时的过期窗口。
// 环境变量单位为秒。
func provideMessagePushRouteTTL() messagePushRouteTTL {
	const defaultTTL = 180 * time.Second
	v := os.Getenv("MESSAGE_PUSH_ROUTE_TTL_SECONDS")
	if v == "" {
		return messagePushRouteTTL(defaultTTL)
	}
	d, err := time.ParseDuration(v + "s")
	if err != nil {
		// 环境变量格式非法时静默回退会掩盖配置错误，这里显式告警以便运维发现。
		logger.Warn(context.Background(), "message-push MESSAGE_PUSH_ROUTE_TTL_SECONDS 解析失败，使用默认值",
			logger.String("raw", v),
			logger.String("fallback", defaultTTL.String()),
			logger.ErrorField("error", err),
		)
		return messagePushRouteTTL(defaultTTL)
	}
	return messagePushRouteTTL(d)
}

// provideMessagePushConnectUserTimeout 提供 connect 推送超时配置。
// 环境变量单位为毫秒。
func provideMessagePushConnectUserTimeout() messagePushConnectUserTimeout {
	const defaultTimeout = 150 * time.Millisecond
	v := os.Getenv("MESSAGE_PUSH_CONNECT_TIMEOUT_USER_MS")
	if v == "" {
		return messagePushConnectUserTimeout(defaultTimeout)
	}
	d, err := time.ParseDuration(v + "ms")
	if err != nil {
		logger.Warn(context.Background(), "message-push MESSAGE_PUSH_CONNECT_TIMEOUT_USER_MS 解析失败，使用默认值",
			logger.String("raw", v),
			logger.String("fallback", defaultTimeout.String()),
			logger.ErrorField("error", err),
		)
		return messagePushConnectUserTimeout(defaultTimeout)
	}
	return messagePushConnectUserTimeout(d)
}

func provideMessagePushUserGRPCAddress() messagePushUserGRPCAddress {
	addr := os.Getenv("USER_GRPC_ADDR")
	if addr == "" {
		addr = ":9094"
	}
	return messagePushUserGRPCAddress(addr)
}

// 群聊扩散依赖 user-service 提供群成员列表，因此该连接是启动必需依赖。
func provideMessagePushUserGRPCConn(_ *zap.Logger, addr messagePushUserGRPCAddress) (*grpc.ClientConn, error) {
	// message-push 访问 user-service 只会命中 GroupService，
	// 因此把重试范围收敛到这一组 service，避免配置面无谓扩大。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry:   grpcx.DefaultClientRetryConfig("user.GroupService"),
	})
	if err != nil {
		return nil, fmt.Errorf("message-push 创建 user-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
	}
	return conn, nil
}

func provideGroupClient(conn *grpc.ClientConn) *groupcli.Client {
	return groupcli.NewClient(conn)
}

// provideRouteRepository 创建路由仓储。
// message-push 依赖它从 Redis 读取用户当前在线设备所在的 connect 节点。
func provideRouteRepository(client *goredis.Client, ttl messagePushRouteTTL) *route.RedisRepository {
	return route.NewRedisRepository(client, time.Duration(ttl))
}

// provideConnectClientManager 创建 connect gRPC 连接管理器。
// 同一地址的连接会在进程内复用，避免每次推送都重新建连。
func provideConnectClientManager() *connectcli.ClientManager {
	return connectcli.NewClientManager()
}

// provideConnectSender 创建 connect 推送发送器。
func provideConnectSender(manager *connectcli.ClientManager, timeout messagePushConnectUserTimeout) *connectcli.Sender {
	return connectcli.NewSender(manager, time.Duration(timeout))
}

// provideEventHandler 创建 Kafka 事件处理器。
// 它负责把 MsgPushEvent 解释为“查路由 / 查群成员 → 调 connect 推送”的执行流程。
func provideEventHandler(routes *route.RedisRepository, sender *connectcli.Sender, groups *groupcli.Client) *consumer.EventHandler {
	return consumer.NewEventHandler(routes, sender, groups)
}

// providePushConsumer 创建 msg.push topic 消费者。
func providePushConsumer(cfg config.KafkaConfig, groupID string, handler *consumer.EventHandler) *consumer.Consumer {
	return consumer.NewConsumer(cfg.Brokers, cfg.MsgPushTopic, groupID, handler)
}

// provideMessagePushHTTPConfig 提供 message-push 指标 HTTP 服务配置。
func provideMessagePushHTTPConfig() mpserver.Config {
	return mpserver.DefaultConfig()
}

// provideMessagePushHTTPServer 创建 message-push 指标 HTTP 服务。
func provideMessagePushHTTPServer(cfg mpserver.Config) *mpserver.Server {
	return mpserver.New(cfg)
}

// messagePushProviderSet 汇总 message-push 所需的全部依赖注入 provider。
var messagePushProviderSet = wire.NewSet(
	provideMessagePushLoggerConfig,
	provideMessagePushRedisConfig,
	provideMessagePushKafkaConfig,
	provideMessagePushLogger,
	provideMessagePushRedisClient,
	provideMessagePushGroupID,
	provideMessagePushRouteTTL,
	provideMessagePushConnectUserTimeout,
	provideMessagePushUserGRPCAddress,
	provideMessagePushUserGRPCConn,
	provideGroupClient,
	provideMessagePushHTTPConfig,
	provideMessagePushHTTPServer,
	provideRouteRepository,
	provideConnectClientManager,
	provideConnectSender,
	provideEventHandler,
	providePushConsumer,
	NewMessagePushApp,
)
