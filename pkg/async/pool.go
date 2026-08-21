// Package async 提供两类互不混用的并发原语。
//
//  1. 后台协程池（本文件）：基于 ants 的进程级全局池，供 RunSafe / TryRunSafe
//     做 fire-and-forget 缓存补偿、异步写 Redis 等。任务与父请求取消树物理隔离，
//     超时独立计时；提交失败只表示没进池，不代表任务执行结果。
//  2. 请求内 Group（group.go）：直接 go，继承父 ctx。用于请求路径上的 fan-out，
//     任一任务失败即取消同组。不占用后台池，避免补偿任务挤占关键路径。
package async

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/panjf2000/ants/v2"
)

// ======================== 异步任务超时常量 ========================
//
// 调用 RunSafe / TryRunSafe 时应使用以下常量而非传 0。
// timeout <= 0 会回退到 DefaultAsyncTimeout，但显式常量更易于 code review
// 判断任务类别（单次 Redis / Pipeline / DB / 含重试补偿）。
const (
	// AsyncRedisTimeout 单次 Redis 操作（SET / DEL / Lua 脚本等）。
	AsyncRedisTimeout = 5 * time.Second

	// AsyncRedisPipelineTimeout Redis Pipeline 批量操作。
	AsyncRedisPipelineTimeout = 10 * time.Second

	// AsyncDBTimeout 单次数据库查询或写入。
	AsyncDBTimeout = 10 * time.Second

	// AsyncRetryTimeout 含重试逻辑的补偿任务。
	AsyncRetryTimeout = 30 * time.Second

	// DefaultAsyncTimeout RunSafe timeout <= 0 时的兜底值。
	DefaultAsyncTimeout = 10 * time.Second
)

var (
	// global 是进程级 ants 池。业务代码应走 RunSafe / Submit，不要直接操作该指针。
	global   *ants.Pool
	globalMu sync.RWMutex
	// cfgCopy 与 global 成对更新，供 Release 在摘掉全局指针后仍按原超时等待在途任务。
	cfgCopy config.AsyncConfig

	contextPropagator   func(parent context.Context) context.Context
	contextPropagatorMu sync.RWMutex

	// submitRejected 记录因池满或未初始化而被丢弃的任务计数，便于监控和告警。
	submitRejected atomic.Int64
)

// SetContextPropagator 设置上下文传递器（建议在 main 初始化时调用一次）。
//
// fn 只负责从父 ctx 提取需要透传的字段（trace_id、user_uuid 等）。
// 后台任务必须与父请求取消树物理隔离：父请求结束不能把补偿任务一起 Cancel。
// 因此包装层会先 WithoutCancel，再交给 fn；调用方不必自己切断取消链。
// 传入 nil 表示清空传递器，后续任务退回 context.Background()。
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

// ErrNotInitialized 表示协程池尚未初始化，或对 nil Group 调用了 Go。
var ErrNotInitialized = errors.New("async pool not initialized")

// ErrNilTask 表示提交了空任务。
var ErrNilTask = errors.New("async task is nil")

// ErrTaskPanic 表示任务发生 panic。Group.Wait 会把它作为第一个错误返回；
// 后台池任务的 panic 只记日志，不回传给提交方。
var ErrTaskPanic = errors.New("async task panic")

// ErrInvalidPoolSize 表示协程池容量配置非法。
var ErrInvalidPoolSize = errors.New("async pool size must be positive")

// Pool 返回全局协程池（未初始化时为 nil）。
// 生命周期比较（如 app.Stop 判断当前全局池是否仍是本实例）应使用本函数，不要缓存指针后跨阶段比较。
func Pool() *ants.Pool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

// ReplaceGlobal 设置全局协程池，供各服务 cmd 在 Wire 装配后挂载已 Build 的实例。
// 未传入 cfg 时使用默认配置，避免 Release 因零值超时而立刻 p.Release()、不等待在途任务。
// p == nil 用于测试隔离或停机后清空，同时丢掉 cfgCopy。
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

// ReplaceGlobalWithReleaseTimeout 设置全局协程池并覆盖释放等待时间。
// 各服务 Stop 路径需要与环境变量中的 async release timeout 对齐时使用本函数。
func ReplaceGlobalWithReleaseTimeout(p *ants.Pool, timeout time.Duration) {
	cfg := config.DefaultAsyncConfig()
	cfg.ReleaseTimeout = timeout
	ReplaceGlobal(p, cfg)
}

