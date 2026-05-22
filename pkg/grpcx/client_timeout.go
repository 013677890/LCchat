package grpcx

import (
	"context"
	"google.golang.org/grpc"
	"time"
)

// ClientTimeoutConfig 定义 gRPC 客户端按方法收紧 deadline 的策略。
// 这里先以内存 map 维护推荐值，后续可平滑演进到配置中心下发。
type ClientTimeoutConfig struct {
	// MethodTimeouts 按 full method 指定超时，例如 /user.AuthService/Login。
	MethodTimeouts map[string]time.Duration
}

// ClientTimeoutUnaryInterceptor 为上游调用方补齐下游方法级 deadline。
// 规则：实际生效 deadline = min(父 deadline, 方法推荐 timeout)。
func ClientTimeoutUnaryInterceptor(cfg ClientTimeoutConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timeout := resolveClientTimeout(method, cfg)
		if timeout <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		callCtx, cancel, _ := newGRPCTimeoutContext(ctx, timeout)
		defer cancel()
		return invoker(callCtx, method, req, reply, cc, opts...)
	}
}

func resolveClientTimeout(fullMethod string, cfg ClientTimeoutConfig) time.Duration {
	if cfg.MethodTimeouts == nil {
		return 0
	}
	return cfg.MethodTimeouts[fullMethod]
}

// DefaultClientMethodTimeouts 返回当前内置的下游方法推荐超时。
// 调用方拿到的是副本，可按需增删而不影响全局默认值。
func DefaultClientMethodTimeouts() map[string]time.Duration {
	return cloneDurationMap(defaultClientMethodTimeouts)
}

func cloneDurationMap(src map[string]time.Duration) map[string]time.Duration {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]time.Duration, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

