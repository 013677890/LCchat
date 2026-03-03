package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usecase"
	"github.com/013677890/LCchat-Backend/apps/msg/mq"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/mysql"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"google.golang.org/grpc"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ============================================================
	// 1. 初始化日志
	// ============================================================
	logCfg := config.DefaultLoggerConfig()
	zl, err := logger.Build(logCfg)
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	logger.ReplaceGlobal(zl)
	defer zl.Sync()

	// ============================================================
	// 2. 初始化 Async 协程池
	// ============================================================
	async.SetContextPropagator(func(parent context.Context) context.Context {
		return ctxmeta.CopyKnownFromParent(parent)
	})

	asyncCfg := config.DefaultAsyncConfig()
	if err := async.Init(asyncCfg); err != nil {
		log.Fatalf("初始化 Async 协程池失败: %v", err)
	}
	defer func() {
		if err := async.Release(); err != nil {
			logger.Error(ctx, "释放 Async 协程池失败", logger.ErrorField("error", err))
		}
	}()
	logger.Info(ctx, "Async 协程池初始化完成", logger.Int("pool_size", asyncCfg.PoolSize))

	// ============================================================
	// 3. 初始化 MySQL
	// ============================================================
	dbCfg := config.DefaultMySQLConfig()
	db, err := mysql.Build(dbCfg)
	if err != nil {
		log.Fatalf("初始化 MySQL 失败: %v", err)
	}
	mysql.ReplaceGlobal(db)
	logger.Info(ctx, "MySQL 初始化成功")

	// ============================================================
	// 4. 初始化 Redis
	// ============================================================
	redisCfg := config.DefaultRedisConfig()
	redisCfg.ReadTimeout = 50 * time.Millisecond
	redisCfg.WriteTimeout = 50 * time.Millisecond

	redisClient, err := pkgredis.Build(redisCfg)
	if err != nil {
		// msg-service 强依赖 Redis（seq 分配 + 幂等锁），Redis 不可用则无法正常工作
		log.Fatalf("初始化 Redis 失败: %v", err)
	}
	pkgredis.ReplaceGlobal(redisClient)
	logger.Info(ctx, "Redis 初始化成功", logger.String("addr", redisCfg.Addr))

	// ============================================================
	// 5. 初始化 Kafka Producer（msg.push topic）
	// ============================================================
	kafkaCfg := config.DefaultKafkaConfig()
	kafkaProducer := kafka.NewProducer(kafkaCfg.Brokers, kafkaCfg.MsgPushTopic)
	defer func() {
		if err := kafkaProducer.Close(); err != nil {
			logger.Error(ctx, "关闭 Kafka Producer 失败", logger.ErrorField("error", err))
		}
	}()
	logger.Info(ctx, "Kafka Producer 初始化成功",
		logger.String("brokers", kafkaCfg.Brokers[0]),
		logger.String("topic", kafkaCfg.MsgPushTopic),
	)

	// 封装为 msg 模块的 Producer（附加 topic 和序列化逻辑）
	msgProducer := mq.NewProducer(kafkaProducer, kafkaCfg.MsgPushTopic)

	// ============================================================
	// 6. 初始化小组件
	// ============================================================
	util.InitSnowflake(2) // 节点 ID=2（与 user-service 节点 ID=1 区分）

	// ============================================================
	// 7. 组装依赖 - Repository 层
	// ============================================================
	msgRepo := message.NewRepository(db, redisClient)
	convRepo := conversation.NewRepository(db)
	logger.Info(ctx, "Repository 层组装完成")

	// ============================================================
	// 8. 组装依赖 - Service 层
	// ============================================================
	msgService := message.NewService(msgRepo)
	convService := conversation.NewService(convRepo)
	logger.Info(ctx, "Service 层组装完成")

	// ============================================================
	// 9. 组装依赖 - Usecase 层（跨领域 Workflow）
	// ============================================================
	sendWf := usecase.NewSendMessageWorkflow(msgService, convService, msgProducer)
	recallWf := usecase.NewRecallMessageWorkflow(msgService, msgProducer)
	markReadWf := usecase.NewMarkReadWorkflow(convService, msgProducer)
	logger.Info(ctx, "Usecase 层组装完成")

	// ============================================================
	// 10. 组装依赖 - Handler 层
	// ============================================================
	msgHandler := handler.NewMsgHandler(msgService, convService, sendWf, recallWf, markReadWf)
	logger.Info(ctx, "Handler 层组装完成")

	// ============================================================
	// 11. 启动 Metrics HTTP Server
	// ============================================================
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", grpcx.DefaultHandler())

	metricsAddr := os.Getenv("MSG_METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9093"
	}
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

	// ============================================================
	// 12. 启动 gRPC Server（阻塞直到服务停止）
	// ============================================================
	grpcAddr := os.Getenv("MSG_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":9092"
	}

	opts := grpcx.ServerOptions{
		Address:          grpcAddr,
		Namespace:        "msg",
		EnableHealth:     true,
		EnableReflection: true, // 生产环境建议关闭
	}

	logger.Info(ctx, "Msg 服务启动中",
		logger.String("grpc_address", grpcAddr),
		logger.String("metrics_address", metricsAddr),
	)

	if _, err := grpcx.Start(ctx, opts, func(s *grpc.Server, hs healthgrpc.HealthServer) {
		msgpb.RegisterMsgServiceServer(s, msgHandler)

		if hs != nil {
			if setter, ok := hs.(interface {
				SetServingStatus(service string, status healthgrpc.HealthCheckResponse_ServingStatus)
			}); ok {
				setter.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
			}
		}
	}); err != nil {
		log.Fatalf("启动 gRPC 服务失败: %v", err)
	}
}
