package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/consumer"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// GroupApp 统一管理 group-service 生命周期。
//
// 当前 group-service 已承载群资料、成员写链路与缓存投影能力；
// 这里继续把 gRPC、metrics、MySQL、Redis、异步池等进程级资源聚合到一个 App 中，
// 便于统一管理对外服务入口与 group.cache projector 等后台组件的生命周期。
type GroupApp struct {
	logger              *zap.Logger
	metricsServer       *http.Server
	grpcServer          *grpc.Server
	grpcListener        net.Listener
	grpcShutdownTimeout time.Duration
	asyncPool           *ants.Pool
	asyncReleaseTimeout time.Duration
	cacheProjector      *consumer.CacheProjector
	cacheReconciler     *consumer.CacheReconciler
	realtimeProducer    *realtimepush.Producer
	db                  *gorm.DB
	redisClient         *goredis.Client
}

// NewGroupApp 只做资源聚合与合法性校验，不在构造阶段启动任何阻塞逻辑。
//
// 约束：
//  1. Provider 负责“创建资源”；
//  2. App 负责“编排资源生命周期”；
//  3. main 只负责“协调进程级退出信号”。
func NewGroupApp(
	log *zap.Logger,
	metricsServer *http.Server,
	built *grpcx.BuiltServer,
	grpcListener net.Listener,
	grpcShutdownTimeout groupGRPCShutdownTimeout,
	asyncPool *ants.Pool,
	asyncReleaseTimeout groupAsyncReleaseTimeout,
	cacheProjector *consumer.CacheProjector,
	cacheReconciler *consumer.CacheReconciler,
	realtimeProducer *realtimepush.Producer,
	db *gorm.DB,
	redisClient *goredis.Client,
) (*GroupApp, error) {
	if built == nil || built.Server == nil {
		return nil, errors.New("grpc server 未初始化")
	}
	if grpcListener == nil {
		return nil, errors.New("grpc listener 未初始化")
	}
	if cacheProjector == nil {
		return nil, errors.New("group cache projector 未初始化")
	}
	if cacheReconciler == nil {
		return nil, errors.New("group cache reconciler 未初始化")
	}

	return &GroupApp{
		logger:              log,
		metricsServer:       metricsServer,
		grpcServer:          built.Server,
		grpcListener:        grpcListener,
		grpcShutdownTimeout: time.Duration(grpcShutdownTimeout),
		asyncPool:           asyncPool,
		asyncReleaseTimeout: time.Duration(asyncReleaseTimeout),
		cacheProjector:      cacheProjector,
		cacheReconciler:     cacheReconciler,
		realtimeProducer:    realtimeProducer,
		db:                  db,
		redisClient:         redisClient,
	}, nil
}

// Run 负责启动所有长生命周期组件。
//
// 当前 group-service 除了 gRPC / metrics 外，还需要启动 group.cache projector，
// 因此这里统一负责：
//  1. 安装进程级全局依赖；
//  2. 启动 metrics HTTP server；
//  3. 启动 group.cache Kafka 消费者；
//  4. 启动 gRPC server 并阻塞当前 goroutine。
func (a *GroupApp) Run(ctx context.Context) error {
	if err := a.installProcessGlobals(ctx); err != nil {
		return err
	}

	if a.metricsServer != nil {
		go func() {
			logger.Info(ctx, "Metrics HTTP Server 启动中", logger.String("address", a.metricsServer.Addr))
			if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error(ctx, "Metrics HTTP Server 启动失败", logger.ErrorField("error", err))
			}
		}()
	}

	go func() {
		if err := a.cacheProjector.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error(ctx, "Group cache projector 运行错误", logger.ErrorField("error", err))
		}
	}()
	go func() {
		if err := a.cacheReconciler.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error(ctx, "Group cache reconciler 运行错误", logger.ErrorField("error", err))
		}
	}()

	logger.Info(ctx, "Group 服务启动中", logger.String("grpc_address", a.grpcListener.Addr().String()))
	if err := grpcx.Run(ctx, a.grpcServer, a.grpcListener); err != nil {
		return fmt.Errorf("运行 gRPC 服务失败: %w", err)
	}
	return nil
}

// Shutdown 按“先停入口、再停后台依赖、最后收资源”的顺序执行。
//
// 即便当前没有复杂后台任务，也保持和其他服务一致的关闭顺序，
// 这样后续新增消费者、内部客户端或缓存刷新器时，不需要改动停机骨架。
func (a *GroupApp) Shutdown(ctx context.Context) error {
	var errs []error

	if a.metricsServer != nil {
		if err := a.metricsServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("关闭 metrics server 失败: %w", err))
		}
	}
	if a.grpcServer != nil {
		if err := grpcx.GracefulStop(ctx, a.grpcServer, a.grpcShutdownTimeout); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			errs = append(errs, fmt.Errorf("关闭 grpc server 失败: %w", err))
		}
	}
	if a.grpcListener != nil {
		if err := a.grpcListener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, fmt.Errorf("关闭 grpc listener 失败: %w", err))
		}
	}
	if a.asyncPool != nil {
		var err error
		if async.Pool() == a.asyncPool {
			err = async.Release()
		} else {
			err = async.ReleasePool(a.asyncPool, a.asyncReleaseTimeout)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("释放 Async 协程池失败: %w", err))
		}
	}
	if a.cacheProjector != nil {
		if err := a.cacheProjector.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Group cache projector 失败: %w", err))
		}
	}
	if a.realtimeProducer != nil {
		if err := a.realtimeProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Group realtime.push 生产者失败: %w", err))
		}
	}
	if a.redisClient != nil {
		if err := a.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Redis 客户端失败: %w", err))
		}
	}
	if a.db != nil {
		sqlDB, err := a.db.DB()
		if err != nil {
			errs = append(errs, fmt.Errorf("获取 MySQL 原生连接失败: %w", err))
		} else if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 MySQL 连接失败: %w", err))
		}
	}
	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}

// installProcessGlobals 在对外提供服务前注册全局 logger / DB / Redis / 异步池。
//
// 这些都是“进程级副作用”，放在 Run 阶段统一安装，
// 可以避免在 Wire provider 中提前污染全局状态，保证依赖装配仍然是纯构造过程。
func (a *GroupApp) installProcessGlobals(ctx context.Context) error {
	logger.ReplaceGlobal(a.logger)
	if a.db != nil {
		mysql.ReplaceGlobal(a.db)
	}
	if a.redisClient != nil {
		pkgredis.ReplaceGlobal(a.redisClient)
	}
	async.SetContextPropagator(func(parent context.Context) context.Context {
		return ctxmeta.ChildContextFromParent(parent)
	})
	async.ReplaceGlobalWithReleaseTimeout(a.asyncPool, a.asyncReleaseTimeout)
	if err := util.InitSnowflakeFromEnv(); err != nil {
		return fmt.Errorf("初始化 Snowflake 节点失败: %w", err)
	}
	_ = ctx
	return nil
}
