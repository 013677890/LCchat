package async

import (
	"context"
	"errors"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/panjf2000/ants/v2"
)

var (
	global   *ants.Pool
	globalMu sync.Mutex
	cfgCopy  config.AsyncConfig
)

// ContextPropagator 由业务层注入，用于从父 ctx 提取需要透传的字段。
var ContextPropagator func(parent context.Context) context.Context

// SetContextPropagator 设置上下文传递器（建议在 main 初始化时调用）。
func SetContextPropagator(fn func(context.Context) context.Context) {
	ContextPropagator = fn
}

// ErrNotInitialized 表示协程池尚未初始化。
var ErrNotInitialized = errors.New("async pool not initialized")

// Pool 返回全局协程池（未初始化时为 nil）。
func Pool() *ants.Pool { return global }

// ReplaceGlobal 设置全局协程池。
func ReplaceGlobal(p *ants.Pool) { global = p }

// Build 根据配置创建协程池实例。
func Build(cfg config.AsyncConfig) (*ants.Pool, error) {
	opts := []ants.Option{
		ants.WithMaxBlockingTasks(cfg.MaxBlockingTasks),
		ants.WithExpiryDuration(cfg.ExpiryDuration),
		ants.WithPanicHandler(func(p any) {
			msg := "async task panic"
			if logger.L() != nil {
				logger.Error(context.Background(), msg,
					logger.Any("panic", p),
					logger.String("stack", string(debug.Stack())),
				)
				return
			}
		}),
	}
	if cfg.Nonblocking {
		opts = append(opts, ants.WithNonblocking(true))
	}

	return ants.NewPool(cfg.PoolSize, opts...)
}

// Init 初始化全局协程池（仅需在进程启动时调用一次）。
func Init(cfg config.AsyncConfig) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if global != nil {
		return nil
	}

	p, err := Build(cfg)
	if err != nil {
		return err
	}

	global = p
	cfgCopy = cfg
	return nil
}

// Submit 将任务投递到全局协程池。
func Submit(task func()) error {
	if global == nil {
		return ErrNotInitialized
	}
	return global.Submit(task)
}

// Release 优雅释放协程池资源（等待任务执行完）。
func Release() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if global == nil {
		return nil
	}

	var err error
	if cfgCopy.ReleaseTimeout > 0 {
		err = global.ReleaseTimeout(cfgCopy.ReleaseTimeout)
	} else {
		global.Release()
	}
	global = nil
	return err
}

// RunSafe 安全的异步任务
func RunSafe(ctx context.Context, task func(ctx context.Context), timeout time.Duration) {
	if task == nil {
		return
	}

	if timeout <= 0 {
		timeout = time.Minute
	}

	baseCtx := context.Background()
	if ContextPropagator != nil && ctx != nil {
		baseCtx = ContextPropagator(ctx)
	}

	runCtx, cancel := context.WithTimeout(baseCtx, timeout)

	wrap := func() {
		defer cancel()
		timer := time.AfterFunc(timeout, func() {
			if runCtx.Err() == context.DeadlineExceeded {
				if logger.L() != nil {
					logger.Warn(runCtx, "async task timeout",
						logger.Duration("timeout", timeout),
					)
					return
				}
				log.Printf("async task timeout: %s", timeout)
			}
		})
		defer timer.Stop()
		defer func() {
			if r := recover(); r != nil {
				msg := "async task panic"
				if logger.L() != nil {
					logger.Error(runCtx, msg,
						logger.Any("panic", r),
						logger.String("stack", string(debug.Stack())),
					)
					return
				}
			}
		}()

		task(runCtx)
	}

	if err := Submit(wrap); err != nil {
		cancel()
		if logger.L() != nil {
			logger.Error(baseCtx, "async submit failed",
				logger.ErrorField("error", err),
				logger.Duration("timeout", timeout),
			)
			return
		}
	}
}
