package mq

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
)

// SetGlobalProducer 设置全局 Kafka Producer 实例。
func SetGlobalProducer(producer *kafka.Producer) {
	redisretry.SetGlobalProducer(producer)
}

// GetGlobalProducer 获取全局 Kafka Producer 实例。
func GetGlobalProducer() *kafka.Producer {
	return redisretry.GetGlobalProducer()
}

// SendRedisTask 使用全局 Producer 发送 Redis 重试任务。
func SendRedisTask(ctx context.Context, task RedisTask) error {
	return redisretry.SendRedisTask(ctx, task)
}
