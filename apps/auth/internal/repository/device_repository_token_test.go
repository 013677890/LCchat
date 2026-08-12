package repository

import (
	"context"
	"testing"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestDeviceRepositoryRefreshTokenLifecycle(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	repository := &deviceRepositoryImpl{redisClient: redisClient}
	ctx := context.Background()
	key := rediskey.RefreshTokenKey("user-1", "device-1")

	// AccessToken 已经完全移出仓储契约；这个生命周期测试只覆盖服务端真正持久化的
	// RefreshToken，防止以后又把两类 Token 重新耦合进同一个删除方法。
	require.NoError(t, repository.StoreRefreshToken(ctx, "user-1", "device-1", "refresh-1", time.Hour))
	storedRefreshToken, err := miniRedis.Get(key)
	require.NoError(t, err)
	require.Equal(t, "refresh-1", storedRefreshToken)

	require.NoError(t, repository.DeleteRefreshToken(ctx, "user-1", "device-1"))
	require.False(t, miniRedis.Exists(key))
}

func TestDeviceRepositoryDeleteRefreshTokenFailsWithoutRedis(t *testing.T) {
	repository := &deviceRepositoryImpl{}

	// 撤销续期凭据属于安全主链路。没有 Redis 客户端时必须明确失败，不能让登出、
	// 修改密码或重置密码在没有实际撤销任何凭据的情况下向上游报告成功。
	err := repository.DeleteRefreshToken(context.Background(), "user-1", "device-1")
	require.ErrorIs(t, err, repoerr.ErrRedis)
}
