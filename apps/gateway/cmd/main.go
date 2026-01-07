package main

import (
	"ChatServer/apps/gateway/internal/router"
	"ChatServer/config"
	"ChatServer/pkg/logger"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx := context.Background()

	// 1. 初始化日志
	cfg := config.DefaultLoggerConfig()
	l, err := logger.Build(cfg)
	if err != nil {
		fmt.Printf("Failed to build logger: %v\n", err)
		os.Exit(1)
	}
	logger.ReplaceGlobal(l)
	defer func() {
		// 同步日志缓冲区
		if err := logger.L().Sync(); err != nil {
			// Sync 在某些情况下会返回错误（如 os.Stdout），可以忽略
			_ = err
		}
	}()

	logger.Info(ctx, "Gateway service initializing...")

	// 2. 初始化路由
	r := router.InitRouter()

	// 3. 配置服务器
	port := 8080 // TODO: 从配置文件读取
	addr := fmt.Sprintf(":%d", port)

	srv := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    10 * time.Second, // 读取超时
		WriteTimeout:   10 * time.Second, // 写入超时
		MaxHeaderBytes: 1 << 20,          // 最大请求头 1MB
	}

	// 4. 启动服务器（在 goroutine 中）
	go func() {
		logger.Info(ctx, "Gateway server starting",
			logger.String("address", addr),
			logger.Int("port", port),
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "Server start failed", logger.ErrorField("error", err))
			os.Exit(1)
		}
	}()

	logger.Info(ctx, "Gateway server started successfully, press Ctrl+C to shutdown")

	// 5. 优雅停机
	quit := make(chan os.Signal, 1)
	// 监听中断信号：Ctrl+C (SIGINT) 和 kill 命令 (SIGTERM)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞等待信号
	sig := <-quit
	logger.Info(ctx, "Received shutdown signal, starting graceful shutdown...",
		logger.String("signal", sig.String()),
	)

	// 6. 设置超时时间，等待正在处理的请求完成
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭 HTTP 服务器
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "Server forced to shutdown", logger.ErrorField("error", err))
		os.Exit(1)
	}

	logger.Info(ctx, "Gateway server exited gracefully")
}