// Build 根据配置创建协程池实例，不挂到全局。
// 启动失败时调用方应 ReleasePool 释放这个尚未挂载的实例，避免泄漏。
func Build(cfg config.AsyncConfig) (*ants.Pool, error) {
	if cfg.PoolSize <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidPoolSize, cfg.PoolSize)
	}

	opts := []ants.Option{
		// ants 默认阻塞入队，且无法感知请求 ctx.Done。池满时阻塞会拖死 HTTP/gRPC
		// 关键路径，因此强制非阻塞：满则立即返回错误，由 TryRunSafe 记 rejected 并上抛。
		ants.WithNonblocking(true),
		ants.WithExpiryDuration(cfg.ExpiryDuration),
		// 兜底 panic：RunSafe 包装层已 recover，这里覆盖未经 Submit 包装的裸任务。
		ants.WithPanicHandler(func(p any) {
			logPanic(context.Background(), "async task panic", p)
		}),
	}
	return ants.NewPool(cfg.PoolSize, opts...)
}

// Init 初始化全局协程池（仅需在进程启动时调用一次）。
// 已初始化时直接返回成功，避免重复 Build 泄漏旧池。
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
// 池满时因 Nonblocking 立即失败，不会在调用方 goroutine 上排队等待。
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

// Release 优雅释放全局协程池。
// 先摘掉全局指针再等待，避免停机等待期间仍有新任务经 Pool()/Submit 进入已关闭的池。
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

// releasePool 按超时释放指定池。timeout <= 0 走 ants.Release（立即关闭，在途任务可能被丢）。
// 已关闭的池再 ReleaseTimeout 会返回 ErrPoolClosed，停机路径视为成功，避免掩盖真正的超时错误。
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

// ReleasePool 释放尚未挂载为全局池的实例，适用于启动失败兜底路径。
func ReleasePool(p *ants.Pool, timeout time.Duration) error {
	return releasePool(p, timeout)
}

// RunSafe 安全提交 fire-and-forget 异步任务。
// 提交失败只打日志并累计 rejected，不返回给调用方。需要感知「没进池」时应改用 TryRunSafe。
func RunSafe(ctx context.Context, task func(ctx context.Context), timeout time.Duration) {
	_ = TryRunSafe(ctx, task, timeout)
}

// TryRunSafe 安全提交异步任务。返回值仅表示是否进入协程池，不代表任务执行结果。
// 需要在提交失败时安排持久化补偿的调用方应使用此方法。
//
// 语义约束：
//   - 与父请求取消树隔离，避免请求结束把补偿任务一起取消；
//   - timeout <= 0 回退 DefaultAsyncTimeout，禁止无限跑；
//   - 任务 panic 只记日志，不击穿进程；
//   - 超时日志可能由定时器或任务返回后两条路径触发，timeoutOnce 保证只打一次。
func TryRunSafe(ctx context.Context, task func(ctx context.Context), timeout time.Duration) error {
	if task == nil {
		return ErrNilTask
	}

	if timeout <= 0 {
		timeout = DefaultAsyncTimeout
	}

	baseCtx := context.Background()
	if propagator := getContextPropagator(); propagator != nil && ctx != nil {
		if propagated := propagator(ctx); propagated != nil {
			baseCtx = propagated
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

		// 任务若忽略 ctx、卡住不返回，AfterFunc 仍能打超时日志；任务正常返回则 Stop 掉定时器。
		timer := time.AfterFunc(timeout, logDeadline)
		defer timer.Stop()
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				logPanic(runCtx, "async task panic", r)
			}
		}()

		task(runCtx)
		// 任务已返回但 ctx 已超时：任务可能忽略了 deadline，补一条日志。
		if runCtx.Err() == context.DeadlineExceeded {
			logDeadline()
		}
	}

	if err := Submit(wrap); err != nil {
		cancel()
		rejected := submitRejected.Add(1)
		logSubmitFailed(baseCtx, err, timeout, rejected)
		return err
	}
	return nil
}

// getContextPropagator 读当前传递器。TryRunSafe 每次提交都取一次，避免持有过期闭包。
func getContextPropagator() func(context.Context) context.Context {
	contextPropagatorMu.RLock()
	defer contextPropagatorMu.RUnlock()
	return contextPropagator
}

// logPanic 记录任务 panic。logger 尚未初始化时（测试或进程极早期）回退到标准库，避免丢栈。
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

// logTimeout 记录后台任务超时。级别用 Warn：超时不一定是故障，但需要可观测。
func logTimeout(ctx context.Context, timeout time.Duration) {
	if logger.L() != nil {
		logger.Warn(ctx, "async task timeout",
			logger.Duration("timeout", timeout),
		)
		return
	}
	log.Printf("async task timeout: %s", timeout)
}

// logSubmitFailed 记录入池失败，并带上累计 rejected，便于和监控对账。
func logSubmitFailed(ctx context.Context, err error, timeout time.Duration, rejectedTotal int64) {
	if logger.L() != nil {
		logger.Error(ctx, "async submit failed",
			logger.ErrorField("error", err),
			logger.Duration("timeout", timeout),
			logger.Int64("rejected_total", rejectedTotal),
		)
		return
	}
	log.Printf("async submit failed: %v, timeout=%s, rejected_total=%d", err, timeout, rejectedTotal)
}
