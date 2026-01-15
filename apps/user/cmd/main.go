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

	// 1) 初始化日志
	logCfg := config.DefaultLoggerConfig()
	zl, err := logger.Build(logCfg)
	if err != nil {
		log.Fatalf("init logger failed: %v", err)
	}
	logger.ReplaceGlobal(zl)
	defer zl.Sync() // 确保缓冲写出

	// 2) 初始化 MySQL
	dbCfg := config.DefaultMySQLConfig()
	db, err := mysql.Build(dbCfg)
	if err != nil {
		log.Fatalf("init mysql failed: %v", err)
	}
	mysql.ReplaceGlobal(db)

	// 3) 组装依赖
	userRepo := repository.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo)
	userHandler := handler.NewUserServiceHandler(userSvc)

	// 4) 启动 gRPC Server
	opts := server.Options{
		Address:          ":9090",
		EnableHealth:     true,
		EnableReflection: true, // 生产可关
	}

	if err := server.Start(ctx, opts, func(s *grpc.Server, hs healthgrpc.HealthServer) {
		userpb.RegisterUserServiceServer(s, userHandler)
		if hs != nil {
			// 设置健康状态（需要类型断言）
			if setter, ok := hs.(interface {
				SetServingStatus(service string, status healthgrpc.HealthCheckResponse_ServingStatus)
			}); ok {
				setter.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
			}
		}
	}); err != nil {
		log.Fatalf("start grpc server failed: %v", err)
	}
}
