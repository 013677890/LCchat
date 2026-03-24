package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/router"
	v1 "github.com/013677890/LCchat-Backend/apps/gateway/internal/router/v1"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/config"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"

	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()
	//设置trace_id 为 0
	traceId := "0"
	ctx = ctxmeta.WithTraceID(ctx, traceId)

	// 1. 初始化日志（必须先于任何 logger 调用）
	cfg := config.DefaultLoggerConfig()
	l, err := logger.Build(cfg)
	if err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	logger.ReplaceGlobal(l)
	defer func() {
		// Sync 在某些输出目标上可能返回错误（如 stdout），这里忽略
		_ = l.Sync()
	}()

	// 2. 初始化 Redis
	redisCfg := config.DefaultRedisConfig()
	redisClient, err := pkgredis.Build(redisCfg)
	if err != nil {
		logger.Error(ctx, "初始化 Redis 失败",
			logger.ErrorField("error", err),
		)
		// Redis 初始化失败不阻塞启动，但限流功能将降级
		redisClient = nil
	} else {
		pkgredis.ReplaceGlobal(redisClient)
		logger.Info(ctx, "Redis 初始化成功",
			logger.String("addr", redisCfg.Addr),
		)
	}

	// 2.5 初始化 Async 协程池
	async.SetContextPropagator(func(parent context.Context) context.Context {
		return ctxmeta.CopyKnownFromParent(parent)
	})

	asyncCfg := config.DefaultAsyncConfig()
	if err := async.Init(asyncCfg); err != nil {
		logger.Error(ctx, "初始化 Async 协程池失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := async.Release(); err != nil {
			logger.Error(ctx, "释放 Async 协程池失败", logger.ErrorField("error", err))
		}
	}()
	logger.Info(ctx, "Async 协程池初始化完成", logger.Int("pool_size", asyncCfg.PoolSize))

	logger.Info(ctx, "Gateway 服务初始化中...")

	// 3. 初始化 MinIO 对象存储
	minioCfg := config.DefaultMinIOConfig()
	minioClient, err := pkgminio.Build(minioCfg)
	if err != nil {
		logger.Error(ctx, "初始化 MinIO 失败",
			logger.ErrorField("error", err),
		)
		// MinIO 初始化失败不阻塞启动，但文件上传功能将不可用
		minioClient = nil
	} else {
		pkgminio.ReplaceGlobal(minioClient)
		logger.Info(ctx, "MinIO 初始化成功",
			logger.String("endpoint", minioCfg.Endpoint),
			logger.String("bucket", minioCfg.BucketName),
			logger.Bool("use_ssl", minioCfg.UseSSL),
		)
	}

	// 4. 初始化 Redis IP 限流器
	// 参数说明：
	//   - rate: 每秒产生的令牌数 (10.0 表示每秒10个令牌)
	//   - burst: 令牌桶容量 (20 表示桶最多20个令牌)
	//   - redisClient: Redis 客户端实例
	// 示例：10 req/s, burst 20 表示正常情况下每秒10个请求，短时间内最多20个
	middleware.InitRedisRateLimiter(10.0, 20, redisClient)
	logger.Info(ctx, "Redis IP 限流器初始化完成",
		logger.Float64("rate", 10.0),
		logger.Int("burst", 20),
		logger.String("blacklist_key", rediskey.GatewayIPBlacklistKey()),
	)

	// 4.5 读取设备活跃同步配置（实际初始化在 gRPC 客户端创建后执行）
	deviceActiveCfg := config.DefaultDeviceActiveConfig()
	deviceactive.SetOnlineWindow(deviceActiveCfg.OnlineWindow)
	logger.Info(ctx, "设备活跃配置已加载",
		logger.Duration("update_interval", deviceActiveCfg.UpdateInterval),
		logger.Duration("flush_interval", deviceActiveCfg.FlushInterval),
		logger.Duration("online_window", deviceActiveCfg.OnlineWindow),
		logger.Int("shard_count", deviceActiveCfg.ShardCount),
		logger.Int("worker_count", deviceActiveCfg.WorkerCount),
		logger.Int("queue_size", deviceActiveCfg.QueueSize),
	)

	// 5. 初始化 gRPC 客户端（依赖注入）
	userServiceAddr := os.Getenv("USER_SERVICE_ADDR")
	if userServiceAddr == "" {
		userServiceAddr = "localhost:9090"
	}

	// 5.1 创建熔断器
	userServiceBreaker := pb.CreateCircuitBreaker("user-service")
	logger.Info(ctx, "熔断器创建成功", logger.String("name", "user-service"))

	// 5.2 创建 gRPC 连接
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

	// 5.2.1 初始化设备活跃时间同步器（分片节流 map + 缓冲 map 批量消费）
	deviceRPCClient := userpb.NewDeviceServiceClient(userServiceConn)
	if err := middleware.InitDeviceActiveSyncer(deviceActiveCfg, func(_ context.Context, items []deviceactive.BatchItem) error {
		const batchSize = 1000
		var firstErr error
		for start := 0; start < len(items); start += batchSize {
			end := start + batchSize
			if end > len(items) {
				end = len(items)
			}
			activeItems := make([]*userpb.UpdateDeviceActiveItem, 0, end-start)
			for i := start; i < end; i++ {
				activeItems = append(activeItems, &userpb.UpdateDeviceActiveItem{
					UserUuid: items[i].UserUUID,
					DeviceId: items[i].DeviceID,
				})
			}

			rpcCtx, cancel := context.WithTimeout(context.Background(), deviceActiveCfg.RPCTimeout)
			_, err := deviceRPCClient.UpdateDeviceActive(rpcCtx, &userpb.UpdateDeviceActiveRequest{Items: activeItems})
			cancel()
			if err != nil && firstErr == nil {
				firstErr = err
				logger.Error(ctx, "批量更新设备活跃时间失败",
					logger.ErrorField("error", err),
					logger.Int("item_count", len(activeItems)),
				)
			}
		}
		return firstErr
	}); err != nil {
		logger.Error(ctx, "设备活跃同步器初始化失败",
			logger.ErrorField("error", err),
		)
	} else {
		logger.Info(ctx, "设备活跃同步器初始化完成",
			logger.Int("shard_count", deviceActiveCfg.ShardCount),
			logger.Duration("update_interval", deviceActiveCfg.UpdateInterval),
			logger.Duration("flush_interval", deviceActiveCfg.FlushInterval),
			logger.Duration("online_window", deviceActiveCfg.OnlineWindow),
			logger.Int("worker_count", deviceActiveCfg.WorkerCount),
			logger.Int("queue_size", deviceActiveCfg.QueueSize),
		)
	}
	defer middleware.ShutdownDeviceActiveSyncer()

	// 5.3 创建 gRPC 客户端
	userClient := pb.NewUserServiceClient(userServiceConn, userServiceConn, userServiceConn, userServiceConn, userServiceConn, userServiceBreaker)
	logger.Info(ctx, "用户服务 gRPC 客户端初始化完成", logger.String("address", userServiceAddr))

	// 5.4 初始化消息服务 gRPC 连接
	msgServiceAddr := os.Getenv("MSG_SERVICE_ADDR")
	if msgServiceAddr == "" {
		msgServiceAddr = "localhost:9092"
	}

	msgServiceBreaker := pb.CreateCircuitBreaker("msg-service")
	logger.Info(ctx, "消息服务熔断器创建成功", logger.String("name", "msg-service"))

	msgServiceConn, err := pb.CreateMsgServiceConnection(msgServiceAddr, msgServiceBreaker)
	if err != nil {
		logger.Error(ctx, "创建消息服务 gRPC 连接失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := msgServiceConn.Close(); err != nil {
			logger.Error(ctx, "关闭消息服务 gRPC 连接失败", logger.ErrorField("error", err))
		}
	}()
	logger.Info(ctx, "消息服务 gRPC 连接创建成功", logger.String("address", msgServiceAddr))

	// 5.5 创建消息服务 gRPC 客户端
	msgClient := pb.NewMsgServiceClient(msgServiceConn, msgServiceBreaker)
	logger.Info(ctx, "消息服务 gRPC 客户端初始化完成", logger.String("address", msgServiceAddr))

	// 6. 初始化 Service 层（依赖注入）
	authService := service.NewAuthService(userClient)
	logger.Info(ctx, "认证服务初始化完成")

	userService := service.NewUserService(userClient)
	logger.Info(ctx, "用户信息服务初始化完成")

	friendService := service.NewFriendService(userClient)
	logger.Info(ctx, "好友服务初始化完成")

	blacklistService := service.NewBlacklistService(userClient)
	logger.Info(ctx, "黑名单服务初始化完成")

	deviceService := service.NewDeviceService(userClient)
	logger.Info(ctx, "设备服务初始化完成")

	msgService := service.NewMsgService(msgClient)
	logger.Info(ctx, "消息服务初始化完成")

	// 7. 初始化 Handler 层（依赖注入）
	authHandler := v1.NewAuthHandler(authService)
	logger.Info(ctx, "认证处理器初始化完成")

	userHandler := v1.NewUserHandler(userService)
	logger.Info(ctx, "用户信息处理器初始化完成")

	friendHandler := v1.NewFriendHandler(friendService)
	logger.Info(ctx, "好友处理器初始化完成")

	blacklistHandler := v1.NewBlacklistHandler(blacklistService)
	logger.Info(ctx, "黑名单处理器初始化完成")

	deviceHandler := v1.NewDeviceHandler(deviceService)
	logger.Info(ctx, "设备处理器初始化完成")

	msgHandler := v1.NewMsgHandler(msgService)
	logger.Info(ctx, "消息处理器初始化完成")

	// 8. 初始化路由（依赖注入）
	// Gin 模式设置: ReleaseMode/DebugMode/TestMode
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)
	r := router.InitRouter(authHandler, userHandler, friendHandler, blacklistHandler, deviceHandler, msgHandler)
	logger.Info(ctx, "路由初始化完成")

	// 9. 配置服务器
	addr := os.Getenv("GATEWAY_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    10 * time.Second, // 读取超时
		WriteTimeout:   10 * time.Second, // 写入超时
		MaxHeaderBytes: 1 << 20,          // 最大请求头 1MB
	}

	// 10. 启动服务器（在 goroutine 中）
	go func() {
		logger.Info(ctx, "Gateway 服务器启动中",
			logger.String("address", addr),
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "服务器启动失败", logger.ErrorField("error", err))
			os.Exit(1)
		}
	}()

	logger.Info(ctx, "Gateway 服务器启动成功，按 Ctrl+C 关闭")

	// 11. 优雅停机
	quit := make(chan os.Signal, 1)
	// 监听中断信号：Ctrl+C (SIGINT) 和 kill 命令 (SIGTERM)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞等待信号
	sig := <-quit
	logger.Info(ctx, "收到关闭信号，开始优雅停机...",
		logger.String("signal", sig.String()),
	)

	// 12. 设置超时时间，等待正在处理的请求完成
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭 HTTP 服务器
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "服务器强制关闭", logger.ErrorField("error", err))
		os.Exit(1)
	}

	logger.Info(ctx, "Gateway 服务器已优雅退出")
}
