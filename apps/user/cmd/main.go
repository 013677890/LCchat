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
	userpb "ChatServer/apps/user/pb"
	"ChatServer/config"
	"ChatServer/pkg/logger"
	"ChatServer/pkg/mysql"
	pkgredis "ChatServer/pkg/redis"

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
	// 调整 Redis 超时时间为 50ms（快速失败）
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

	// 4. 组装依赖 - Repository层（注入 Redis）
	userRepo := repository.NewUserRepository(db, redisClient)
	relationRepo := repository.NewRelationRepository(db)
	applyRepo := repository.NewApplyRequestRepository(db)
	deviceRepo := repository.NewDeviceSessionRepository(db)

	// 5. 组装依赖 - Service层
	authService := service.NewAuthService(userRepo, deviceRepo)
	userQueryService := service.NewUserInfoService(userRepo)
	friendService := service.NewFriendService(userRepo, relationRepo, applyRepo)
	deviceService := service.NewDeviceService(deviceRepo)

	// 5. 组装依赖 - Handler层
	userHandler := handler.NewUserServiceHandler(
		authService,
		userQueryService,
		friendService,
		deviceService,
	)

	// 6. 启动gRPC Server
	opts := server.Options{
		Address:          ":9090",
		EnableHealth:     true,
		EnableReflection: true, // 生产环境建议关闭
	}

	logger.Info(ctx, "准备启动用户服务", logger.String("address", opts.Address))

	if err := server.Start(ctx, opts, func(s *grpc.Server, hs healthgrpc.HealthServer) {
		// 注册用户服务
		userpb.RegisterUserServiceServer(s, userHandler)

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

	// 7. 启动 Metrics HTTP Server（暴露 Prometheus 指标）
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
