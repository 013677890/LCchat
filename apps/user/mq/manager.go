package mq

import (
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"context"
	"encoding/json"
	"sync"
)

// ==================== 全局 Redis 重试管理器 ====================

var (
	globalProducer *kafka.Producer
	producerMu     sync.RWMutex
)

// SetGlobalProducer 设置全局 Kafka Producer 实例
// 应在应用启动时调用一次
func SetGlobalProducer(producer *kafka.Producer) {
	producerMu.Lock()
	defer producerMu.Unlock()
	globalProducer = producer
}

// GetGlobalProducer 获取全局 Kafka Producer 实例
func GetGlobalProducer() *kafka.Producer {
	//producerMu.RLock()
	//defer producerMu.RUnlock()
	return globalProducer
}

// SendRedisTask 使用全局 Producer 发送 Redis 任务
// 如果全局 Producer 未初始化，返回 nil（不报错，避免影响主流程）
func SendRedisTask(ctx context.Context, task RedisTask) error {
	producer := GetGlobalProducer()
	if producer == nil {
		// Producer 未初始化，静默失败
		return nil
	}

	// 序列化任务
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return producer.Send(ctx, data)
}
