package async

import (
	"context"
	"errors"
	"fmt"
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
	globalMu sync.RWMutex
	cfgCopy  config.AsyncConfig

	contextPropagator   func(parent context.Context) context.Context
	contextPropagatorMu sync.RWMutex
)

// SetContextPropagator 设置上下文传递器（建议在 main 初始化时调用）。
//
// fn 用于从父 ctx 提取需要透传的字段。后台异步任务必须与父请求取消树物理隔离，
// 因此传入 fn 的 parent 已通过 context.WithoutCancel 切断父级取消和 deadline。
func SetContextPropagator(fn func(context.Context) context.Context) {
	contextPropagatorMu.Lock()
	defer contextPropagatorMu.Unlock()

	if fn == nil {
		contextPropagator = nil
		return
	}
	contextPropagator = func(parent context.Context) context.Context {
		if parent == nil {
			parent = context.Background()
		}
		return fn(context.WithoutCancel(parent))
	}
}

// ErrNotInitialized 表示协程池尚未初始化。
var ErrNotInitialized = errors.New("async pool not initialized")

// ErrNilTask 表示提交了空任务。
var ErrNilTask = errors.New("async task is nil")

// ErrTaskPanic 表示协程池任务发生 panic。
var ErrTaskPanic = errors.New("async task panic")

// ErrInvalidPoolSize 表示协程池容量配置非法。
var ErrInvalidPoolSize = errors.New("async pool size must be positive")

// Pool 返回全局协程池（未初始化时为 nil）。
func Pool() *ants.Pool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

// ReplaceGlobal 设置全局协程池。
// 未传入 cfg 时使用默认配置，确保 Release 仍按默认超时等待运行中任务退出。
func ReplaceGlobal(p *ants.Pool, cfgs ...config.AsyncConfig) {
	cfg := config.DefaultAsyncConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	global = p
	if p == nil {
		cfgCopy = config.AsyncConfig{}
		return
	}
	cfgCopy = cfg
}

// ReplaceGlobalWithReleaseTimeout 设置全局协程池并指定释放等待时间。
func ReplaceGlobalWithReleaseTimeout(p *ants.Pool, timeout time.Duration) {
	cfg := config.DefaultAsyncConfig()
	cfg.ReleaseTimeout = timeout
	ReplaceGlobal(p, cfg)
}

// Build 根据配置创建协程池实例。
func Build(cfg config.AsyncConfig) (*ants.Pool, error) {
	if cfg.PoolSize <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidPoolSize, cfg.PoolSize)
	}

	opts := []ants.Option{
		// ants 阻塞入队无法感知请求 ctx.Done，因此协程池强制非阻塞提交。
		ants.WithNonblocking(true),
		ants.WithExpiryDuration(cfg.ExpiryDuration),
		ants.WithPanicHandler(func(p any) {
			logPanic(context.Background(), "async task panic", p)
		}),
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
	if task == nil {
		return ErrNilTask
	}

	p := Pool()
	if p == nil {
		return ErrNotInitialized
	}
	return p.Submit(task)
}

// Release 优雅释放协程池资源（等待任务执行完）。
func Release() error {
	globalMu.Lock()
	p := global
	cfg := cfgCopy
	global = nil
	cfgCopy = config.AsyncConfig{}
	globalMu.Unlock()

	if p == nil {
		return nil
	}

	return releasePool(p, cfg.ReleaseTimeout)
}

func releasePool(p *ants.Pool, timeout time.Duration) error {
	if p == nil {
		return nil
	}
	if timeout > 0 {
		if err := p.ReleaseTimeout(timeout); err != nil && !errors.Is(err, ants.ErrPoolClosed) {
			return err
		}
		return nil
	}
	p.Release()
	return nil
}

// ReleasePool 释放指定协程池实例，适用于尚未挂载为全局池的启动失败兜底路径。
func ReleasePool(p *ants.Pool, timeout time.Duration) error {
	return releasePool(p, timeout)
}

