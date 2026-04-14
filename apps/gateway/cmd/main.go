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
// 1. 调用 Wire injector 构造 GatewayApp；
// 2. 监听系统退出信号；
// 3. 编排 Run / Shutdown 两个生命周期入口。
//
// 这样主函数本身不再承担对象拼装职责，后续新增依赖时不会继续膨胀成“大型启动脚本”。
func main() {
	app, err := initializeGatewayApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 GatewayApp 失败: %v\n", err)
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
			logger.Error(context.Background(), "Gateway 服务运行失败", logger.ErrorField("error", err))
		}
	case <-ctx.Done():
		logger.Warn(context.Background(), "收到退出信号，开始关闭 Gateway 服务", logger.Any("err", ctx.Err()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(context.Background(), "关闭 Gateway 服务失败", logger.ErrorField("error", err))
		os.Exit(1)
	}
}