var defaultClientMethodTimeouts = map[string]time.Duration{
	// 认证链路包含 bcrypt 与 SMTP 等重操作，500ms 在本地/容器环境下容易误判超时。
	"/auth.AuthService/Register":                         2 * time.Second,
	"/auth.AuthService/Login":                            1500 * time.Millisecond,
	"/auth.AuthService/LoginByCode":                      1500 * time.Millisecond,
	"/auth.AuthService/SendVerifyCode":                   5 * time.Second,
	"/auth.AuthService/VerifyCode":                       500 * time.Millisecond,
	"/auth.AuthService/RefreshToken":                     300 * time.Millisecond,
	"/auth.AuthService/Logout":                           1000 * time.Millisecond,
	"/auth.AuthService/ResetPassword":                    2 * time.Second,
	"/auth.AccountService/ChangePassword":                2 * time.Second,
	"/auth.AccountService/ChangeEmail":                   1500 * time.Millisecond,
	"/auth.AccountService/ChangeTelephone":               500 * time.Millisecond,
	"/auth.AccountService/DeleteAccount":                 2 * time.Second,
	"/auth.InternalAuthService/FindAccountByEmail":       300 * time.Millisecond,
	"/auth.InternalAuthService/FindAccountByTelephone":   300 * time.Millisecond,
	"/auth.InternalAuthService/UpdateLoginDisplay":       300 * time.Millisecond,
	"/auth.InternalAuthService/BatchCheckAccountStatus":  500 * time.Millisecond,
	"/auth.DeviceService/GetDeviceList":                  300 * time.Millisecond,
	"/auth.DeviceService/KickDevice":                     500 * time.Millisecond,
	"/auth.DeviceService/GetOnlineStatus":                300 * time.Millisecond,
	"/auth.DeviceService/BatchGetOnlineStatus":           800 * time.Millisecond,
	"/auth.DeviceService/UpdateDeviceActive":             800 * time.Millisecond,
	"/auth.DeviceService/UpdateDeviceStatus":             500 * time.Millisecond,
	"/user.UserService/GetProfile":                       300 * time.Millisecond,
	"/user.UserService/GetOtherProfile":                  300 * time.Millisecond,
	"/user.UserService/SearchUser":                       800 * time.Millisecond,
	// 资料更新要写入主表并触发展示字段事件，预算不能压得和只读接口一样紧。
	"/user.UserService/UpdateProfile":                    5 * time.Second,
	"/user.UserService/UploadAvatar":                     500 * time.Millisecond,
	"/user.UserService/GetQRCode":                        300 * time.Millisecond,
	"/user.UserService/ParseQRCode":                      300 * time.Millisecond,
	"/user.UserService/BatchGetProfile":                  800 * time.Millisecond,
	"/user.InternalProfileService/CreateProfile":         500 * time.Millisecond,
	"/user.InternalProfileService/BatchGetUserCard":      800 * time.Millisecond,
	"/user.InternalProfileService/BatchGetPublicProfile": 800 * time.Millisecond,
	"/relation.FriendService/SendFriendApply":            500 * time.Millisecond,
	"/relation.FriendService/GetFriendApplyList":         300 * time.Millisecond,
	"/relation.FriendService/GetSentApplyList":           300 * time.Millisecond,
	"/relation.FriendService/HandleFriendApply":          500 * time.Millisecond,
	"/relation.FriendService/GetUnreadApplyCount":        300 * time.Millisecond,
	"/relation.FriendService/MarkApplyAsRead":            300 * time.Millisecond,
	"/relation.FriendService/GetFriendList":              300 * time.Millisecond,
	"/relation.FriendService/SyncFriendList":             800 * time.Millisecond,
	"/relation.FriendService/DeleteFriend":               500 * time.Millisecond,
	"/relation.FriendService/SetFriendRemark":            500 * time.Millisecond,
	"/relation.FriendService/SetFriendTag":               500 * time.Millisecond,
	"/relation.FriendService/CheckIsFriend":              300 * time.Millisecond,
	"/relation.FriendService/BatchCheckIsFriend":         800 * time.Millisecond,
	"/relation.FriendService/GetRelationStatus":          300 * time.Millisecond,
	"/relation.BlacklistService/AddBlacklist":            500 * time.Millisecond,
	"/relation.BlacklistService/RemoveBlacklist":         500 * time.Millisecond,
	"/relation.BlacklistService/GetBlacklistList":        300 * time.Millisecond,
	"/relation.BlacklistService/CheckIsBlacklist":        300 * time.Millisecond,
	"/group.GroupService/CreateGroup":                    800 * time.Millisecond,
	"/group.GroupService/DismissGroup":                   800 * time.Millisecond,
	"/group.GroupService/GetGroupInfo":                   300 * time.Millisecond,
	"/group.GroupService/UpdateGroupInfo":                500 * time.Millisecond,
	"/group.GroupService/UpdateGroupNotice":              500 * time.Millisecond,
	"/group.GroupService/TransferGroupOwner":             500 * time.Millisecond,
	"/group.GroupService/UpdateMemberRole":               500 * time.Millisecond,
	"/group.GroupService/ApplyJoinGroup":                 500 * time.Millisecond,
	"/group.GroupService/CancelJoinGroupApplication":     500 * time.Millisecond,
	"/group.GroupService/GetMyJoinGroupApplication":      300 * time.Millisecond,
	"/group.GroupService/ListMyJoinGroupApplications":    500 * time.Millisecond,
	"/group.GroupService/ReviewJoinGroup":                500 * time.Millisecond,
	"/group.GroupService/ListJoinRequests":               500 * time.Millisecond,
	"/group.GroupService/ListReviewedJoinRequests":       500 * time.Millisecond,
	"/group.GroupService/AddMember":                      800 * time.Millisecond,
	"/group.GroupService/LeaveGroup":                     500 * time.Millisecond,
	"/group.GroupService/RemoveMember":                   500 * time.Millisecond,
	"/group.GroupService/GetMemberList":                  800 * time.Millisecond,
	"/group.GroupService/SearchGroupMembers":             800 * time.Millisecond,
	"/group.GroupService/UpdateMyGroupNickname":          500 * time.Millisecond,
	"/group.GroupService/MuteGroupMember":                500 * time.Millisecond,
	"/group.GroupService/UpdateGroupMuteSetting":         500 * time.Millisecond,
	"/group.GroupService/GetGroupList":                   500 * time.Millisecond,
	"/group.GroupService/SearchGroups":                   800 * time.Millisecond,
	"/group.GroupService/GetGroupMemberIds":              500 * time.Millisecond,
	"/group.GroupService/CheckGroupMember":               300 * time.Millisecond,
	"/group.GroupService/CheckGroupSendPermission":       300 * time.Millisecond,
	"/group.GroupService/GetJoinRequestPendingCount":     300 * time.Millisecond,
	"/group.GroupService/UpdateGroupMemberNickname":      500 * time.Millisecond,
	"/msg.MsgService/SendMessage":                        1000 * time.Millisecond,
	"/msg.MsgService/PullMessages":                       500 * time.Millisecond,
	"/msg.MsgService/GetMessagesByIds":                   500 * time.Millisecond,
	// 撤回消息会先更新 DB，再尽力投递撤回通知；本地三副本环境下 500ms 容易误伤。
	"/msg.MsgService/RecallMessage":                      1500 * time.Millisecond,
	"/msg.MsgService/GetConversations":                   800 * time.Millisecond,
	"/msg.MsgService/MarkRead":                           500 * time.Millisecond,
	"/msg.MsgService/DeleteConversation":                 500 * time.Millisecond,
	"/msg.MsgService/UpdateConversationSettings":         500 * time.Millisecond,
	"/connect.ConnectService/PushToDevice":               300 * time.Millisecond,
	"/connect.ConnectService/PushToUser":                 300 * time.Millisecond,
	"/connect.ConnectService/BroadcastToUsers":           500 * time.Millisecond,
	"/connect.ConnectService/KickConnection":             300 * time.Millisecond,
}
