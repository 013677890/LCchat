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

func main() {
	app, err := initializeMsgApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 MsgApp 失败: %v\n", err)
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
			logger.Error(context.Background(), "Msg 服务运行失败", logger.ErrorField("error", err))
		}
	case <-ctx.Done():
		logger.Warn(context.Background(), "收到退出信号，开始关闭 Msg 服务", logger.Any("err", ctx.Err()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(context.Background(), "关闭 Msg 服务失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
}
