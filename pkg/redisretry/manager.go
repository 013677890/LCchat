package redisretry

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
)

var errProducerNotConfigured = errors.New("Redis DEL 补偿 producer 未初始化")

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

func getGlobalProducer() *kafka.Producer {
	producerMu.RLock()
	defer producerMu.RUnlock()
	return globalProducer
}

func sendRedisTask(ctx context.Context, task RedisTask) error {
	if err := task.Validate(); err != nil {
		return err
	}

	producer := getGlobalProducer()
	if producer == nil {
		return errProducerNotConfigured
	}

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return producer.SendWithKey(ctx, []byte(task.Keys[0]), data)
}
