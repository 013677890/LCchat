package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/consumer"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// RelationApp 统一管理 relation-service 生命周期。
type RelationApp struct {
	logger                 *zap.Logger
	metricsServer          *http.Server
	grpcServer             *grpc.Server
	grpcListener           net.Listener
	grpcShutdownTimeout    time.Duration
	asyncPool              *ants.Pool
	asyncReleaseTimeout    relationAsyncReleaseTimeout
	accountDeletedConsumer *consumer.AccountDeletedConsumer
	db                     *gorm.DB
	redisClient            *goredis.Client
}

// NewRelationApp 只做资源聚合与合法性校验，不在构造阶段启动任何阻塞逻辑。
func NewRelationApp(
	log *zap.Logger,
	metricsServer *http.Server,
	built *grpcx.BuiltServer,
	grpcListener net.Listener,
	grpcShutdownTimeout relationGRPCShutdownTimeout,
	asyncPool *ants.Pool,
	asyncReleaseTimeout relationAsyncReleaseTimeout,
	accountDeletedConsumer *consumer.AccountDeletedConsumer,
	db *gorm.DB,
	redisClient *goredis.Client,
) (*RelationApp, error) {
	if built == nil || built.Server == nil {
		return nil, errors.New("grpc server 未初始化")
	}
	if grpcListener == nil {
		return nil, errors.New("grpc listener 未初始化")
	}
	return &RelationApp{
		logger:                 log,
		metricsServer:          metricsServer,
		grpcServer:             built.Server,
		grpcListener:           grpcListener,
		grpcShutdownTimeout:    time.Duration(grpcShutdownTimeout),
		asyncPool:              asyncPool,
		asyncReleaseTimeout:    asyncReleaseTimeout,
		accountDeletedConsumer: accountDeletedConsumer,
		db:                     db,
		redisClient:            redisClient,
	}, nil
}

// Run 负责启动所有长生命周期组件。
func (a *RelationApp) Run(ctx context.Context) error {
	a.installProcessGlobals(ctx)

	if a.metricsServer != nil {
		go func() {
			logger.Info(ctx, "Metrics HTTP Server 启动中", logger.String("address", a.metricsServer.Addr))
			if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error(ctx, "Metrics HTTP Server 启动失败", logger.ErrorField("error", err))
			}
		}()
	}

	if a.accountDeletedConsumer != nil {
		go func() {
			if err := a.accountDeletedConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error(ctx, "Relation account.deleted 消费者运行错误", logger.ErrorField("error", err))
			}
		}()
	}

	logger.Info(ctx, "Relation 服务启动中", logger.String("grpc_address", a.grpcListener.Addr().String()))
	if err := grpcx.Run(ctx, a.grpcServer, a.grpcListener); err != nil {
		return fmt.Errorf("运行 gRPC 服务失败: %w", err)
	}
	return nil
}

// Shutdown 按"先停入口、再停后台依赖、最后收资源"的顺序执行。
func (a *RelationApp) Shutdown(ctx context.Context) error {
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
			err = async.ReleasePool(a.asyncPool, time.Duration(a.asyncReleaseTimeout))
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("释放 Async 协程池失败: %w", err))
		}
	}
	if a.accountDeletedConsumer != nil {
		if err := a.accountDeletedConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Relation account.deleted 消费者失败: %w", err))
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
func (a *RelationApp) installProcessGlobals(ctx context.Context) {
	logger.ReplaceGlobal(a.logger)
	if a.db != nil {
		mysql.ReplaceGlobal(a.db)
	}
	if a.redisClient != nil {
		pkgredis.ReplaceGlobal(a.redisClient)
	}
	async.SetContextPropagator(func(parent context.Context) context.Context {
		return ctxmeta.CopyKnownFromParent(parent)
	})
	async.ReplaceGlobalWithReleaseTimeout(a.asyncPool, time.Duration(a.asyncReleaseTimeout))
	_ = ctx
}
