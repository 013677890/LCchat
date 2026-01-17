package main

import (
	"context"
	"log"

	"ChatServer/apps/user/internal/handler"
	"ChatServer/apps/user/internal/repository"
	"ChatServer/apps/user/internal/server"
	"ChatServer/apps/user/internal/service"
	userpb "ChatServer/apps/user/pb"
	"ChatServer/config"
	"ChatServer/pkg/logger"
	"ChatServer/pkg/mysql"

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

	// 3. 组装依赖 - Repository层
	userRepo := repository.NewUserRepository(db)
	relationRepo := repository.NewRelationRepository(db)
	applyRepo := repository.NewApplyRequestRepository(db)
	deviceRepo := repository.NewDeviceSessionRepository(db)

	// 4. 组装依赖 - Service层
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
}
