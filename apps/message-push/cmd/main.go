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
// 与 API 服务不同：消费 Pool 致命失败必须非零退出，交由编排系统告警与拉起新实例。
func main() {
	app, err := initializeMessagePushApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 MessagePushApp 失败: %v\n", err)
		os.Exit(1)
	}

	// 进程上下文与系统信号绑定；SIGINT/SIGTERM 后 ctx 取消，消费循环应尽快退出。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run 放在独立 goroutine：主 goroutine 同时等待「运行致命错误」与「退出信号」。
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- app.Run(ctx)
	}()

	var runErr error
	select {
	case err := <-runErrCh:
		if err != nil {
			runErr = err
			logger.Error(context.Background(), "message-push 服务运行失败", logger.ErrorField("error", err))
		}
	case <-ctx.Done():
		logger.Warn(context.Background(), "收到退出信号，开始关闭 message-push 服务", logger.Any("err", ctx.Err()))
	}

	// 关闭使用独立超时上下文，避免资源回收无限阻塞；先于进程退出尽量释放 Reader/连接。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(context.Background(), "关闭 message-push 服务失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
	if runErr != nil {
		// 主业失败：非零退出。不要在此处吞掉 runErr 假装健康。
		os.Exit(1)
	}
}
