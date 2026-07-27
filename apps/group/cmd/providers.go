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
	"github.com/013677890/LCchat-Backend/apps/group/internal/service"
	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
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
// interval 直接使用 Go duration（例如 10m、30s），不再同时接受旧式“裸秒数”；
// 非法显式配置会让服务启动失败，避免运维以为对账已启用、实际却悄悄回落默认值。
func provideGroupCacheReconcilerConfig() (consumer.CacheReconcilerConfig, error) {
	cfg := consumer.CacheReconcilerConfig{
		Interval:  10 * time.Minute,
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

// provideGroupCacheProjector 构造 group.cache 投影消费者。
//
// 这里把消费者的 topic / groupID 解析放在 provider，而不是 consumer 内部，原因是：
//  1. 配置读取职责应留在启动装配层；
//  2. consumer 只关心“如何消费并处理消息”；
//  3. 后续若需要按环境覆盖 topic / groupID，改这里即可。
func provideGroupCacheProjector(
	cfg config.KafkaConfig,
	projectorRepo repository.IGroupCacheProjectorRepository,
	db *gorm.DB,
) *consumer.CacheProjector {
	groupID := cfg.GroupCacheGroupID
	if groupID == "" {
		groupID = "group-cache-projector-group"
	}
	return consumer.NewCacheProjector(cfg.Brokers, cfg.GroupCacheTopic, groupID, projectorRepo, db)
}

// provideGroupMetricsServer 复用 grpcx 的统一 metrics HTTP server。
//
// 当前和 relation/user/msg 一样，先只暴露 /metrics，
// 后续如果 group-service 需要独立 /health，再按实际运行特征补充。
func provideGroupMetricsServer(addr groupMetricsAddress, built *grpcx.BuiltServer) *http.Server {
	return grpcx.NewMetricsHTTPServer(string(addr), built.Metrics)
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

func provideGroupGRPCServer(register grpcx.RegistrationFunc, addr groupGRPCAddress) (*grpcx.BuiltServer, error) {
	return grpcx.NewServer(grpcx.ServerOptions{
		Address:          string(addr),
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

// group-service 当前只有一个聚合仓储，占位承接未来的群资料、成员、审批等持久化能力。
var groupRepositoryProviderSet = wire.NewSet(
	repository.NewGroupRepository,
	repository.NewGroupCacheProjectorRepository,
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
