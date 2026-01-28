package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"ChatServer/apps/user/internal/handler"
	"ChatServer/apps/user/internal/interceptors"
	"ChatServer/apps/user/internal/repository"
	"ChatServer/apps/user/internal/server"
	"ChatServer/apps/user/internal/service"
	"ChatServer/apps/user/mq"
	userpb "ChatServer/apps/user/pb"
	"ChatServer/config"
	"ChatServer/pkg/kafka"
	"ChatServer/pkg/logger"
	"ChatServer/pkg/mysql"
	pkgredis "ChatServer/pkg/redis"
	"ChatServer/pkg/util"

	"google.golang.org/grpc"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始化日志
	logCfg := config.DefaultLoggerConfig()
	zl, err := logger.Build(logCfg)
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	logger.ReplaceGlobal(zl)
	defer zl.Sync()

	// 2. 初始化MySQL
	dbCfg := config.DefaultMySQLConfig()
	db, err := mysql.Build(dbCfg)
	if err != nil {
		log.Fatalf("初始化MySQL失败: %v", err)
	}
	mysql.ReplaceGlobal(db)

	// 3. 初始化Redis
	redisCfg := config.DefaultRedisConfig()
	// 调整 Redis 读写超时时间为 50ms（快速失败）
	redisCfg.ReadTimeout = 50 * time.Millisecond
	redisCfg.WriteTimeout = 50 * time.Millisecond

	redisClient, err := pkgredis.Build(redisCfg)
	if err != nil {
		// Redis 初始化失败不阻塞启动（降级到只用 MySQL）
		logger.Warn(ctx, "Redis 初始化失败，将降级到 MySQL-Only 模式",
			logger.ErrorField("error", err),
		)
		redisClient = nil
	} else {
		pkgredis.ReplaceGlobal(redisClient)
		logger.Info(ctx, "Redis 初始化成功",
			logger.String("addr", redisCfg.Addr),
		)
	}

	// 4. 初始化 Kafka（仅在 Redis 可用时启动）
	var kafkaProducer *kafka.Producer
	var redisConsumer *mq.RedisRetryConsumer
	if redisClient != nil {
		kafkaCfg := config.DefaultKafkaConfig()

		// 创建 Kafka Producer
		kafkaProducer = kafka.NewProducer(kafkaCfg.Brokers, kafkaCfg.RedisRetryTopic)
		mq.SetGlobalProducer(kafkaProducer)
		logger.Info(ctx, "Kafka Producer 初始化成功",
			logger.String("brokers", kafkaCfg.Brokers[0]),
			logger.String("topic", kafkaCfg.RedisRetryTopic),
		)

		// 创建 Redis 重试消费者
		zapLogger := kafka.NewZapLoggerAdapter(logger.L())
		redisConsumer = mq.NewRedisRetryConsumer(
			kafkaCfg.Brokers,
			kafkaCfg.RedisRetryTopic,
			kafkaCfg.ConsumerConfig.GroupID,
			redisClient,
			kafkaProducer,
			zapLogger,
		)

		// 启动消费者（在后台 goroutine 中运行）
		go func() {
			logger.Info(ctx, "Redis 重试消费者启动中",
				logger.String("topic", kafkaCfg.RedisRetryTopic),
				logger.String("group_id", kafkaCfg.ConsumerConfig.GroupID),
			)
			if err := redisConsumer.Start(ctx); err != nil {
				logger.Error(ctx, "Redis 重试消费者运行错误", logger.ErrorField("error", err))
			}
		}()

		// 确保程序退出时关闭 Kafka 连接
		defer func() {
			if kafkaProducer != nil {
				if err := kafkaProducer.Close(); err != nil {
					logger.Error(ctx, "关闭 Kafka Producer 失败", logger.ErrorField("error", err))
				}
			}
			if redisConsumer != nil {
				if err := redisConsumer.Close(); err != nil {
					logger.Error(ctx, "关闭 Redis 重试消费者失败", logger.ErrorField("error", err))
				}
			}
		}()
	}

	// 5. 组装依赖 - Repository 层
	authRepo := repository.NewAuthRepository(db, redisClient)
	userRepo := repository.NewUserRepository(db, redisClient)
	friendRepo := repository.NewFriendRepository(db, redisClient)
	applyRepo := repository.NewApplyRepository(db, redisClient)
	blacklistRepo := repository.NewBlacklistRepository(db, redisClient)
	deviceRepo := repository.NewDeviceRepository(db, redisClient)

	// 6. 组装依赖 - Service 层
	authService := service.NewAuthService(authRepo, deviceRepo)
	userService := service.NewUserService(userRepo, authRepo)
	friendService := service.NewFriendService(userRepo, friendRepo, applyRepo)
	blacklistService := service.NewBlacklistService(blacklistRepo)
	deviceService := service.NewDeviceService(deviceRepo)

	// 7. 组装依赖 - Handler 层
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(authService, userService, friendService, deviceService)
	friendHandler := handler.NewFriendHandler(friendService)
	blacklistHandler := handler.NewBlacklistHandler(blacklistService)
	deviceHandler := handler.NewDeviceHandler(deviceService)

	// 8. 初始化小组件
	util.InitSnowflake(1) // 雪花算法

	// 9. 启动 gRPC Server
	opts := server.Options{
		Address:          ":9090",
		EnableHealth:     true,
		EnableReflection: true, // 生产环境建议关闭
	}

	logger.Info(ctx, "准备启动用户服务", logger.String("address", opts.Address))

	if err := server.Start(ctx, opts, func(s *grpc.Server, hs healthgrpc.HealthServer) {
		// 注册认证服务
		userpb.RegisterAuthServiceServer(s, authHandler)
		// 注册用户服务
		userpb.RegisterUserServiceServer(s, userHandler)
		// 注册好友服务
		userpb.RegisterFriendServiceServer(s, friendHandler)
		// 注册黑名单服务
		userpb.RegisterBlacklistServiceServer(s, blacklistHandler)
		// 注册设备服务
		userpb.RegisterDeviceServiceServer(s, deviceHandler)

		// 设置健康检查状态
		if hs != nil {
			if setter, ok := hs.(interface {
				SetServingStatus(service string, status healthgrpc.HealthCheckResponse_ServingStatus)
			}); ok {
				setter.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
			}
		}
	}); err != nil {
		log.Fatalf("启动gRPC服务失败: %v", err)
	}

	// 10. 启动 Metrics HTTP Server（暴露 Prometheus 指标）
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", interceptors.GetMetricsHandler())

	metricsAddr := ":9091"
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: metricsMux,
	}

	go func() {
		logger.Info(ctx, "Metrics HTTP Server 启动中", logger.String("address", metricsAddr))
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "Metrics HTTP Server 启动失败", logger.ErrorField("error", err))
		}
	}()

	logger.Info(ctx, "User 服务启动成功",
		logger.String("grpc_address", opts.Address),
		logger.String("metrics_address", metricsAddr),
	)
}
