package async

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Group 表示一组需要随请求收敛的并发任务。
//
// 与 RunSafe 的后台异步语义相反：
//   - 直接继承父 ctx 的取消和 deadline，请求结束则同组停止；
//   - 不占用后台 ants 池，避免缓存/补偿任务挤占请求关键路径；
//   - 任一任务返回错误或 panic，都会 cancel 同组，Wait 返回第一个错误。
//
// 典型用法：gateway 聚合多个只读 gRPC、msg 会话 fan-out。
// 不要用 Group 做 fire-and-forget 补偿，那是 RunSafe 的职责。
type Group struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	errOnce sync.Once
	err     error
}

// NewGroup 创建一个请求内并发组。
// timeout > 0 时在父 ctx 上再加一层超时，用于限制整个 fan-out 墙钟时间；
// timeout <= 0 时只 WithCancel，仍保留独立 cancel，以便某个任务失败时取消兄弟任务。
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

// Context 返回并发组的派生 ctx，可供调用方在 Go 之外与同组共享取消信号。
func (g *Group) Context() context.Context {
	if g == nil || g.ctx == nil {
		return context.Background()
	}
	return g.ctx
}

// Go 启动请求内轻量并发任务。
// 组已取消时立即失败并计入 Wait 的错误，避免在已超时请求上继续膨胀 goroutine。
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
		// 组已取消则拒绝新任务：不再 go，并把取消原因回给本次 Go 的调用方。
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
// 任务错误优先于 ctx 超时/取消：调用方需要知道是业务失败还是墙钟到期。
// 返回前会 cancel，释放 timeout timer，并让仍阻塞在 ctx 上的任务尽快退出。
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

// setError 只保留第一个错误并立即 cancel，让其余任务通过 ctx.Done 尽快退出。
func (g *Group) setError(err error) {
	if err == nil {
		return
	}
	g.errOnce.Do(func() {
		g.err = err
		g.cancel()
	})
}
