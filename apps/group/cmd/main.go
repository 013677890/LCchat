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

// main 只保留进程级职责：
//  1. 通过 Wire 组装 GroupApp；
//  2. 监听系统退出信号；
//  3. 统一协调 Run / Shutdown 生命周期。
//
// 这样可以把“依赖装配”“服务启动”“优雅停机”三个关注点明确拆开，
// 避免在 main 中堆积具体业务依赖，后续扩展消费者、内部 gRPC 客户端时也更好维护。
func main() {
	app, err := initializeGroupApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 GroupApp 失败: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- app.Run(ctx)
	}()

	select {
	case err := <-runErrCh:
		if err != nil {
			logger.Error(context.Background(), "Group 服务运行失败", logger.ErrorField("error", err))
		}
	case <-ctx.Done():
		logger.Warn(context.Background(), "收到退出信号，开始关闭 Group 服务", logger.Any("err", ctx.Err()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(context.Background(), "关闭 Group 服务失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
}
