package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// isolatedPoolRestartBackoff 是旁路 Pool 致命退出后的固定重启间隔。
// 使用固定值而不是指数退避：确定性毒配置/已关闭 Reader 场景下也能限制日志刷屏，
// 同时让运维在指标上稳定观察到「隔离重启中」而不是尖刺后消失。
const isolatedPoolRestartBackoff = 5 * time.Second

// isolatedPoolFailures 统计 API 服务中被隔离的 Kafka 消费 Pool 致命退出次数。
// 使用全局 promauto 注册，便于多服务统一指标名；各服务再通过 IsolatedPoolFailureCollector
// 挂到自己的独立 Registry，保证 /metrics 能刮到该信号。
var isolatedPoolFailures = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lcchat",
	Subsystem: "kafka",
	Name:      "isolated_pool_failures_total",
	Help:      "API 服务中被隔离的 Kafka 消费 Pool 致命退出次数。",
}, []string{"pool_name"})

// IsolatedPoolFailureCollector 返回 API 服务旁路 Pool 故障计数器。
// 各服务应在 Wire/启动阶段 RegisterCollector 到本服务 metrics Registry；
// 注册失败必须 fail-fast，避免组件已运行但关键故障指标不可见。
func IsolatedPoolFailureCollector() prometheus.Collector {
	return isolatedPoolFailures
}

// RunIsolatedPool 在 API 服务中监督一个旁路消费 Pool，直到 ctx 取消。
//
// 适用场景：
//   - auth/user/relation/group/msg 等「主业是 gRPC、Kafka 是旁路」的服务；
//   - 需要在 Kafka 消费组件致命失败时继续对外提供 API，避免与 Kafka 雪崩绑定。
//
// 不适用场景：
//   - message-push：消费即主业，Pool 致命失败应让进程非零退出，由编排系统重建。
//
// 行为约定：
//  1. 调用 start(ctx) 阻塞运行一整轮 Pool.Start；
//  2. ctx 取消或 error 为 context.Canceled：正常关停，直接返回；
//  3. 其它错误或「ctx 仍有效却 nil 返回」：记指标 + Error 日志，固定退避后再次 start；
//  4. 错误不会返回给 gRPC 主循环，从而实现 isolate。
//
// 与 ManualConsumerPool 的关系：
//   - Pool 内部仍会在 worker 致命时 cancel 兄弟并整体失败（禁止半残并发）；
//   - 本函数只决定「Pool 失败之后进程是否继续服务」。
//
// 重启前提（重要）：
//   start 通常绑定同一 *ManualConsumerPool 实例。重入 Start 要求底层 Reader 仍可使用。
//   若上一轮因 Close 导致 Reader 已关闭，后续每轮都会快速致命并刷指标，直到进程重启
//   或调用方改为「失败后重建 Pool」。进程正常关停路径应先 cancel ctx，再 Close Pool，
//   让监督循环因 Canceled 退出，而不是先 Close 再指望重入自愈。
func RunIsolatedPool(ctx context.Context, poolName string, start func(context.Context) error) {
	for {
		err := start(ctx)
		// 关停优先：父 ctx 取消时无论 start 返回什么，都结束监督，避免关停后仍重启。
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}
		if err == nil {
			// Pool.Start 在 ctx 仍有效时不应“干净返回”；视为实现缺陷并按致命处理。
			err = errors.New("消费 Pool 在上下文有效时意外结束")
		}

		isolatedPoolFailures.WithLabelValues(poolName).Inc()
		logger.Error(ctx, "旁路 Kafka 消费 Pool 致命退出，将隔离故障并退避重启",
			logger.String("pool_name", poolName),
			logger.Any("restart_backoff", isolatedPoolRestartBackoff.String()),
			logger.ErrorField("error", err),
		)

		// 固定退避避免紧密重启刷屏；ctx 取消时立即停止监督，不把退避算进关停阻塞。
		timer := time.NewTimer(isolatedPoolRestartBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
