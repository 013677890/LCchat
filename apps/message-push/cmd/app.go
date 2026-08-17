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
// 与 auth/user 等 API 服务不同：本进程主业是 Kafka 消费 + 调 connect 下行，
// 因此任一消费 Pool 致命失败应导致 Run 返回错误并由 main 非零退出。
type MessagePushApp struct {
	logger        *zap.Logger
	msgConsumer   *consumer.Consumer
	realConsumer  *consumer.Consumer
	redis         *goredis.Client
	connectCli    *connectcli.ClientManager
	groupGRPCConn *grpc.ClientConn
	httpServer    *mpserver.Server
}

// pushConsumers 聚合 message-push 需要同时启动的两个 Kafka 消费 Pool。
// msg 与 realtime 使用不同 topic/group，互不抢消息。
type pushConsumers struct {
	msg      *consumer.Consumer
	realtime *consumer.Consumer
}

// NewMessagePushApp 聚合依赖并做空指针校验，不在构造阶段启动阻塞逻辑。
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

// Run 启动 metrics HTTP 与两个消费 Pool，并阻塞到致命失败或父 ctx 取消。
//
// 失败策略（message-push 专用，勿套用到 API 服务）：
//   - 任一消费 Pool 非 Canceled 致命退出：立刻 cancel 兄弟 Pool，返回 error，
//     由 main 非零退出，禁止只剩一个 topic 在跑的半残实例；
//   - Kafka 临时错误不会走到这里（在 Reader 循环内退避）；
//   - 父 ctx 取消：正常关停路径，返回 nil，再由 Shutdown 关资源。
func (a *MessagePushApp) Run(ctx context.Context) error {
	// 启动后先替换全局 logger，确保后续包级日志都落到 message-push 自己的日志配置。
	logger.ReplaceGlobal(a.logger)
	logger.Info(ctx, "message-push 服务启动中")

	// runCtx 绑定两个 Pool：任一路致命时 cancel，促使另一路尽快从 Fetch/Handle 返回。
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpErrCh := make(chan error, 1)
	go func() {
		httpErrCh <- a.httpServer.Start()
	}()

	logger.Info(ctx, "message-push 指标 HTTP 服务已启动",
		logger.String("addr", a.httpServer.Addr()),
	)

	msgConsumerErrCh := make(chan error, 1)
	go func() {
		msgConsumerErrCh <- a.msgConsumer.Start(runCtx)
	}()

	realtimeConsumerErrCh := make(chan error, 1)
	go func() {
		realtimeConsumerErrCh <- a.realConsumer.Start(runCtx)
	}()

	select {
	case err := <-httpErrCh:
		// HTTP 异常时同样取消消费，避免指标口挂了仍长时间推送却不可观测。
		cancel()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("运行 message-push http server 失败: %w", err)
		}
		return nil
	case err := <-msgConsumerErrCh:
		// 立刻取消 realtime Pool，缩短半残窗口；defer cancel 作为兜底。
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("运行 msg.push consumer 失败: %w", err)
		}
		return nil
	case err := <-realtimeConsumerErrCh:
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("运行 realtime.push consumer 失败: %w", err)
		}
		return nil
	case <-ctx.Done():
		// 信号关停：cancel 已由 defer 处理；消费退出错误在 Shutdown 前可忽略。
		return nil
	}
}

// Shutdown 按「先停消费 Pool，再停 HTTP/下游连接」关闭资源。
// 调用方应在取消 Run 的 ctx 之后再调用，避免 Close 与活跃 Fetch 无序竞态刷屏。
func (a *MessagePushApp) Shutdown(ctx context.Context) error {
	var errs []error
	// 先关 Kafka Reader，停止继续拉消息与调 connect。
	if a.msgConsumer != nil {
		if err := a.msgConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 msg.push consumer pool 失败: %w", err))
		}
	}
	if a.realConsumer != nil {
		if err := a.realConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 realtime.push consumer pool 失败: %w", err))
		}
	}

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
