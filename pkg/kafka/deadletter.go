package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DeadLetterRecord 描述一条被旁路到死信的消息及其上下文。
// 通用消费者只能填写 Kafka 元信息与原始字节；EventType 等 payload 语义字段可由调用方按需补充。
type DeadLetterRecord struct {
	Topic         string
	Partition     int
	Offset        int64
	Key           []byte
	EventType     string // 可选：通用消费者通常留空，由具备 payload 语义的调用方填写
	Payload       []byte
	Err           string
	Attempts      int
	FirstFailedAt time.Time
	LastFailedAt  time.Time
}

// DeadLetterSink 把超过重试预算的「毒消息」持久化到死信存储，用于解除队头阻塞。
//
// 约定：Park 返回非 nil 表示落地失败——此时消费者绝不提交 offset，回退到继续阻塞重试，
// 以此保证「不丢消息」优先于「分区前进」。仅当 Park 成功后才提交 offset 旁路该消息。
type DeadLetterSink interface {
	Park(ctx context.Context, rec DeadLetterRecord) error
}

// PermanentError 标记“再次执行 handler 也不可能成功”的确定性消息错误。
//
// 典型场景是 JSON/Proto 解码失败、事件 schema_version 不受支持、必填业务字段缺失。
// 通用消费者看到该类型后不会消耗两分钟重试预算，而是在第一次失败时立即写死信；
// 只有死信落地成功才提交 offset。基础设施错误绝不能包装成该类型。
type PermanentError struct {
	err error
}

func (e *PermanentError) Error() string {
	if e == nil || e.err == nil {
		return "permanent kafka message error"
	}
	return e.err.Error()
}

func (e *PermanentError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// Permanent 把确定性 handler 错误标记为立即死信。
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	if IsPermanent(err) {
		return err
	}
	return &PermanentError{err: fmt.Errorf("permanent kafka message error: %w", err)}
}

// IsPermanent 判断错误链中是否包含 PermanentError。
func IsPermanent(err error) bool {
	var target *PermanentError
	return errors.As(err, &target)
}
