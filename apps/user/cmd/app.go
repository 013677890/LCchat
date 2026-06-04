package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/user/mq"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// UserApp 统一管理 user 服务生命周期。
type UserApp struct {
	logger                 *zap.Logger
	metricsServer          *http.Server
	grpcServer             *grpc.Server
	grpcListener           net.Listener
	grpcShutdownTimeout    time.Duration
	redisConsumer          *mq.RedisRetryConsumer
	userCreatedConsumer    *consumer.UserCreatedConsumer
	accountDeletedConsumer *consumer.AccountDeletedConsumer
	kafkaProducer          *kafka.Producer
	asyncPool              *ants.Pool
	asyncReleaseTimeout    time.Duration
	db                     *gorm.DB
	redisClient            *goredis.Client
}

// NewUserApp 只做资源聚合与合法性校验，不在构造阶段启动任何阻塞逻辑。
func NewUserApp(
	log *zap.Logger,
	metricsServer *http.Server,
	built *grpcx.BuiltServer,
	grpcListener net.Listener,
	grpcShutdownTimeout userGRPCShutdownTimeout,
	redisConsumer *mq.RedisRetryConsumer,
	userCreatedConsumer *consumer.UserCreatedConsumer,
	accountDeletedConsumer *consumer.AccountDeletedConsumer,
	kafkaProducer *kafka.Producer,
	asyncPool *ants.Pool,
	asyncReleaseTimeout userAsyncReleaseTimeout,
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
		logger:                 log,
		metricsServer:          metricsServer,
		grpcServer:             built.Server,
		grpcListener:           grpcListener,
		grpcShutdownTimeout:    time.Duration(grpcShutdownTimeout),
		redisConsumer:          redisConsumer,
		userCreatedConsumer:    userCreatedConsumer,
		accountDeletedConsumer: accountDeletedConsumer,
		kafkaProducer:          kafkaProducer,
		asyncPool:              asyncPool,
		asyncReleaseTimeout:    time.Duration(asyncReleaseTimeout),
		db:                     db,
		redisClient:            redisClient,
	}, nil
}

// Run 负责启动所有长生命周期组件。
// 约束：后台 worker 先启动，最后由 gRPC Serve 阻塞当前 goroutine，
// 这样 main 只需要感知一个运行入口。
func (a *UserApp) Run(ctx context.Context) error {
	a.installProcessGlobals(ctx)

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

	if a.userCreatedConsumer != nil {
		go func() {
			if err := a.userCreatedConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error(ctx, "User user_created 消费者运行错误", logger.ErrorField("error", err))
			}
		}()
	}

	if a.accountDeletedConsumer != nil {
		go func() {
			if err := a.accountDeletedConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error(ctx, "User account.deleted 消费者运行错误", logger.ErrorField("error", err))
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
	if a.userCreatedConsumer != nil {
		if err := a.userCreatedConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 User user_created 消费者失败: %w", err))
		}
	}
	if a.accountDeletedConsumer != nil {
		if err := a.accountDeletedConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 User account.deleted 消费者失败: %w", err))
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
	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}

// installProcessGlobals 在对外提供服务前注册全局 logger / DB / Redis / 异步池等，
// 与 Snowflake、邮件、设备在线窗口、Kafka 全局 Producer 等进程级副作用。
// 放在 Run 而非 Wire Provider，避免在依赖装配阶段产生隐式全局状态。
func (a *UserApp) installProcessGlobals(ctx context.Context) {
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
	_ = util.InitSnowflake(1)
	util.SetEmailConfig(util.EmailConfig{
		SMTPHost:     userGetEnv("EMAIL_SMTP_HOST", "smtp.qq.com"),
		SMTPPort:     userGetEnvInt("EMAIL_SMTP_PORT", 465),
		SenderEmail:  userGetEnv("EMAIL_SENDER", "2315635418@qq.com"),
		SenderName:   userGetEnv("EMAIL_SENDER_NAME", "LCChat"),
		AuthPassword: os.Getenv("EMAIL_AUTH_CODE"),
	})
	if a.kafkaProducer != nil {
		mq.SetGlobalProducer(a.kafkaProducer)
	}
	_ = ctx
}

func userGetEnv(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	return v
}

func userGetEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
}
