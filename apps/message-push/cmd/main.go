package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// main 完成 message-push 进程的启动、阻塞等待与优雅关闭。
func main() {
	app, err := initializeMessagePushApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 MessagePushApp 失败: %v\n", err)
		os.Exit(1)
	}

	// 进程上下文与系统信号绑定。
	// 收到 SIGINT / SIGTERM 后，ctx 会被取消，消费循环可据此退出。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 单独起 goroutine 跑消费主循环。
	// 主 goroutine 负责等待“运行异常”或“退出信号”两种结束条件。
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- app.Run(ctx)
	}()

	select {
	case err := <-runErrCh:
		if err != nil {
			logger.Error(context.Background(), "message-push 服务运行失败", logger.ErrorField("error", err))
		}
	case <-ctx.Done():
		logger.Warn(context.Background(), "收到退出信号，开始关闭 message-push 服务", logger.Any("err", ctx.Err()))
	}

	// 关闭阶段使用独立超时上下文，避免资源回收无限阻塞。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(context.Background(), "关闭 message-push 服务失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
}
