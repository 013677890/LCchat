package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/groupcli"
	mpserver "github.com/013677890/LCchat-Backend/apps/message-push/internal/server"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// messagePushRouteTTL = 路由存活窗口。
// 超过该窗口未活跃的设备路由会在读取时被视为过期，不再参与推送。
type messagePushRouteTTL time.Duration

// messagePushConnectUserTimeout = 单次 connect 推送 RPC 超时时间。
// 用于限制 message-push 调用 connect 节点时的最长等待时间。
type messagePushConnectUserTimeout time.Duration

// messagePushMaxFanoutConcurrency = 单条 msg.push 事件同时执行的 connect 节点扇出上限。
// 它不控制 Kafka consumer 数量，也不允许同一节点内对设备做无界并发。
type messagePushMaxFanoutConcurrency int

type messagePushGroupGRPCAddress string

// msgPushConsumer 标识 msg.push 专用消费者，避免 Wire 注入多个相同类型时歧义。
type msgPushConsumer struct{ *consumer.Consumer }

// realtimePushConsumer 标识 realtime.push 专用消费者，避免 Wire 注入多个相同类型时歧义。
type realtimePushConsumer struct{ *consumer.Consumer }

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
//
// 心跳会无条件刷新路由 activeMs（新鲜度约等于客户端心跳周期），该窗口只需覆盖
// 数个心跳周期即可；推送侧宁可多尝试（目标不在时 connect 会拒绝），因此默认取
// presence.DefaultPushWindow（360s），比 auth 在线判定窗口更宽。
func provideMessagePushRouteTTL() messagePushRouteTTL {
	const defaultTTL = route.DefaultPushWindow

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

// provideMessagePushMaxFanoutConcurrency 提供 connect 节点扇出并发上限。
// 未配置时使用明确的产品默认值 32；一旦显式配置，就必须是正整数。
// 非法值直接使依赖初始化失败，禁止回退到默认值后带着错误配置继续运行。
// 运行时还会与实际节点数取最小值，所以偏大的合法配置不会分配无用 worker。
func provideMessagePushMaxFanoutConcurrency() (messagePushMaxFanoutConcurrency, error) {
	const defaultConcurrency = 32

	raw := os.Getenv("MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY")
	if raw == "" {
		return messagePushMaxFanoutConcurrency(defaultConcurrency), nil
	}
	concurrency, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf(
			"MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY 必须是正整数（当前值=%q）: %w",
			raw,
			err,
		)
	}

	if concurrency <= 0 {
		return 0, fmt.Errorf(
			"MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY 必须大于零（当前值=%q）",
			raw,
		)
	}

	return messagePushMaxFanoutConcurrency(concurrency), nil
}

func provideMessagePushGroupGRPCAddress() messagePushGroupGRPCAddress {
	addr := os.Getenv("GROUP_GRPC_ADDR")
	if addr == "" {
		addr = ":9095"
	}
	return messagePushGroupGRPCAddress(addr)
}

// 群聊扩散依赖 group-service 提供群成员列表，因此该连接是启动必需依赖。
func provideMessagePushGroupGRPCConn(_ *zap.Logger, addr messagePushGroupGRPCAddress) (*grpc.ClientConn, error) {
	// message-push → group 只读取群成员列表做扩散，配置式重试仅允许这两个只读 full method。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry: grpcx.DefaultClientRetryConfig(
			"/group.GroupService/GetGroupMemberIds",
			"/group.GroupService/GetMemberList",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("message-push 创建 group-service gRPC 连接失败（addr=%s）: %w", string(addr), err)
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
// 它负责把 MsgPushEvent 解释为“查路由 / 查群成员 -> 调 connect 推送”的执行流程。
// 并发上限是强制构造参数；EventHandler 会再次校验依赖与配置，不提供旧签名或默认回退。
func provideEventHandler(
	routes *route.RedisRepository,
	sender *connectcli.Sender,
	groups *groupcli.Client,
	concurrency messagePushMaxFanoutConcurrency,
) (*consumer.EventHandler, error) {
	return consumer.NewEventHandler(
		routes,
		sender,
		groups,
		int(concurrency),
	)
}

// provideRealtimeHandler 创建 realtime.push 事件处理器。
// 它负责把非消息类提醒按目标类型扩散到在线设备并复用 connect 下行链路。
func provideRealtimeHandler(routes *route.RedisRepository, sender *connectcli.Sender, groups *groupcli.Client) *consumer.RealtimeHandler {
	return consumer.NewRealtimeHandler(routes, sender, groups)
}

// parseMessagePushPoolWorkers 严格解析指定 message-push Pool 的 Reader 数。
// 空值使用统一默认值 3，任何显式非法值都返回带环境变量名的启动错误。
func parseMessagePushPoolWorkers(envName string) (int, error) {
	workers, err := kafka.ParsePoolWorkers(os.Getenv(envName))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", envName, err)
	}
	return workers, nil
}

// providePushConsumer 创建 msg.push topic 消费 Pool。
// 显式并发必须在 1～64，非法配置直接中止初始化，避免生产实例悄悄以错误容量运行。
func providePushConsumer(cfg config.KafkaConfig, groupID string, handler *consumer.EventHandler) (msgPushConsumer, error) {
	workers, err := parseMessagePushPoolWorkers("KAFKA_MSG_PUSH_CONSUMER_CONCURRENCY")
	if err != nil {
		return msgPushConsumer{}, err
	}
	pool, err := consumer.NewConsumer(cfg.Brokers, cfg.MsgPushTopic, groupID, workers, handler)
	if err != nil {
		return msgPushConsumer{}, err
	}
	return msgPushConsumer{Consumer: pool}, nil
}

// provideRealtimePushConsumer 创建 realtime.push topic 消费 Pool。
// 它使用独立 groupID 与并发配置，不会和 msg.push 消费者互相抢占消息。
func provideRealtimePushConsumer(cfg config.KafkaConfig, handler *consumer.RealtimeHandler) (realtimePushConsumer, error) {
	workers, err := parseMessagePushPoolWorkers("KAFKA_REALTIME_PUSH_CONSUMER_CONCURRENCY")
	if err != nil {
		return realtimePushConsumer{}, err
	}
	pool, err := consumer.NewConsumer(cfg.Brokers, cfg.RealtimePushTopic, cfg.RealtimePushGroupID, workers, handler)
	if err != nil {
		return realtimePushConsumer{}, err
	}
	return realtimePushConsumer{Consumer: pool}, nil
}

// providePushConsumers 聚合 message-push 启动所需的所有 Kafka 消费者。
func providePushConsumers(msg msgPushConsumer, realtime realtimePushConsumer) pushConsumers {
	return pushConsumers{msg: msg.Consumer, realtime: realtime.Consumer}
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
	provideMessagePushMaxFanoutConcurrency,
	provideMessagePushGroupGRPCAddress,
	provideMessagePushGroupGRPCConn,
	provideGroupClient,
	provideMessagePushHTTPConfig,
	provideMessagePushHTTPServer,
	provideRouteRepository,
	provideConnectClientManager,
	provideConnectSender,
	provideEventHandler,
	provideRealtimeHandler,
	providePushConsumer,
	provideRealtimePushConsumer,
	providePushConsumers,
	NewMessagePushApp,
)
