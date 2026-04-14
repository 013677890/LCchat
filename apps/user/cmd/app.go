package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/mq"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// UserApp 统一管理 user 服务生命周期。
type UserApp struct {
	logger              *zap.Logger
	metricsServer       *http.Server
	grpcServer          *grpc.Server
	grpcListener        net.Listener
	grpcShutdownTimeout time.Duration
	redisConsumer       *mq.RedisRetryConsumer
	kafkaProducer       *kafka.Producer
	asyncPool           *ants.Pool
	asyncReleaseTimeout time.Duration
	db                  *gorm.DB
	redisClient         *goredis.Client
}

// NewUserApp 只做资源聚合与合法性校验，不在构造阶段启动任何阻塞逻辑。
func NewUserApp(
	log *zap.Logger,
	metricsServer *http.Server,
	built *grpcx.BuiltServer,
	grpcListener net.Listener,
	grpcShutdownTimeout time.Duration,
	redisConsumer *mq.RedisRetryConsumer,
	kafkaProducer *kafka.Producer,
	asyncPool *ants.Pool,
	asyncReleaseTimeout time.Duration,
	db *gorm.DB,
	redisClient *goredis.Client,
) (*UserApp, error) {
	if built == nil || built.Server == nil {
		return nil, errors.New("grpc server 未初始化")
	}
	if grpcListener == nil {
		return nil, errors.New("grpc listener 未初始化")
	}
	return &UserApp{
		logger:              log,
		metricsServer:       metricsServer,
		grpcServer:          built.Server,
		grpcListener:        grpcListener,
		grpcShutdownTimeout: grpcShutdownTimeout,
		redisConsumer:       redisConsumer,
		kafkaProducer:       kafkaProducer,
		asyncPool:           asyncPool,
		asyncReleaseTimeout: asyncReleaseTimeout,
		db:                  db,
		redisClient:         redisClient,
	}, nil
}

// Run 负责启动所有长生命周期组件。
// 约束：后台 worker 先启动，最后由 gRPC Serve 阻塞当前 goroutine，
// 这样 main 只需要感知一个运行入口。
func (a *UserApp) Run(ctx context.Context) error {
	if a.metricsServer != nil {
		go func() {
			logger.Info(ctx, "Metrics HTTP Server 启动中", logger.String("address", a.metricsServer.Addr))
			if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error(ctx, "Metrics HTTP Server 启动失败", logger.ErrorField("error", err))
			}
		}()
	}

	if a.redisConsumer != nil {
		go func() {
			logger.Info(ctx, "Redis 重试消费者启动中")
			if err := a.redisConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error(ctx, "Redis 重试消费者运行错误", logger.ErrorField("error", err))
			}
		}()
	}

	logger.Info(ctx, "User 服务启动中", logger.String("grpc_address", a.grpcListener.Addr().String()))
	if err := grpcx.Run(ctx, a.grpcServer, a.grpcListener); err != nil {
		return fmt.Errorf("运行 gRPC 服务失败: %w", err)
	}
	return nil
}

// Shutdown 按“先停入口、再停后台依赖、最后收资源”的顺序执行，
// 避免新请求继续进入时底层资源已被提前关闭。
func (a *UserApp) Shutdown(ctx context.Context) error {
	var errs []error

	if a.metricsServer != nil {
		if err := a.metricsServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("关闭 metrics server 失败: %w", err))
		}
	}
	// 先停止 gRPC 对外入口，阻止新请求继续进入。
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
	if a.redisConsumer != nil {
		if err := a.redisConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Redis 重试消费者失败: %w", err))
		}
	}
	if a.kafkaProducer != nil {
		if err := a.kafkaProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Kafka Producer 失败: %w", err))
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
	if a.asyncPool != nil {
		if a.asyncReleaseTimeout > 0 {
			if err := a.asyncPool.ReleaseTimeout(a.asyncReleaseTimeout); err != nil {
				errs = append(errs, fmt.Errorf("释放 Async 协程池失败: %w", err))
			}
		} else {
			a.asyncPool.Release()
		}
	}
	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}
