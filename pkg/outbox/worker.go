package outbox

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Handler 是事件处理回调函数。
// 返回 error 表示处理失败，worker 会进行重试。
type Handler func(ctx context.Context, event *Event) error

// WorkerConfig 控制 Outbox worker 的行为。
type WorkerConfig struct {
	// PollInterval 轮询间隔，默认 1s。
	PollInterval time.Duration

	// BatchSize 单次轮询取出的最大事件数，默认 50。
	BatchSize int

	// MaxRetries 最大重试次数，默认 10。超过后标记为 Failed（DLQ）。
	MaxRetries int

	// BaseRetryDelay 重试基础延迟（指数退避的底数），默认 1s。
	BaseRetryDelay time.Duration

	// Logger 可选日志函数。为 nil 时使用 log.Printf。
	Logger func(format string, args ...interface{})
}

func (c *WorkerConfig) defaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 50
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 10
	}
	if c.BaseRetryDelay <= 0 {
		c.BaseRetryDelay = time.Second
	}
	if c.Logger == nil {
		c.Logger = log.Printf
	}
}

// Worker 是 Outbox 事件轮询处理器。
// 它定期从 outbox_events 表中取出未处理的事件，调用注册的 Handler 处理。
//
// 使用示例:
//
//	w := outbox.NewWorker(db, outbox.WorkerConfig{PollInterval: time.Second})
//	w.Register("user_created", func(ctx context.Context, e *outbox.Event) error {
//	    // 调用 user-service.CreateProfile
//	    return nil
//	})
//	go w.Start(ctx) // 在后台运行
//	// shutdown 时 cancel ctx 即可优雅退出
type Worker struct {
	db       *gorm.DB
	cfg      WorkerConfig
	handlers map[string]Handler
	mu       sync.RWMutex
	started  bool
}

// NewWorker 创建一个 Outbox worker。
func NewWorker(db *gorm.DB, cfg WorkerConfig) *Worker {
	cfg.defaults()
	return &Worker{
		db:       db,
		cfg:      cfg,
		handlers: make(map[string]Handler),
	}
}

// Register 注册指定事件类型的处理回调。
// 必须在 Start 之前调用。
func (w *Worker) Register(eventType string, h Handler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[eventType] = h
}

// Start 启动轮询循环。阻塞运行，直到 ctx 被取消。
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	w.started = true
	w.mu.Unlock()

	w.cfg.Logger("outbox worker started, poll_interval=%s, batch_size=%d", w.cfg.PollInterval, w.cfg.BatchSize)

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.cfg.Logger("outbox worker stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// poll 执行一次轮询：取出待处理事件并逐个处理。
func (w *Worker) poll(ctx context.Context) {
	now := time.Now()

	var events []Event
	err := w.db.WithContext(ctx).
		Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", EventStatusPending, now).
		Order("created_at ASC").
		Limit(w.cfg.BatchSize).
		Find(&events).Error

	if err != nil {
		w.cfg.Logger("outbox poll error: %v", err)
		return
	}

	for i := range events {
		if ctx.Err() != nil {
			return
		}
		w.processEvent(ctx, &events[i])
	}
}

// processEvent 处理单个事件。
func (w *Worker) processEvent(ctx context.Context, event *Event) {
	w.mu.RLock()
	handler, ok := w.handlers[event.EventType]
	w.mu.RUnlock()

	if !ok {
		w.cfg.Logger("outbox: no handler registered for event_type=%s, id=%d", event.EventType, event.ID)
		return
	}

	// 标记为处理中
	w.db.Model(event).Update("status", EventStatusProcessing)

	// 执行处理回调
	err := handler(ctx, event)

	if err == nil {
		// 成功
		now := time.Now()
		w.db.Model(event).Updates(map[string]interface{}{
			"status":       EventStatusDone,
			"processed_at": now,
		})
		return
	}

	// 失败：更新重试计数和下次重试时间
	event.RetryCount++
	maxRetries := w.cfg.MaxRetries
	if event.MaxRetries > 0 {
		maxRetries = event.MaxRetries
	}

	if event.RetryCount >= maxRetries {
		// 超过最大重试次数，进入 DLQ
		w.db.Model(event).Updates(map[string]interface{}{
			"status":      EventStatusFailed,
			"retry_count": event.RetryCount,
			"last_error":  truncateError(err, 1000),
		})
		w.cfg.Logger("outbox: event %d (type=%s, entity=%s) moved to DLQ after %d retries: %v",
			event.ID, event.EventType, event.EntityID, event.RetryCount, err)
		return
	}

	// 指数退避
	delay := w.cfg.BaseRetryDelay * time.Duration(math.Pow(2, float64(event.RetryCount-1)))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute // 上限 5 分钟
	}
	nextRetry := time.Now().Add(delay)

	w.db.Model(event).Updates(map[string]interface{}{
		"status":        EventStatusPending, // 回到 pending，等待下次轮询
		"retry_count":   event.RetryCount,
		"next_retry_at": nextRetry,
		"last_error":    truncateError(err, 1000),
	})
}

// InsertEvent 在指定事务内写入一条 Outbox 事件。
// 调用方应在业务事务内调用此函数，保证事件与业务数据的原子性。
//
// 使用示例:
//
//	db.Transaction(func(tx *gorm.DB) error {
//	    // 1. 业务操作
//	    tx.Create(&userAccount)
//	    // 2. 写入 Outbox 事件
//	    return outbox.InsertEvent(tx, "user_created", userUUID, payload)
//	})
func InsertEvent(tx *gorm.DB, eventType, entityID, payload string) error {
	event := &Event{
		EventType: eventType,
		EntityID:  entityID,
		Payload:   payload,
		Status:    EventStatusPending,
		CreatedAt: time.Now(),
	}
	return tx.Create(event).Error
}

// CheckIdempotent 检查事件是否已处理过（幂等）。
// 返回 true 表示已处理，调用方应跳过。
func CheckIdempotent(db *gorm.DB, eventType, entityID string) (bool, error) {
	var count int64
	err := db.Model(&IdempotentRecord{}).
		Where("event_type = ? AND entity_id = ?", eventType, entityID).
		Count(&count).Error
	return count > 0, err
}

// MarkIdempotent 标记事件已处理。
func MarkIdempotent(db *gorm.DB, eventType, entityID string) error {
	record := &IdempotentRecord{
		EventType:   eventType,
		EntityID:    entityID,
		ProcessedAt: time.Now(),
	}
	return db.Create(record).Error
}

func truncateError(err error, maxLen int) string {
	s := fmt.Sprintf("%v", err)
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
