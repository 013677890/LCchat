package redisretry

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
)

var (
	globalProducer *kafka.Producer
	producerMu     sync.RWMutex
)

// SetGlobalProducer 设置全局 Kafka Producer 实例。
func SetGlobalProducer(producer *kafka.Producer) {
	producerMu.Lock()
	defer producerMu.Unlock()
	globalProducer = producer
}

// GetGlobalProducer 获取全局 Kafka Producer 实例。
func GetGlobalProducer() *kafka.Producer {
	producerMu.RLock()
	defer producerMu.RUnlock()
	return globalProducer
}

// SendRedisTask 使用全局 Producer 发送 Redis 重试任务。
func SendRedisTask(ctx context.Context, task RedisTask) error {
	producer := GetGlobalProducer()
	if producer == nil {
		return nil
	}

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return producer.Send(ctx, data)
}
