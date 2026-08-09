package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultKafkaConfigSeparatesRedisInvalidationByService(t *testing.T) {
	t.Setenv("KAFKA_AUTH_REDIS_RETRY_TOPIC", "")
	t.Setenv("KAFKA_AUTH_REDIS_RETRY_GROUP_ID", "")
	t.Setenv("KAFKA_USER_REDIS_RETRY_TOPIC", "")
	t.Setenv("KAFKA_USER_REDIS_RETRY_GROUP_ID", "")
	t.Setenv("KAFKA_RELATION_REDIS_RETRY_TOPIC", "")
	t.Setenv("KAFKA_RELATION_REDIS_RETRY_GROUP_ID", "")

	cfg := DefaultKafkaConfig()

	require.Equal(t, "auth.redis.invalidate", cfg.AuthRedisRetryTopic)
	require.Equal(t, "auth-redis-invalidate-group", cfg.AuthRedisRetryGroupID)
	require.Equal(t, "user.redis.invalidate", cfg.UserRedisRetryTopic)
	require.Equal(t, "user-redis-invalidate-group", cfg.UserRedisRetryGroupID)
	require.Equal(t, "relation.redis.invalidate", cfg.RelationRedisRetryTopic)
	require.Equal(t, "relation-redis-invalidate-group", cfg.RelationRedisRetryGroupID)
	require.Len(t, map[string]struct{}{
		cfg.AuthRedisRetryTopic:     {},
		cfg.UserRedisRetryTopic:     {},
		cfg.RelationRedisRetryTopic: {},
	}, 3)
	require.Len(t, map[string]struct{}{
		cfg.AuthRedisRetryGroupID:     {},
		cfg.UserRedisRetryGroupID:     {},
		cfg.RelationRedisRetryGroupID: {},
	}, 3)
}
