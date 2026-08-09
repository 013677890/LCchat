package redisretry

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type testRedisError string

func (e testRedisError) Error() string { return string(e) }
func (testRedisError) RedisError()     {}

func TestWriteFailureHookReportsEvalKeysAfterFinalFailure(t *testing.T) {
	wantErr := errors.New("redis unavailable")
	var gotTask RedisTask
	var gotErr error
	hook := newWriteFailureHook("relation-service.redis-write", func(_ context.Context, task RedisTask, err error) {
		gotTask = task
		gotErr = err
	})
	cmd := redis.NewCmd(context.Background(), "eval", "return 1", 2, "friend:u1", "friend:u2", "arg")

	err := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return wantErr
	})(context.Background(), cmd)

	require.ErrorIs(t, err, wantErr)
	require.ErrorIs(t, gotErr, wantErr)
	require.Equal(t, []string{"friend:u1", "friend:u2"}, gotTask.Keys)
	require.Equal(t, "relation-service.redis-write.eval", gotTask.Source)
}

func TestWriteFailureHookPipelineReportsOnlyFailedWriteKeys(t *testing.T) {
	wantErr := errors.New("pipeline connection failed")
	var gotTask RedisTask
	hook := newWriteFailureHook("relation-service.redis-write", func(_ context.Context, task RedisTask, _ error) {
		gotTask = task
	})
	readCmd := redis.NewStringCmd(context.Background(), "get", "profile:u1")
	readCmd.SetErr(wantErr)
	writeCmd := redis.NewStatusCmd(context.Background(), "expire", "friend:u1", 60)
	writeCmd.SetErr(wantErr)

	err := hook.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return wantErr
	})(context.Background(), []redis.Cmder{readCmd, writeCmd})

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, []string{"friend:u1"}, gotTask.Keys)
	require.Equal(t, "relation-service.redis-write.pipeline", gotTask.Source)
}

func TestWriteFailureHookDoesNotRecursivelyReportCompensationDel(t *testing.T) {
	wantErr := errors.New("redis unavailable")
	reports := 0
	hook := newWriteFailureHook("relation-service.redis-write", func(context.Context, RedisTask, error) {
		reports++
	})
	cmd := redis.NewIntCmd(context.Background(), "del", "friend:u1")

	err := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return wantErr
	})(withCompensationContext(context.Background()), cmd)

	require.ErrorIs(t, err, wantErr)
	require.Zero(t, reports)
}

func TestWriteFailureHookIgnoresReadFailure(t *testing.T) {
	wantErr := errors.New("redis unavailable")
	reports := 0
	hook := newWriteFailureHook("relation-service.redis-write", func(context.Context, RedisTask, error) {
		reports++
	})
	cmd := redis.NewStringCmd(context.Background(), "get", "friend:u1")

	_ = hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return wantErr
	})(context.Background(), cmd)

	require.Zero(t, reports)
}

func TestWriteFailureHookIgnoresEvalShaNoScriptFallback(t *testing.T) {
	reports := 0
	hook := newWriteFailureHook("relation-service.redis-write", func(context.Context, RedisTask, error) {
		reports++
	})
	cmd := redis.NewCmd(context.Background(), "evalsha", "hash", 1, "friend:u1")
	noscriptErr := testRedisError("NOSCRIPT No matching script. Please use EVAL")

	_ = hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return noscriptErr
	})(context.Background(), cmd)

	require.Zero(t, reports)
}

func TestWriteFailureHookDoesNotInvalidateSuccessfulWriteAfterReadError(t *testing.T) {
	wrongTypeErr := errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	reports := 0
	hook := newWriteFailureHook("relation-service.redis-write", func(context.Context, RedisTask, error) {
		reports++
	})
	readCmd := redis.NewStringCmd(context.Background(), "hget", "friend:u1", "user-2")
	readCmd.SetErr(wrongTypeErr)
	writeCmd := redis.NewStatusCmd(context.Background(), "expire", "friend:u1", 60)

	_ = hook.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return wrongTypeErr
	})(context.Background(), []redis.Cmder{readCmd, writeCmd})

	require.Zero(t, reports)
}

func TestWriteKeysSupportsOnlyCurrentRelationCommands(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		cmd  redis.Cmder
		want []string
	}{
		{name: "del", cmd: redis.NewCmd(ctx, "del", "cache:1", "cache:2"), want: []string{"cache:1", "cache:2"}},
		{name: "eval", cmd: redis.NewCmd(ctx, "eval", "return 1", 1, "cache:1"), want: []string{"cache:1"}},
		{name: "evalsha", cmd: redis.NewCmd(ctx, "evalsha", "hash", 1, "cache:1"), want: []string{"cache:1"}},
		{name: "set", cmd: redis.NewCmd(ctx, "set", "cache:1", "value"), want: []string{"cache:1"}},
		{name: "incr", cmd: redis.NewCmd(ctx, "incr", "cache:1"), want: []string{"cache:1"}},
		{name: "expire", cmd: redis.NewCmd(ctx, "expire", "cache:1", 60), want: []string{"cache:1"}},
		{name: "hset", cmd: redis.NewCmd(ctx, "hset", "cache:1", "field", "value"), want: []string{"cache:1"}},
		{name: "zadd", cmd: redis.NewCmd(ctx, "zadd", "cache:1", 1, "member"), want: []string{"cache:1"}},
		{name: "future command rejected", cmd: redis.NewCmd(ctx, "mset", "cache:1", "value"), want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, writeKeys(test.cmd))
		})
	}
}
