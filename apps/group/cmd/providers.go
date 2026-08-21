package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/group/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/cache"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/compose"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/projection"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/store"
	"github.com/013677890/LCchat-Backend/apps/group/internal/service"
	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// 这些别名用于避免 Wire 在多个 string / duration 参数之间误绑定。
// 由于地址、超时本质上都只是基础类型，如果不显式区分，依赖图变大后会更容易出现装配歧义。
type groupGRPCAddress string
type groupMetricsAddress string
type groupAsyncReleaseTimeout time.Duration
type groupGRPCShutdownTimeout time.Duration

const groupGRPCDefaultTimeout = 300 * time.Millisecond

// 以下 provider 统一保持“薄函数”风格：
//  1. 只负责构造某一类资源；
//  2. 不在这里启动后台任务；
//  3. 复杂生命周期交给 GroupApp.Run/Shutdown 管理。
func provideGroupLoggerConfig() config.LoggerConfig { return config.DefaultLoggerConfig() }
func provideGroupAsyncConfig() config.AsyncConfig   { return config.DefaultAsyncConfig() }
func provideGroupMySQLConfig() config.MySQLConfig   { return config.DefaultMySQLConfig() }
func provideGroupKafkaConfig() config.KafkaConfig   { return config.DefaultKafkaConfig() }

func provideGroupRedisConfig() config.RedisConfig {
	cfg := config.DefaultRedisConfig()
	cfg.ReadTimeout = 50 * time.Millisecond
	cfg.WriteTimeout = 50 * time.Millisecond
	return cfg
}

func provideGroupLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

func provideGroupAsyncPool(_ *zap.Logger, cfg config.AsyncConfig) (*ants.Pool, error) {
	return async.Build(cfg)
}

func provideGroupAsyncReleaseTimeout(cfg config.AsyncConfig) groupAsyncReleaseTimeout {
	return groupAsyncReleaseTimeout(cfg.ReleaseTimeout)
}

func provideGroupMySQLDB(_ *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	return mysql.Build(cfg)
}

// group-service 允许 Redis 缺失后以 MySQL-Only 模式降级运行。
//
// 当前高频权限链路会优先使用 Redis 缓存群成员，但 Redis 不可用时仍可通过
// singleflight 收敛回源查询，保证主链路可用，只是失去缓存加速收益。
func provideGroupRedisClient(log *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	client, err := pkgredis.Build(cfg)
	if err != nil {
		logger.Warn(context.Background(), "Redis 初始化失败，将降级到 MySQL-Only 模式", logger.ErrorField("error", err))
		_ = log
		return nil, nil
	}
	return client, nil
}

func provideGroupGRPCAddress() groupGRPCAddress {
	addr := os.Getenv("GROUP_GRPC_ADDR")
	if addr == "" {
		addr = ":9095"
	}
	return groupGRPCAddress(addr)
}

func provideGroupMetricsAddress() groupMetricsAddress {
	addr := os.Getenv("GROUP_METRICS_ADDR")
	if addr == "" {
		addr = ":9195"
	}
	return groupMetricsAddress(addr)
}

func provideGroupGRPCShutdownTimeout() groupGRPCShutdownTimeout {
	return groupGRPCShutdownTimeout(10 * time.Second)
}

// provideGroupCacheReconcilerConfig 解析唯一受支持的对账配置契约。
//
// interval 直接使用 Go duration（例如 6h、30m），表示相邻两轮之间的基准等待时间；
// reconciler 会在该基准上增加 ±20% 抖动，不再同时接受旧式“裸秒数”；
// 非法显式配置会让服务启动失败，避免运维以为对账已启用、实际却悄悄回落默认值。
func provideGroupCacheReconcilerConfig() (consumer.CacheReconcilerConfig, error) {
	cfg := consumer.CacheReconcilerConfig{
		Interval:  6 * time.Hour,
		BatchSize: 100,
	}
	if raw := os.Getenv("GROUP_CACHE_RECONCILE_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil || interval <= 0 {
			return consumer.CacheReconcilerConfig{}, fmt.Errorf(
				"GROUP_CACHE_RECONCILE_INTERVAL 必须是正数 Go duration: %q",
				raw,
			)
		}
		cfg.Interval = interval
	}

	if raw := os.Getenv("GROUP_CACHE_RECONCILE_BATCH_SIZE"); raw != "" {
		batchSize, err := strconv.Atoi(raw)
		if err != nil || batchSize <= 0 {
			return consumer.CacheReconcilerConfig{}, fmt.Errorf(
				"GROUP_CACHE_RECONCILE_BATCH_SIZE 必须是正整数: %q",
				raw,
			)
		}
		cfg.BatchSize = batchSize
	}

	return cfg, nil
}

func provideGroupCacheReconciler(
	projectorRepo repository.IGroupCacheProjectorRepository,
	cfg consumer.CacheReconcilerConfig,
) (*consumer.CacheReconciler, error) {
	return consumer.NewCacheReconciler(projectorRepo, cfg)
}

// provideGroupRealtimePushProducer 构造 group-service 的实时提醒生产者。
func provideGroupRealtimePushProducer(cfg config.KafkaConfig) *realtimepush.Producer {
	return realtimepush.NewKafkaTopicProducer(cfg.Brokers, cfg.RealtimePushTopic)
}

