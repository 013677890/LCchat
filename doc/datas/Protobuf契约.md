# Protobuf 契约

本文整理当前 `proto/` 目录中的 gRPC 与消息协议契约，帮助前端、Gateway 和微服务开发快速定位字段来源。HTTP JSON 字段以 Gateway DTO 为准；服务间通信和 WebSocket 二进制帧以 Protobuf 为准。

## 1. 契约总览

| 文件 | 服务/消息 | 主要用途 |
| --- | --- | --- |
| `proto/auth/auth_service.proto` | `AuthService` | 注册、登录、验证码、刷新 Token、登出、重置密码。 |
| `proto/auth/account_service.proto` | `AccountService` | 修改密码、换绑邮箱/手机号、注销账号。 |
| `proto/auth/device_service.proto` | `DeviceService` | 设备列表、踢设备、在线状态、设备活跃同步。 |
| `proto/auth/internal_auth_service.proto` | `InternalAuthService` | auth 内部账号能力。 |
| `proto/user/user_service.proto` | `UserService` | 用户资料、搜索、头像、二维码、批量资料。 |
| `proto/user/internal_profile_service.proto` | `InternalProfileService` | 内部资料聚合和初始化。 |
| `proto/user/group_service.proto` | user 内部群相关契约 | 用户域对群展示的辅助能力。 |
| `proto/relation/friend_service.proto` | `FriendService` | 好友申请、好友列表、同步、关系判断。 |
| `proto/relation/blacklist_service.proto` | `BlacklistService` | 黑名单增删查和判断。 |
| `proto/group/group_service.proto` | `GroupService` | 群资料、成员、入群申请、权限校验。 |
| `proto/msg/msg_service.proto` | `MsgService` | 消息发送、拉取、撤回、会话管理。 |
| `proto/msg/msg_common.proto` | `MsgItem`、`ConversationItem` | 消息和会话通用模型。 |
| `proto/msg/msg_push_event.proto` | `MsgPushEvent` | Kafka `msg.push` 下行事件。 |
| `proto/connect/connect.proto` | `ConnectService`、`MessageEnvelope` | Connect gRPC 和 WebSocket 外层帧。 |
| `proto/connect/ws_control.proto` | `MessageAck`、`ErrorFrame` | WebSocket 控制帧 payload。 |
| `proto/common/common.proto` | 通用模型 | 多服务共享消息。 |

## 2. HTTP DTO 与 Protobuf 的关系

Gateway 对外暴露 HTTP JSON，不直接暴露 Protobuf 字段名。

| 层 | 字段命名 | 示例 |
| --- | --- | --- |
| HTTP JSON | DTO 标签为准，多数 camelCase | `verifyCode`、`userUuid`、`clientMsgId`。 |
| Protobuf | snake_case | `verify_code`、`user_uuid`、`client_msg_id`。 |
| Go | 驼峰 | `VerifyCode`、`UserUUID`、`ClientMsgID`。 |

维护要求：

1. 修改 Protobuf 后必须同步 Gateway DTO 转换函数。
2. 修改 HTTP DTO JSON 标签后必须同步 API 文档。
3. 不允许前端按 Protobuf 字段名调用 HTTP，除非 DTO 本身明确使用下划线，例如当前刷新 Token 的 `device_id`。

## 3. auth 契约

### 3.1 `AuthService`

| RPC | 请求 | 响应 | HTTP 对应 |
| --- | --- | --- | --- |
| `Register` | `RegisterRequest` | `RegisterResponse` | `POST /api/v1/public/user/register` |
| `Login` | `LoginRequest` | `LoginResponse` | `POST /api/v1/public/user/login` |
| `LoginByCode` | `LoginByCodeRequest` | `LoginByCodeResponse` | `POST /api/v1/public/user/login-by-code` |
| `SendVerifyCode` | `SendVerifyCodeRequest` | `SendVerifyCodeResponse` | `POST /api/v1/public/user/send-verify-code` |
| `VerifyCode` | `VerifyCodeRequest` | `VerifyCodeResponse` | `POST /api/v1/public/user/verify-code` |
| `RefreshToken` | `RefreshTokenRequest` | `RefreshTokenResponse` | `POST /api/v1/public/user/refresh-token` |
| `Logout` | `LogoutRequest` | `LogoutResponse` | `POST /api/v1/auth/user/logout` |
| `ResetPassword` | `ResetPasswordRequest` | `ResetPasswordResponse` | `POST /api/v1/public/user/reset-password` |

关键对象：

| 对象 | 字段 | 说明 |
| --- | --- | --- |
| `DeviceInfo` | `device_name/platform/os_version/app_version` | 登录时上报设备信息。 |
| `LoginResponse` | `access_token/refresh_token/token_type/expires_in/user_info` | 登录返回 Token 和最小资料。 |

### 3.2 `AccountService` 与 `DeviceService`

`AccountService` 负责账号安全操作：`ChangePassword`、`ChangeEmail`、`ChangeTelephone`、`DeleteAccount`。

`DeviceService` 负责设备能力：`GetDeviceList`、`KickDevice`、`GetOnlineStatus`、`BatchGetOnlineStatus`、`UpdateDeviceActive`、`UpdateDeviceStatus`。

## 4. user 契约

