package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/consumer"
	mpserver "github.com/013677890/LCchat-Backend/apps/message-push/internal/server"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// MessagePushApp 统一管理 message-push 生命周期。
type MessagePushApp struct {
	logger        *zap.Logger
	msgConsumer   *consumer.Consumer
	realConsumer  *consumer.Consumer
	redis         *goredis.Client
	connectCli    *connectcli.ClientManager
	groupGRPCConn *grpc.ClientConn
	httpServer    *mpserver.Server
}

// pushConsumers 聚合 message-push 需要同时启动的 Kafka 消费者。
type pushConsumers struct {
	msg      *consumer.Consumer
	realtime *consumer.Consumer
}

// NewMessagePushApp 创建 app。
func NewMessagePushApp(log *zap.Logger, consumers pushConsumers, redis *goredis.Client, connectCli *connectcli.ClientManager, groupGRPCConn *grpc.ClientConn, httpServer *mpserver.Server) (*MessagePushApp, error) {
	if log == nil {
		return nil, errors.New("logger 未初始化")
	}
	if consumers.msg == nil {
		return nil, errors.New("msg.push consumer 未初始化")
	}
	if consumers.realtime == nil {
		return nil, errors.New("realtime.push consumer 未初始化")
	}
	if connectCli == nil {
		return nil, errors.New("connect client manager 未初始化")
	}
	if groupGRPCConn == nil {
		return nil, errors.New("group-service gRPC 连接未初始化")
	}
	if httpServer == nil {
		return nil, errors.New("http server 未初始化")
	}
	return &MessagePushApp{logger: log, msgConsumer: consumers.msg, realConsumer: consumers.realtime, redis: redis, connectCli: connectCli, groupGRPCConn: groupGRPCConn, httpServer: httpServer}, nil
}

// Run 启动服务。
func (a *MessagePushApp) Run(ctx context.Context) error {
	// 启动后先替换全局 logger，确保后续包级日志都落到 message-push 自己的日志配置。
	logger.ReplaceGlobal(a.logger)
	logger.Info(ctx, "message-push 服务启动中")

	httpErrCh := make(chan error, 1)
	go func() {
		httpErrCh <- a.httpServer.Start()
	}()
	logger.Info(ctx, "message-push 指标 HTTP 服务已启动",
		logger.String("addr", a.httpServer.Addr()),
	)

	msgConsumerErrCh := make(chan error, 1)
	go func() {
		msgConsumerErrCh <- a.msgConsumer.Start(ctx)
	}()

	realtimeConsumerErrCh := make(chan error, 1)
	go func() {
		realtimeConsumerErrCh <- a.realConsumer.Start(ctx)
	}()

	select {
	case err := <-httpErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("运行 message-push http server 失败: %w", err)
		}
		return nil
	case err := <-msgConsumerErrCh:
		if err != nil {
			return fmt.Errorf("运行 msg.push consumer 失败: %w", err)
		}
		return nil
	case err := <-realtimeConsumerErrCh:
		if err != nil {
			return fmt.Errorf("运行 realtime.push consumer 失败: %w", err)
		}
		return nil
	case <-ctx.Done():
		return nil
	}
}

// Shutdown 关闭资源。
func (a *MessagePushApp) Shutdown(ctx context.Context) error {
	var errs []error
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("关闭 HTTP server 失败: %w", err))
		}
	}
	if a.connectCli != nil {
		// 先关 connect 客户端连接池，避免退出过程中仍有下游连接悬挂。
		if err := a.connectCli.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if a.groupGRPCConn != nil {
		if err := a.groupGRPCConn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 group-service gRPC 连接失败: %w", err))
		}
	}
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Redis 客户端失败: %w", err))
		}
	}
	if a.logger != nil {
		_ = a.logger.Sync()
	}
	return errors.Join(errs...)
}
