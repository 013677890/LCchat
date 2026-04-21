package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/consumer"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// MessagePushApp 统一管理 message-push 生命周期。
type MessagePushApp struct {
	logger     *zap.Logger
	consumer   *consumer.Consumer
	redis      *goredis.Client
	connectCli *connectcli.ClientManager
}

// NewMessagePushApp 创建 app。
func NewMessagePushApp(log *zap.Logger, consumer *consumer.Consumer, redis *goredis.Client, connectCli *connectcli.ClientManager) (*MessagePushApp, error) {
	if log == nil {
		return nil, errors.New("logger 未初始化")
	}
	if consumer == nil {
		return nil, errors.New("consumer 未初始化")
	}
	if connectCli == nil {
		return nil, errors.New("connect client manager 未初始化")
	}
	return &MessagePushApp{logger: log, consumer: consumer, redis: redis, connectCli: connectCli}, nil
}

// Run 启动服务。
func (a *MessagePushApp) Run(ctx context.Context) error {
	// 启动后先替换全局 logger，确保后续包级日志都落到 message-push 自己的日志配置。
	logger.ReplaceGlobal(a.logger)
	logger.Info(ctx, "message-push 服务启动中")
	if err := a.consumer.Start(ctx); err != nil {
		return fmt.Errorf("运行 message-push consumer 失败: %w", err)
	}
	return nil
}

// Shutdown 关闭资源。
func (a *MessagePushApp) Shutdown(ctx context.Context) error {
	var errs []error
	if a.connectCli != nil {
		// 先关 connect 客户端连接池，避免退出过程中仍有下游连接悬挂。
		if err := a.connectCli.Close(ctx); err != nil {
			errs = append(errs, err)
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
