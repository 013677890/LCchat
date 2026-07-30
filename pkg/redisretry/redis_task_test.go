package redisretry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildDelTaskCopiesAndValidatesKeys(t *testing.T) {
	keys := []string{"user:1", "user:2"}
	task := BuildDelTask(keys...)
	keys[0] = "changed"

	require.Equal(t, []string{"user:1", "user:2"}, task.Keys)
	require.NoError(t, task.Validate())
	require.False(t, task.Timestamp.IsZero())
}

func TestRedisTaskValidateRejectsMissingOrEmptyKeys(t *testing.T) {
	require.Error(t, (RedisTask{}).Validate())
	require.Error(t, (RedisTask{Keys: []string{"valid", ""}}).Validate())
}
