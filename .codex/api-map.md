# Gateway API 映射

本文件以 apps/gateway/internal/router/router.go 为路由事实，以对应 Gateway service 为下游映射事实。请求/响应字段以 dto、proto 和实际 handler 为准。

## 通用约束

- 健康检查为 GET /health，指标为 GET /metrics，均无认证且不设请求超时。
- /api/v1/public/user 下的接口不需要 JWT；/api/v1/auth 下的接口都经 JWTAuthMiddleware，调用身份由 JWT 注入请求上下文。
- 所有请求经过追踪、可信代理/客户端 IP、恢复、日志、Prometheus、CORS 和请求级超时中间件。
- IP 限流为每秒 10、突发 20，Redis 不可用时放行；登录后的用户限流为每秒 100、突发 200。
- 未显式列出的路由使用默认 2 秒预算。改路由时同时维护 router.go 的 gatewayRequestTimeouts，避免意外沿用默认。
- 改写接口不因 Gateway 重试而自动安全；下游 gRPC 写方法默认没有重试白名单。

## 公开账号接口

| 方法与路径 | Gateway handler | 下游 RPC/服务 | 超时 |
| --- | --- | --- | --- |
| POST /api/v1/public/user/login | Login | Auth.Login | 3 秒 |
| POST /api/v1/public/user/login-by-code | LoginByCode | Auth.LoginByCode | 3 秒 |
| POST /api/v1/public/user/register | Register | Auth.Register | 5 秒 |
| POST /api/v1/public/user/send-verify-code | SendVerifyCode | Auth.SendVerifyCode | 5 秒 |
| POST /api/v1/public/user/reset-password | ResetPassword | Auth.ResetPassword | 5 秒 |
| POST /api/v1/public/user/refresh-token | RefreshToken | Auth.RefreshToken | 2 秒 |
| POST /api/v1/public/user/verify-code | VerifyCode | Auth.VerifyCode | 2 秒 |
| POST /api/v1/public/user/parse-qrcode | ParseQRCode | User.ParseQRCode | 1 秒 |

## 登录后用户、资料与设备接口

| 方法与路径 | Gateway handler | 下游 RPC/聚合 | 超时 |
| --- | --- | --- | --- |
| GET /api/v1/auth/user/profile | GetProfile | User.GetProfile | 5 秒 |
| PUT /api/v1/auth/user/profile | UpdateProfile | User.UpdateProfile | 5 秒 |
| GET /api/v1/auth/user/profile/:userUuid | GetOtherProfile | User.GetOtherProfile，并补充 Relation.CheckIsFriend | 2 秒 |
| GET /api/v1/auth/user/search | SearchUser | User.SearchUser；邮箱精确检索会组合 Auth.FindAccountByEmail 与 User.BatchGetProfile | 3 秒 |
| POST /api/v1/auth/user/avatar | UploadAvatar | User.UploadAvatar | 3 秒 |
| GET /api/v1/auth/user/qrcode | GetQRCode | User.GetQRCode | 1 秒 |
| POST /api/v1/auth/user/batch-profile | BatchGetProfile | User.BatchGetProfile | 3 秒 |
| GET /api/v1/auth/user/devices | GetDeviceList | Auth.GetDeviceList | 1 秒 |
| DELETE /api/v1/auth/user/devices/:deviceId | KickDevice | Auth.KickDevice | 2 秒 |
| GET /api/v1/auth/user/online-status/:userUuid | GetOnlineStatus | Auth.GetOnlineStatus | 1 秒 |
| POST /api/v1/auth/user/batch-online-status | BatchGetOnlineStatus | Auth.BatchGetOnlineStatus | 2 秒 |
| POST /api/v1/auth/user/change-password | ChangePassword | Auth.ChangePassword；附加用户限流每秒 2、突发 5 | 5 秒 |
| POST /api/v1/auth/user/change-email | ChangeEmail | Auth.ChangeEmail；附加用户限流每秒 2、突发 5 | 3 秒 |
| POST /api/v1/auth/user/delete-account | DeleteAccount | Auth.DeleteAccount；附加用户限流每秒 2、突发 5 | 5 秒 |
| POST /api/v1/auth/user/logout | Logout | Auth.Logout | 1 秒 |

