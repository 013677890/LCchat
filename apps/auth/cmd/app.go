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

	"github.com/013677890/LCchat-Backend/apps/auth/mq"
	"github.com/013677890/LCchat-Backend/config"
	pkgdeviceactive "github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/util"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// AuthApp 统一管理 auth 服务生命周期。
type AuthApp struct {
	logger              *zap.Logger
	metricsServer       *http.Server
	grpcServer          *grpc.Server
	grpcListener        net.Listener
	grpcShutdownTimeout time.Duration
	redisConsumer       *mq.RedisRetryConsumer
	kafkaProducer       *kafka.Producer
	db                  *gorm.DB
	redisClient         *goredis.Client
}

// NewAuthApp 只做资源聚合与合法性校验，不在构造阶段启动阻塞逻辑。
func NewAuthApp(
	log *zap.Logger,
	metricsServer *http.Server,
	built *grpcx.BuiltServer,
	grpcListener net.Listener,
	grpcShutdownTimeout authGRPCShutdownTimeout,
	redisConsumer *mq.RedisRetryConsumer,
	kafkaProducer *kafka.Producer,
	db *gorm.DB,
	redisClient *goredis.Client,
) (*AuthApp, error) {
	if built == nil || built.Server == nil {
		return nil, errors.New("grpc server 未初始化")
	}
	if grpcListener == nil {
		return nil, errors.New("grpc listener 未初始化")
	}
	return &AuthApp{
		logger:              log,
		metricsServer:       metricsServer,
		grpcServer:          built.Server,
		grpcListener:        grpcListener,
		grpcShutdownTimeout: time.Duration(grpcShutdownTimeout),
		redisConsumer:       redisConsumer,
		kafkaProducer:       kafkaProducer,
		db:                  db,
		redisClient:         redisClient,
	}, nil
}

// Run 启动 auth 服务的长生命周期组件。
func (a *AuthApp) Run(ctx context.Context) error {
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

	logger.Info(ctx, "Auth 服务启动中", logger.String("grpc_address", a.grpcListener.Addr().String()))
	if err := grpcx.Run(ctx, a.grpcServer, a.grpcListener); err != nil {
		return fmt.Errorf("运行 gRPC 服务失败: %w", err)
	}
	return nil
}

// Shutdown 按“先停入口、再停后台依赖、最后收资源”的顺序执行关闭。
func (a *AuthApp) Shutdown(ctx context.Context) error {
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
	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}

// installProcessGlobals 在服务启动前安装进程级全局依赖与配置。
func (a *AuthApp) installProcessGlobals(ctx context.Context) {
	logger.ReplaceGlobal(a.logger)
	if a.db != nil {
		mysql.ReplaceGlobal(a.db)
	}
	if a.redisClient != nil {
		pkgredis.ReplaceGlobal(a.redisClient)
	}
	_ = util.InitSnowflake(1)
	util.SetEmailConfig(util.EmailConfig{
		SMTPHost:     authGetEnv("EMAIL_SMTP_HOST", "smtp.qq.com"),
		SMTPPort:     authGetEnvInt("EMAIL_SMTP_PORT", 465),
		SenderEmail:  authGetEnv("EMAIL_SENDER", "2315635418@qq.com"),
		SenderName:   authGetEnv("EMAIL_SENDER_NAME", "LCChat"),
		AuthPassword: os.Getenv("EMAIL_AUTH_CODE"),
	})
	deviceCfg := config.DefaultDeviceActiveConfig()
	pkgdeviceactive.SetOnlineWindow(deviceCfg.OnlineWindow)
	if a.kafkaProducer != nil {
		mq.SetGlobalProducer(a.kafkaProducer)
	}
	_ = ctx
}

func authGetEnv(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	return v
}

func authGetEnvInt(key string, defaultValue int) int {
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