| RPC | 说明 | HTTP 对应 |
| --- | --- | --- |
| `GetProfile` | 当前用户完整资料 | `GET /api/v1/auth/user/profile` |
| `GetOtherProfile` | 他人公开资料 | `GET /api/v1/auth/user/profile/:userUuid` |
| `SearchUser` | 搜索用户 | `GET /api/v1/auth/user/search` |
| `UpdateProfile` | 更新资料 | `PUT /api/v1/auth/user/profile` |
| `UploadAvatar` | 更新头像 URL | `POST /api/v1/auth/user/avatar` |
| `GetQRCode` | 获取用户二维码 | `GET /api/v1/auth/user/qrcode` |
| `ParseQRCode` | 解析二维码 token | `POST /api/v1/public/user/parse-qrcode` |
| `BatchGetProfile` | 批量资料卡片 | `POST /api/v1/auth/user/batch-profile` |

`UserInfo` 与 HTTP `UserProfile` 对应，包含 `uuid`、`nickname`、`avatar`、`gender`、`signature`、`birthday`。

## 5. relation 契约

### 5.1 `FriendService`

覆盖好友申请、收到/发出申请列表、处理申请、未读数、标记已读、好友列表、增量同步、删除好友、备注、标签、好友判断和关系状态。

关键设计：

- 好友关系为单向记录，成为好友时双方各有一条正常关系。
- `SyncFriendList` 通过版本号返回增量变更。
- `GetRelationStatus` 聚合好友和黑名单状态，供前端展示和 msg 权限判断参考。

### 5.2 `BlacklistService`

覆盖拉黑、取消拉黑、黑名单列表、判断是否拉黑。黑名单会影响好友关系展示和消息发送权限。

## 6. group 契约

`GroupService` 是群域唯一对外 gRPC 服务，能力包括：

| 分类 | RPC |
| --- | --- |
| 群资料 | `CreateGroup`、`DismissGroup`、`GetGroupInfo`、`UpdateGroupInfo`、`UpdateGroupNotice`、`GetGroupList` |
| 入群申请 | `ApplyJoinGroup`、`CancelJoinGroupApplication`、`GetMyJoinGroupApplication`、`ListMyJoinGroupApplications`、`ReviewJoinGroup`、`ListJoinRequests`、`ListReviewedJoinRequests`、`GetJoinRequestPendingCount` |
| 成员管理 | `AddMember`、`LeaveGroup`、`RemoveMember`、`GetMemberList`、`SearchGroupMembers`、`UpdateMyGroupNickname` |
| 权限设置 | `TransferGroupOwner`、`UpdateMemberRole`、`MuteGroupMember`、`UpdateGroupMuteSetting` |
| 内部校验 | `GetGroupMemberIds`、`CheckGroupMember`、`CheckGroupSendPermission` |

HTTP 返回的 `GroupInfoDTO` 与 proto 群资料响应字段对应：`group_uuid`、`name`、`avatar`、`owner_uuid`、`member_count`、`notice`、`add_mode`、`mute_all`。

## 7. msg 契约

### 7.1 `MsgService`

| RPC | 说明 |
| --- | --- |
| `SendMessage` | 发送单聊/群聊消息，幂等落库并触发下行事件。 |
| `PullMessages` | 基于 `conv_id + anchor_seq + direction` 拉取历史。 |
| `GetMessagesByIds` | 批量反查消息。 |
| `RecallMessage` | 撤回消息。 |
| `GetConversations` | 获取会话列表，支持全量和增量。 |
| `MarkRead` | 标记已读并触发多端同步。 |
| `DeleteConversation` | 当前用户视角逻辑删除会话。 |
| `UpdateConversationSettings` | 更新免打扰/置顶。 |

### 7.2 通用模型

| 模型 | 说明 |
| --- | --- |
| `MsgItem` | 消息完整结构，用于拉取和 WebSocket 下行。 |
| `LastMsgPreview` | 会话列表最后消息预览。 |
| `ConversationItem` | 会话列表项。 |

枚举：`ConvType` 中 `1=P2P`，`2=GROUP`；`PullDirection` 中 `1=FORWARD`，`2=BACKWARD`。

## 8. connect 契约

### 8.1 `ConnectService`

| RPC | 调用方 | 说明 |
| --- | --- | --- |
| `PushToDevice` | message-push | 投递到指定设备。 |
| `PushToUser` | message-push | 投递到本节点某用户所有设备。 |
| `BroadcastToUsers` | 管理能力 | 批量广播。 |
| `KickConnection` | auth/管理能力 | 踢掉指定连接。 |

### 8.2 WebSocket Protobuf

| 模型 | 说明 |
| --- | --- |
| `MessageEnvelope` | WebSocket Binary Frame 外层包。 |
| `MessageAck` | 客户端 ACK payload。 |
| `MessageAckAck` | 服务端 ACK 确认 payload。 |
| `ErrorFrame` | 协议层错误 payload。 |

详见 [WebSocket协议](../api/06-WebSocket协议.md)。

## 9. 维护规则

1. Protobuf 字段一旦被前端或跨服务使用，修改必须同步更新 API、服务文档和转换函数。
2. 删除字段前应同步删除所有 DTO 转换和前端文档，不保留无用兼容逻辑。
3. 新增 gRPC 服务必须补齐服务文档、调用链路文档和错误码映射。
4. WebSocket payload 新增 type 时必须同步 `msg_push_event.proto`、`message-push` 和 `06-WebSocket协议.md`。