## 好友与黑名单接口

| 方法与路径 | Gateway handler | 下游 RPC | 超时 |
| --- | --- | --- | --- |
| POST /api/v1/auth/friend/apply | SendFriendApply | Relation.SendFriendApply | 2 秒 |
| GET /api/v1/auth/friend/apply-list | GetFriendApplyList | Relation.GetFriendApplyList | 1 秒 |
| GET /api/v1/auth/friend/apply/sent | GetSentApplyList | Relation.GetSentApplyList | 1 秒 |
| POST /api/v1/auth/friend/apply/handle | HandleFriendApply | Relation.HandleFriendApply | 2 秒 |
| GET /api/v1/auth/friend/apply/unread | GetUnreadApplyCount | Relation.GetUnreadApplyCount | 1 秒 |
| POST /api/v1/auth/friend/apply/read | MarkApplyAsRead | Relation.MarkApplyAsRead | 1 秒 |
| GET /api/v1/auth/friend/list | GetFriendList | Relation.GetFriendList | 2 秒 |
| POST /api/v1/auth/friend/sync | SyncFriendList | Relation.SyncFriendList | 2 秒 |
| POST /api/v1/auth/friend/delete | DeleteFriend | Relation.DeleteFriend | 2 秒 |
| POST /api/v1/auth/friend/remark | SetFriendRemark | Relation.SetFriendRemark | 2 秒 |
| POST /api/v1/auth/friend/tag | SetFriendTag | Relation.SetFriendTag | 2 秒 |
| POST /api/v1/auth/friend/check | CheckIsFriend | Relation.CheckIsFriend | 1 秒 |
| POST /api/v1/auth/friend/relation | GetRelationStatus | Relation.GetRelationStatus | 1 秒 |
| POST /api/v1/auth/blacklist | AddBlacklist | Relation.AddBlacklist | 2 秒 |
| GET /api/v1/auth/blacklist | GetBlacklistList | Relation.GetBlacklistList | 2 秒 |
| DELETE /api/v1/auth/blacklist/:userUuid | RemoveBlacklist | Relation.RemoveBlacklist | 2 秒 |
| POST /api/v1/auth/blacklist/check | CheckIsBlacklist | Relation.CheckIsBlacklist | 1 秒 |

当前黑名单 check 仍对外暴露。它的目标用户授权边界要随 DTO、Relation RPC 与业务要求一并核对，不能只按路径名称判断安全性。

## 消息与会话接口

| 方法与路径 | Gateway handler | 下游 RPC | 超时 |
| --- | --- | --- | --- |
| POST /api/v1/auth/messages/send | SendMessage | Msg.SendMessage | 3 秒 |
| GET /api/v1/auth/messages/pull | PullMessages | Msg.PullMessages | 2 秒 |
| POST /api/v1/auth/messages/sync-batch | BatchSyncMessages | Msg.BatchSyncMessages；附加用户限流每秒 5、突发 10 | 默认 2 秒 |
| POST /api/v1/auth/messages/get-by-ids | GetMessagesByIds | Msg.GetMessagesByIds | 2 秒 |
| POST /api/v1/auth/messages/recall | RecallMessage | Msg.RecallMessage | 2 秒 |
| GET /api/v1/auth/conversations | GetConversations | Msg.GetConversations | 2 秒 |
| POST /api/v1/auth/conversations/mark-read | MarkRead | Msg.MarkRead | 2 秒 |
| DELETE /api/v1/auth/conversations/:convId | DeleteConversation | Msg.DeleteConversation | 2 秒 |
| PATCH /api/v1/auth/conversations/settings | UpdateConversationSettings | Msg.UpdateConversationSettings | 2 秒 |

## 群接口

