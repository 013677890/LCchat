package main

import (
	"ChatServer/apps/gateway/internal/middleware"
	"ChatServer/apps/gateway/internal/pb"
	"ChatServer/apps/gateway/internal/router"
	v1 "ChatServer/apps/gateway/internal/router/v1"
	"ChatServer/apps/gateway/internal/service"
	"ChatServer/config"
	"ChatServer/pkg/logger"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()

	// 1. 初始化日志
	cfg := config.DefaultLoggerConfig()
	l, err := logger.Build(cfg)
	if err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	logger.ReplaceGlobal(l)
	defer func() {
		// 同步日志缓冲区
		if err := logger.L().Sync(); err != nil {
			// Sync 在某些情况下会返回错误（如 os.Stdout），可以忽略
			_ = err
		}
	}()

	logger.Info(ctx, "Gateway 服务初始化中...")

	// 2. 初始化用户级别限流器
	// TODO: 从配置文件读取限流参数
	// 参数说明：
	//   - requestsPerSecond: 每个用户每秒允许的请求数（令牌产生速率）
	//   - burst: 令牌桶容量（允许的突发请求数）
	// 示例：10 req/s, burst 20 表示正常情况下每秒10个请求，短时间内最多20个
	middleware.InitUserRateLimiter(10, 20)
	logger.Info(ctx, "用户限流器初始化完成",
		logger.Float64("requests_per_second", 10),
		logger.Int("burst", 20),
	)

	// 3. 初始化 gRPC 客户端（依赖注入）
	// TODO: 从配置文件读取user服务地址
	userServiceAddr := "localhost:9090"

	// 3.1 创建熔断器
	userServiceBreaker := pb.CreateCircuitBreaker("user-service")
	logger.Info(ctx, "熔断器创建成功", logger.String("name", "user-service"))

	// 3.2 创建 gRPC 连接
	userServiceConn, err := pb.CreateUserServiceConnection(userServiceAddr, userServiceBreaker)
	if err != nil {
		logger.Error(ctx, "创建用户服务 gRPC 连接失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := userServiceConn.Close(); err != nil {
			logger.Error(ctx, "关闭用户服务 gRPC 连接失败", logger.ErrorField("error", err))
		}
	}()
	logger.Info(ctx, "用户服务 gRPC 连接创建成功", logger.String("address", userServiceAddr))

	// 3.3 创建 gRPC 客户端
	userClient := pb.NewUserServiceClient(userServiceConn, userServiceConn, userServiceConn, userServiceConn, userServiceConn, userServiceBreaker)
	logger.Info(ctx, "用户服务 gRPC 客户端初始化完成", logger.String("address", userServiceAddr))

	// 4. 初始化 Service 层（依赖注入）
	authService := service.NewAuthService(userClient)
	logger.Info(ctx, "认证服务初始化完成")

	userService := service.NewUserService(userClient)
	logger.Info(ctx, "用户信息服务初始化完成")

	// 5. 初始化 Handler 层（依赖注入）
	authHandler := v1.NewAuthHandler(authService)
	logger.Info(ctx, "认证处理器初始化完成")

	userHandler := v1.NewUserHandler(userService)
	logger.Info(ctx, "用户信息处理器初始化完成")

	// 6. 初始化路由（依赖注入）
	// Gin 模式设置: ReleaseMode/DebugMode/TestMode
	gin.SetMode(gin.ReleaseMode)
	r := router.InitRouter(authHandler, userHandler)
	logger.Info(ctx, "路由初始化完成")

	// 7. 配置服务器
	port := 8080 // TODO: 从配置文件读取
	addr := "127.0.0.1:" + fmt.Sprintf("%d", port)

	srv := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    10 * time.Second, // 读取超时
		WriteTimeout:   10 * time.Second, // 写入超时
		MaxHeaderBytes: 1 << 20,          // 最大请求头 1MB
	}

	// 8. 启动服务器（在 goroutine 中）
	go func() {
		logger.Info(ctx, "Gateway 服务器启动中",
			logger.String("address", addr),
			logger.Int("port", port),
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "服务器启动失败", logger.ErrorField("error", err))
			os.Exit(1)
		}
	}()

	logger.Info(ctx, "Gateway 服务器启动成功，按 Ctrl+C 关闭")

	// 9. 优雅停机
	quit := make(chan os.Signal, 1)
	// 监听中断信号：Ctrl+C (SIGINT) 和 kill 命令 (SIGTERM)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞等待信号
	sig := <-quit
	logger.Info(ctx, "收到关闭信号，开始优雅停机...",
		logger.String("signal", sig.String()),
	)

	// 10. 设置超时时间，等待正在处理的请求完成
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭 HTTP 服务器
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "服务器强制关闭", logger.ErrorField("error", err))
		os.Exit(1)
	}

	logger.Info(ctx, "Gateway 服务器已优雅退出")
}
