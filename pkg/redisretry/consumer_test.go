package redisretry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type testLogger struct{}

func (testLogger) Info(context.Context, string, map[string]interface{})  {}
func (testLogger) Error(context.Context, string, map[string]interface{}) {}

func TestRedisRetryConsumerDeletesEveryTaskKey(t *testing.T) {
	mr := miniredis.RunT(t)
	require.NoError(t, mr.Set("cache:1", "old"))
	require.NoError(t, mr.Set("cache:2", "old"))

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	consumer := &RedisRetryConsumer{redisClient: client, logger: testLogger{}}
	message, err := json.Marshal(BuildDelTask("cache:1", "cache:2"))
	require.NoError(t, err)
	require.NoError(t, consumer.processMessage(context.Background(), message))
	require.False(t, mr.Exists("cache:1"))
	require.False(t, mr.Exists("cache:2"))
}

func TestRedisRetryConsumerTreatsInvalidPayloadAsPermanent(t *testing.T) {
	consumer := &RedisRetryConsumer{logger: testLogger{}}

	err := consumer.processMessage(context.Background(), []byte("{"))
	require.Error(t, err)
	require.True(t, kafka.IsPermanent(err))

	message, marshalErr := json.Marshal(RedisTask{})
	require.NoError(t, marshalErr)
	err = consumer.processMessage(context.Background(), message)
	require.Error(t, err)
	require.True(t, kafka.IsPermanent(err))
	err = consumer.processMessage(context.Background(), []byte("{\"type\":\"simple\",\"command\":\"set\",\"args\":[\"cache:1\",\"stale\"]}"))
	require.Error(t, err)
	require.True(t, kafka.IsPermanent(err))
}

func TestRedisRetryConsumerLeavesRedisFailureRetryable(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		MaxRetries:   -1,
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	mr.Close()

	consumer := &RedisRetryConsumer{redisClient: client, logger: testLogger{}}
	message, err := json.Marshal(BuildDelTask("cache:1"))
	require.NoError(t, err)

	err = consumer.processMessage(context.Background(), message)
	require.Error(t, err)
	require.False(t, kafka.IsPermanent(err))
}
