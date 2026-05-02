package mq

import (
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
	"github.com/redis/go-redis/v9"
)

type RedisRetryConsumer = redisretry.RedisRetryConsumer

// NewRedisRetryConsumer 创建 Redis 重试消费者。
func NewRedisRetryConsumer(
	brokers []string,
	topic string,
	groupID string,
	redisClient *redis.Client,
	producer *kafka.Producer,
	logger kafka.Logger,
) *RedisRetryConsumer {
	return redisretry.NewRedisRetryConsumer(brokers, topic, groupID, redisClient, producer, logger)
}
