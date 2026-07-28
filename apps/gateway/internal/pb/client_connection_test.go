package pb

import (
	"strings"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 禁止写进 gateway 重试白名单的典型写方法，防止后续回归为 Service 级或误放写接口。
var gatewayForbiddenRetryMethods = []string{
	"/auth.AuthService/Register",
	"/auth.AuthService/Login",
	"/auth.AuthService/RefreshToken",
	"/auth.AuthService/SendVerifyCode",
	"/auth.AccountService/ChangePassword",
	"/auth.AccountService/ChangeEmail",
	"/user.UserService/UpdateProfile",
	"/relation.FriendService/SendFriendApply",
	"/relation.FriendService/HandleFriendApply",
	"/group.GroupService/CreateGroup",
	"/group.GroupService/DismissGroup",
	"/group.GroupService/AddMember",
	"/group.GroupService/RemoveMember",
	"/group.GroupService/ReviewJoinGroup",
	"/msg.MsgService/SendMessage",
	"/msg.MsgService/RecallMessage",
	"/msg.MsgService/MarkRead",
	"/msg.MsgService/DeleteConversation",
	"/msg.MsgService/UpdateConversationSettings",
}

func TestGatewayRetryWhitelists_AreExactFullMethodsAndValid(t *testing.T) {
	lists := []struct {
		name    string
		methods []string
	}{
		{"auth", gatewayAuthRetryMethods},
		{"user", gatewayUserRetryMethods},
		{"relation", gatewayRelationRetryMethods},
		{"group", gatewayGroupRetryMethods},
		{"msg", gatewayMsgRetryMethods},
	}
	for _, tc := range lists {
		t.Run(tc.name, func(t *testing.T) {
			require.NotEmpty(t, tc.methods)
			for _, m := range tc.methods {
				assert.True(t, strings.HasPrefix(m, "/"), "full method 必须以 / 开头: %s", m)
				parts := strings.Split(strings.TrimPrefix(m, "/"), "/")
				require.Len(t, parts, 2, "full method 必须是 /package.Service/Method: %s", m)
				assert.NotEmpty(t, parts[0])
				assert.NotEmpty(t, parts[1])
				assert.Contains(t, parts[0], ".", "service 应为 package.Service: %s", m)
			}
			// NewClient 会严格校验 full method；非法或重复配置在启动期失败。
			conn, err := grpcx.NewClient(grpcx.ClientOptions{
				Address: "127.0.0.1:1",
				Retry:   grpcx.DefaultClientRetryConfig(tc.methods...),
			})
			require.NoError(t, err)
			require.NoError(t, conn.Close())
		})
	}
}

func TestGatewayRetryWhitelists_ExcludeWritesAndSendMessage(t *testing.T) {
	allowed := map[string]struct{}{}
	for _, methods := range [][]string{
		gatewayAuthRetryMethods,
		gatewayUserRetryMethods,
		gatewayRelationRetryMethods,
		gatewayGroupRetryMethods,
		gatewayMsgRetryMethods,
	} {
		for _, m := range methods {
			allowed[m] = struct{}{}
		}
	}
	for _, forbidden := range gatewayForbiddenRetryMethods {
		_, ok := allowed[forbidden]
		assert.False(t, ok, "写方法不应出现在 gateway 重试白名单: %s", forbidden)
	}
	// SendMessage 即使有 client_msg_id 也必须排除。
	_, ok := allowed["/msg.MsgService/SendMessage"]
	assert.False(t, ok)
}
