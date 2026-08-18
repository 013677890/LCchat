// Package pusherr 存放 message-push 业务 Handler 与 consumer 适配层共用的错误哨兵。
//
// 拆到独立包是为了让 msgpush / realtime / consumer 都能 errors.Is 同一哨兵，
// 同时避免 consumer 反向依赖业务包，或业务包互相 import。
package pusherr

import "errors"

// ErrRetriable 表示当前消息处理失败属于可重试范畴（Redis 抖动、connect 瞬时不可达等）。
//
// 约定：
//   - 业务 Handler（msgpush / realtime）对瞬时失败用 fmt.Errorf("%w: ...", ErrRetriable) 包装；
//   - consumer 适配层用 errors.Is 识别后做有限次本地退避；耗尽后仍返回 nil 以放行 offset；
//   - 永久性错误（proto 反序列化失败、字段校验失败、不支持的事件类型）不得包装为本错误，
//     应由 Handler 记日志后直接返回 nil，表示「跳过并提交」。
//
// 本哨兵只表达「是否值得本地再试」，不表达 Kafka/进程级致命失败。
var ErrRetriable = errors.New("message-push: retriable handle error")
