package svc

import (
	"context"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/util"
	"github.com/stretchr/testify/require"
)

func TestAuthenticateUsesStatelessAccessToken(t *testing.T) {
	accessToken, err := util.GenerateToken("user-1", "device-1")
	require.NoError(t, err)

	// 故意不注入 Redis 客户端：AccessToken 是自包含 JWT，握手成功不应依赖任何
	// Redis Token 状态。Redis 仍会被路由和 ACK 使用，但那属于建连后的实时能力。
	service := NewConnectService(nil, nil)
	session, err := service.Authenticate(context.Background(), accessToken, "device-1", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "user-1", session.UserUUID)
	require.Equal(t, "device-1", session.DeviceID)
}

func TestAuthenticateRejectsDeviceMismatch(t *testing.T) {
	accessToken, err := util.GenerateToken("user-1", "device-1")
	require.NoError(t, err)

	service := NewConnectService(nil, nil)
	_, err = service.Authenticate(context.Background(), accessToken, "device-2", "127.0.0.1")
	require.ErrorIs(t, err, ErrTokenInvalid)
}