// Group 表示一组需要随请求收敛的协程池任务。
//
// 与 RunSafe 的后台异步语义不同，Group 直接继承父 ctx 的取消和 deadline，
// 适用于请求内 fan-out 并发；任务返回错误或任务 panic 都会取消同组任务，
// 并由 Wait 返回第一个错误。
type Group struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	errOnce sync.Once
	err     error
}

// NewGroup 创建一个请求内并发组。
// timeout > 0 时会在父 ctx 基础上增加一层超时；timeout <= 0 时仅继承父 ctx。
func NewGroup(ctx context.Context, timeout time.Duration) *Group {
	if ctx == nil {
		ctx = context.Background()
	}

	var groupCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		groupCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		groupCtx, cancel = context.WithCancel(ctx)
	}

	return &Group{ctx: groupCtx, cancel: cancel}
}

// Context 返回并发组的派生 ctx。
func (g *Group) Context() context.Context {
	if g == nil || g.ctx == nil {
		return context.Background()
	}
	return g.ctx
}

// Go 启动请求内轻量并发任务。
// Group 不复用后台异步协程池，避免后台缓存/补偿任务挤占请求关键路径。
func (g *Group) Go(task func(ctx context.Context) error) error {
	if g == nil {
		return ErrNotInitialized
	}
	if task == nil {
		g.setError(ErrNilTask)
		return ErrNilTask
	}

	select {
	case <-g.ctx.Done():
		err := g.ctx.Err()
		g.setError(err)
		return err
	default:
	}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("%w: %v", ErrTaskPanic, r)
				logPanic(g.ctx, "async group task panic", r)
				g.setError(err)
			}
		}()

		if err := task(g.ctx); err != nil {
			g.setError(err)
		}
	}()

	return nil
}

// Wait 等待所有已成功提交的任务结束，并返回第一个错误。
func (g *Group) Wait() error {
	if g == nil {
		return nil
	}

	g.wg.Wait()
	err := g.err
	ctxErr := g.ctx.Err()
	g.cancel()

	if err != nil {
		return err
	}
	return ctxErr
}

func (g *Group) setError(err error) {
	if err == nil {
		return
	}
	g.errOnce.Do(func() {
		g.err = err
		g.cancel()
	})
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
	if propagator := getContextPropagator(); propagator != nil && ctx != nil {
		if propagated := propagator(ctx); propagated != nil {
			baseCtx = context.WithoutCancel(propagated)
		}
	}

	runCtx, cancel := context.WithTimeout(baseCtx, timeout)

	wrap := func() {
		var timeoutOnce sync.Once
		logDeadline := func() {
			timeoutOnce.Do(func() {
				logTimeout(runCtx, timeout)
			})
		}
		timer := time.AfterFunc(timeout, logDeadline)
		defer timer.Stop()
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				logPanic(runCtx, "async task panic", r)
			}
		}()

		task(runCtx)
		if runCtx.Err() == context.DeadlineExceeded {
			logDeadline()
		}
	}

	if err := Submit(wrap); err != nil {
		cancel()
		logSubmitFailed(baseCtx, err, timeout)
	}
}

func getContextPropagator() func(context.Context) context.Context {
	contextPropagatorMu.RLock()
	defer contextPropagatorMu.RUnlock()
	return contextPropagator
}

func logPanic(ctx context.Context, msg string, p any) {
	stack := string(debug.Stack())
	if logger.L() != nil {
		logger.Error(ctx, msg,
			logger.Any("panic", p),
			logger.String("stack", stack),
		)
		return
	}
	log.Printf("%s: %v\n%s", msg, p, stack)
}

func logTimeout(ctx context.Context, timeout time.Duration) {
	if logger.L() != nil {
		logger.Warn(ctx, "async task timeout",
			logger.Duration("timeout", timeout),
		)
		return
	}
	log.Printf("async task timeout: %s", timeout)
}

func logSubmitFailed(ctx context.Context, err error, timeout time.Duration) {
	if logger.L() != nil {
		logger.Error(ctx, "async submit failed",
			logger.ErrorField("error", err),
			logger.Duration("timeout", timeout),
		)
		return
	}
	log.Printf("async submit failed: %v, timeout=%s", err, timeout)
}