| 方法与路径 | Gateway handler | 下游 RPC | 超时 |
| --- | --- | --- | --- |
| POST /api/v1/auth/groups | CreateGroup | Group.CreateGroup | 2 秒 |
| GET /api/v1/auth/groups | GetGroupList | Group.GetGroupList | 2 秒 |
| GET /api/v1/auth/groups/search | SearchGroups | Group.SearchGroups | 3 秒 |
| GET /api/v1/auth/groups/:groupUuid | GetGroupInfo | Group.GetGroupInfo | 2 秒 |
| PATCH /api/v1/auth/groups/:groupUuid | UpdateGroupInfo | Group.UpdateGroupInfo | 默认 2 秒 |
| PUT /api/v1/auth/groups/:groupUuid/notice | UpdateGroupNotice | Group.UpdateGroupNotice | 2 秒 |
| POST /api/v1/auth/groups/:groupUuid/apply | ApplyJoinGroup | Group.ApplyJoinGroup | 2 秒 |
| DELETE /api/v1/auth/groups/:groupUuid/apply | CancelJoinGroupApplication | Group.CancelJoinGroupApplication | 默认 2 秒 |
| GET /api/v1/auth/groups/join-applications | ListMyJoinGroupApplications | Group.ListMyJoinGroupApplications | 1 秒 |
| GET /api/v1/auth/groups/:groupUuid/my-join-application | GetMyJoinGroupApplication | Group.GetMyJoinGroupApplication | 1 秒 |
| GET /api/v1/auth/groups/:groupUuid/join-requests | ListJoinRequests | Group.ListJoinRequests | 2 秒 |
| GET /api/v1/auth/groups/:groupUuid/join-requests/pending-count | GetJoinRequestPendingCount | Group.GetJoinRequestPendingCount | 1 秒 |
| GET /api/v1/auth/groups/:groupUuid/join-requests/reviewed | ListReviewedJoinRequests | Group.ListReviewedJoinRequests | 2 秒 |
| POST /api/v1/auth/groups/:groupUuid/join-requests/:applyId/review | ReviewJoinGroup | Group.ReviewJoinGroup | 2 秒 |
| POST /api/v1/auth/groups/:groupUuid/transfer-owner | TransferGroupOwner | Group.TransferGroupOwner | 2 秒 |
| POST /api/v1/auth/groups/:groupUuid/leave | LeaveGroup | Group.LeaveGroup | 2 秒 |
| PATCH /api/v1/auth/groups/:groupUuid/my-nickname | UpdateMyGroupNickname | Group.UpdateMyGroupNickname | 2 秒 |
| PATCH /api/v1/auth/groups/:groupUuid/mute-setting | UpdateGroupMuteSetting | Group.UpdateGroupMuteSetting | 2 秒 |
| DELETE /api/v1/auth/groups/:groupUuid | DismissGroup | Group.DismissGroup | 默认 2 秒 |
| POST /api/v1/auth/groups/:groupUuid/members | AddMember | Group.AddMember | 2 秒 |
| GET /api/v1/auth/groups/:groupUuid/members/search | SearchGroupMembers | Group.SearchGroupMembers | 3 秒 |
| GET /api/v1/auth/groups/:groupUuid/members | GetMemberList | Group.GetMemberList | 2 秒 |
| DELETE /api/v1/auth/groups/:groupUuid/members/:userUuid | RemoveMember | Group.RemoveMember | 2 秒 |
| PATCH /api/v1/auth/groups/:groupUuid/members/:userUuid/nickname | UpdateGroupMemberNickname | Group.UpdateGroupMemberNickname | 2 秒 |
| PATCH /api/v1/auth/groups/:groupUuid/members/:userUuid/mute | MuteGroupMember | Group.MuteGroupMember | 2 秒 |
| PATCH /api/v1/auth/groups/:groupUuid/members/:userUuid/role | UpdateMemberRole | Group.UpdateMemberRole | 2 秒 |
| GET /api/v1/auth/groups/:groupUuid/member-ids | GetGroupMemberIDs | Group.GetGroupMemberIds | 1 秒 |

路由顺序是契约的一部分：/groups/search 必须位于 /groups/:groupUuid 之前。新增静态子路径时同样避免被动态参数路由吞掉。
