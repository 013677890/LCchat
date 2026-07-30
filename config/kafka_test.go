package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultKafkaConfigSeparatesAuthAndUserRedisInvalidation(t *testing.T) {
	t.Setenv("KAFKA_AUTH_REDIS_RETRY_TOPIC", "")
	t.Setenv("KAFKA_AUTH_REDIS_RETRY_GROUP_ID", "")
	t.Setenv("KAFKA_USER_REDIS_RETRY_TOPIC", "")
	t.Setenv("KAFKA_USER_REDIS_RETRY_GROUP_ID", "")

	cfg := DefaultKafkaConfig()

	require.Equal(t, "auth.redis.invalidate", cfg.AuthRedisRetryTopic)
	require.Equal(t, "auth-redis-invalidate-group", cfg.AuthRedisRetryGroupID)
	require.Equal(t, "user.redis.invalidate", cfg.UserRedisRetryTopic)
	require.Equal(t, "user-redis-invalidate-group", cfg.UserRedisRetryGroupID)
	require.NotEqual(t, cfg.AuthRedisRetryTopic, cfg.UserRedisRetryTopic)
	require.NotEqual(t, cfg.AuthRedisRetryGroupID, cfg.UserRedisRetryGroupID)
}