// provideGroupService 显式把单个 realtime producer 绑定到 service 的可变参数构造器。
//
// Wire 会把 variadic 参数视为 []Publisher，无法自行推断“把这个 producer 作为唯一
// 元素传入”；保留薄 wrapper 可让 wire_gen.go 始终由 go generate 可靠再生。
func provideGroupService(
	groupRepo repository.IGroupRepository,
	producer *realtimepush.Producer,
) service.IGroupService {
	return service.NewGroupService(groupRepo, producer)
}

// provideGroupCacheProjector 构造 group.cache 分区级并行投影消费者。
//
// 这里把消费者的 topic / groupID / worker 并发解析放在 provider，而不是 consumer 内部，原因是：
//  1. 配置读取职责应留在启动装配层；
//  2. consumer 只关心“如何消费并处理消息”；
//  3. 后续若需要按环境覆盖 topic / groupID / concurrency，改这里即可。
//
// KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY：未配置默认 3；显式值必须是 1～64 正整数，非法直接启动失败。
func provideGroupCacheProjector(
	cfg config.KafkaConfig,
	projectorRepo repository.IGroupCacheProjectorRepository,
	db *gorm.DB,
) (*consumer.CacheProjector, error) {
	groupID := cfg.GroupCacheGroupID
	if groupID == "" {
		groupID = "group-cache-projector-group"
	}
	workers, err := kafka.ParsePoolWorkers(os.Getenv("KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY"))
	if err != nil {
		return nil, fmt.Errorf("KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY: %w", err)
	}
	return consumer.NewCacheProjector(cfg.Brokers, cfg.GroupCacheTopic, groupID, workers, projectorRepo, db)
}

// provideGroupMetricsServer 注册 group 的 gRPC 与旁路 Kafka Pool 指标并创建 HTTP 服务。
// 指标注册失败会中止初始化；当前端点只暴露 /metrics，独立 /health 按实际需要再补充。
func provideGroupMetricsServer(addr groupMetricsAddress, built *grpcx.BuiltServer) (*http.Server, error) {
	if err := built.Metrics.RegisterCollector(kafka.IsolatedPoolFailureCollector()); err != nil {
		return nil, fmt.Errorf("注册 group Kafka 旁路 Pool 指标失败: %w", err)
	}
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics), nil
}

// provideGroupRegistration 负责把 proto service 注册到 gRPC Server。
//
// 这里把“注册什么服务”和“如何创建 server”拆开，
// 可以让 Wire 只负责装配依赖，运行时编排继续由 GroupApp 控制。
func provideGroupRegistration(groupHandler *handler.GroupHandler) grpcx.RegistrationFunc {
	return func(s *grpc.Server) {
		grouppb.RegisterGroupServiceServer(s, groupHandler)
	}
}

func provideGroupGRPCServer(register grpcx.RegistrationFunc) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Namespace:        "group",
		Timeout:          &grpcx.TimeoutConfig{DefaultTimeout: groupGRPCDefaultTimeout},
		EnableHealth:     true,
		EnableReflection: grpcx.EnableDevelopmentReflection(),
	}, register)
}

func provideGroupGRPCListener(addr groupGRPCAddress) (net.Listener, error) {
	return grpcx.NewListener(string(addr))
}

var groupInfraProviderSet = wire.NewSet(
	provideGroupLoggerConfig,
	provideGroupAsyncConfig,
	provideGroupMySQLConfig,
	provideGroupKafkaConfig,
	provideGroupRedisConfig,
	provideGroupLogger,
	provideGroupAsyncPool,
	provideGroupAsyncReleaseTimeout,
	provideGroupMySQLDB,
	provideGroupRedisClient,
	provideGroupGRPCAddress,
	provideGroupMetricsAddress,
	provideGroupGRPCShutdownTimeout,
	provideGroupCacheReconcilerConfig,
	provideGroupRealtimePushProducer,
	provideGroupMetricsServer,
	provideGroupRegistration,
	provideGroupGRPCServer,
	provideGroupGRPCListener,
)

// groupRepositoryProviderSet 装配 store + cache + projection，再由 compose 组合成 service 门面。
//
// 这里必须把投影仓储绑到父包接口，才能让 consumer 继续只依赖 IGroupCacheProjectorRepository。
var groupRepositoryProviderSet = wire.NewSet(
	store.New,
	projection.New,
	cache.NewWithProjector,
	compose.New,
	wire.Bind(new(repository.IGroupRepository), new(*compose.Facade)),
	wire.Bind(new(repository.IGroupCacheProjectorRepository), new(*projection.Repository)),
)

// service / handler 先只保留单一入口，后续若群审批、群角色拆成独立 service，
// 可以继续沿用相同 provider set 模式扩展。
var groupServiceProviderSet = wire.NewSet(
	provideGroupService,
)

var groupHandlerProviderSet = wire.NewSet(
	handler.NewGroupHandler,
)

var groupAppProviderSet = wire.NewSet(
	groupInfraProviderSet,
	groupRepositoryProviderSet,
	provideGroupCacheProjector,
	provideGroupCacheReconciler,
	groupServiceProviderSet,
	groupHandlerProviderSet,
	NewGroupApp,
)
