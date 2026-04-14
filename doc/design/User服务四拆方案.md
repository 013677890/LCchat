# User 服务四拆方案

> 基于现有 `apps/user` 代码与当前业务边界，将用户域拆分为四个独立服务，并同步完成数据拆表、proto 全量正名、表名统一与跨服务调用梳理。

---

## 一、拆分总览

```text
apps/user（现在，单体）
    ↓
apps/auth-service      认证 + 设备会话 + 账号生命周期
apps/relation-service  好友申请 + 好友关系 + 黑名单
apps/user-service      用户资料（建议语义上理解为 profile-service）
apps/group-service     群组（新建）
```

| 新服务 | gRPC 端口 | 承接的 proto Service | 职责摘要 |
|---|---|---|---|
| `auth-service` | `:9090` | `AuthService` + `DeviceService` + `AccountService`（新建） | 登录、注册、验证码、Token、设备会话、账号安全、账号注销 |
| `relation-service` | `:9093` | `FriendService` + `BlacklistService` | 好友申请、好友关系、标签、黑名单 |
| `user-service` | `:9094` | `UserService` | 用户资料、公开资料、搜索、二维码名片 |
| `group-service` | `:9095` | `GroupService`（新建） | 群资料、群成员、群角色、入群审批 |

Gateway 从当前只连接 `user:9090`，调整为同时连接四个服务地址。

---

## 二、核心设计决策

### 2.1 数据层采用方案 B：拆分成两张表

原先 `user_info` 同时承载“认证字段”和“资料字段”，拆分后改为：

| 新表 | 归属服务 | 主要字段 |
|---|---|---|
| `user_account` | `auth-service` | `user_uuid`、`email`、`telephone`、`password_hash`、`status`、`deleted_at`、`last_login_at` |
| `user_profile` | `user-service` | `user_uuid`、`nickname`、`avatar`、`gender`、`birthday`、`signature`、`qrcode_token` |

说明：
- `user_uuid` 作为跨服务统一主键。
- `user_account` 与 `user_profile` 逻辑上为 1:1 关系。
- 不再保留 `user_info` 作为兼容表，不做向前兼容。
- 注册流程中由 `auth-service` 完成 `user_account` 创建，并同步/异步调用 `user-service` 创建默认 `user_profile`。

### 2.2 一次性全量正名

本次拆分不做向前兼容，统一进行：

1. **proto package 重命名**
2. **go_package 重定向**
3. **表名统一**
4. **Gateway / Connect / 其他服务 import 全量更新**

### 2.3 表名统一策略

本次统一使用复数命名：

| 旧表名 | 新表名 |
|---|---|
| `user_info` | `user_account` / `user_profile` |
| `user_relation` | `user_relations` |
| `apply_request` | `apply_requests` |
| `device_session` | `device_sessions` |
| `group_info` | `groups` |
| `group_member` | `group_members` |

不保留旧表名兼容逻辑。

### 2.4 账号生命周期归属调整

以下接口归属 `auth-service`，不再保留在 `user-service`：

- `ChangePassword`
- `ChangeEmail`
- `ChangeTelephone`
- `DeleteAccount`

原因：
- 这些操作本质属于账号安全或账号生命周期，不属于资料域。
- 它们都会影响认证态、设备会话、可登录性或跨服务数据清理。

### 2.5 跨服务查询必须提供最小必要视图接口

不允许直接复用大而全的资料接口完成内部组装，内部查询统一收敛为专用接口，例如：

- `GetLoginProfile`
- `BatchGetUserCard`
- `BatchGetPublicProfile`
- `GetProfileStatus`

避免服务之间通过通用 `GetProfile` / `GetOtherProfile` 形成耦合蔓延。

---

## 三、各服务职责与承接内容

## 3.1 auth-service（认证 + 设备 + 账号生命周期）

**承接**：`AuthService` + `DeviceService` + `AccountService`（新建）

### Repository
- `IAuthRepository`：注册、登录、验证码、Token、密码、邮箱/手机绑定
- `IDeviceRepository`：设备会话、在线状态、活跃时间、踢设备、设备级 Token 管理
- 可选新增 `IAccountLifecycleRepository`：注销、删除标记、生命周期审计

### 数据归属
- `user_account`
- `device_sessions`
- Redis 中的验证码、access token、refresh token、设备活跃状态、在线状态

### 对外暴露 gRPC

```text
AuthService:
  Register / Login / LoginByCode / Logout /
  RefreshToken / SendVerifyCode / VerifyCode / ResetPassword

DeviceService:
  GetDeviceList / KickDevice / GetOnlineStatus /
  BatchGetOnlineStatus / UpdateDeviceActive / UpdateDeviceStatus

AccountService:
  ChangePassword / ChangeEmail / ChangeTelephone / DeleteAccount
```

### 职责边界
负责：
- 账号创建与认证
- 登录态签发与刷新
- 设备会话与在线状态
- 账号安全相关操作
- 账号注销编排

不负责：
- 用户公开资料展示
- 好友与黑名单逻辑
- 群逻辑

---

## 3.2 relation-service（好友 + 申请 + 黑名单）

**承接**：`FriendService` + `BlacklistService`

### Repository
- `IFriendRepository`：好友关系、备注、标签、增量同步
- `IApplyRepository`：好友申请、已读状态、未读数
- `IBlacklistRepository`：拉黑、取消拉黑、黑名单查询

### 数据归属
- `user_relations`
- `apply_requests`
- Redis 中的关系缓存、申请列表缓存、未读数、标签缓存

### 对外暴露 gRPC

```text
FriendService:
  SendFriendApply / GetFriendApplyList / GetSentApplyList /
  HandleFriendApply / GetUnreadApplyCount / MarkApplyAsRead /
  GetFriendList / SyncFriendList / DeleteFriend /
  SetFriendRemark / SetFriendTag / GetTagList /
  CheckIsFriend / BatchCheckIsFriend / GetRelationStatus

BlacklistService:
  AddBlacklist / RemoveBlacklist / GetBlacklistList / CheckIsBlacklist
```

### 职责边界
负责：
- 用户与用户之间的关系
- 好友申请与审批
- 标签、备注、黑名单
- 关系状态判断

不负责：
- 用户资料主数据
- 账号状态
- 群成员关系

说明：
- “用户与用户”的关系归 `relation-service`
- “用户与群”的关系归 `group-service`

---

## 3.3 user-service（用户资料，语义上等同 profile-service）

**承接**：`UserService`

### Repository
- `IUserRepository`：资料读写、搜索、二维码名片、资料缓存

### 数据归属
- `user_profile`
- Redis 中的资料缓存、二维码 token 映射、用户卡片缓存

### 对外暴露 gRPC（面向 Gateway）

```text
UserService:
  GetProfile / GetOtherProfile / SearchUser / UpdateProfile /
  UploadAvatar / GetQRCode / ParseQRCode / BatchGetProfile
```

### 内部接口（面向其他服务，单独定义为 InternalProfileService）

```text
InternalProfileService:
  CreateProfile         ← auth-service 注册成功后调用，初始化默认 user_profile
  MarkProfileDeleted    ← auth-service 注销账号时调用，标记资料为已注销态
  GetLoginProfile       ← auth-service 登录后调用，返回最小必要登录展示字段
  BatchGetUserCard      ← relation/group-service 批量获取头像+昵称用户卡片
  BatchGetPublicProfile ← relation-service 校验账号是否可见/已注销
```

> `InternalProfileService` 与 `UserService` 定义在同一个 `user-service` 进程中，但 proto 文件单独拆开（如 `internal_profile_service.proto`），语义上与对外接口明确区分。

### 职责边界
负责：
- 自己资料
- 他人公开资料
- 搜索用户
- 头像、昵称、签名、生日、性别
- 二维码名片
- 批量资料查询

不负责：
- 密码、邮箱、手机号绑定
- 设备会话
- 注销账号
- 好友关系
- 群关系

---

## 3.4 group-service（群组）

**现状**：当前仓库无完整群组实现，本服务为新增服务。

### 需要新建 proto
- `proto/group/group_service.proto`

### 数据归属
- `groups`
- `group_members`
- 后续可扩展：`group_announcements`、`group_join_requests`、`group_role_logs`

### 对外暴露 gRPC

```text
GroupService:
  CreateGroup / DismissGroup
  GetGroupInfo / UpdateGroupInfo
  AddMember / RemoveMember / GetMemberList
  GetGroupList
  UpdateMemberRole / TransferOwner
  ApplyJoinGroup / ApproveJoinGroup / RejectJoinGroup（建议预留）
  GetGroupMemberIds（供 msg / connect 内部调用，建议预留）
```

### 职责边界
负责：
- 群主数据
- 群成员与角色
- 群管理规则
- 入群审批

不负责：
- 用户资料主数据
- 用户与用户关系
- 消息落库与消息路由

---

## 四、当前业务逻辑下的跨服务调用清单

本节基于当前仓库已经存在的业务能力，以及拆分后的目标边界，明确列出同步 RPC 调用关系。

## 4.1 Gateway 的调用关系

### 登录 / 注册 / 刷新 Token / 登出
- Gateway → `auth-service`

### 用户资料相关
- Gateway → `user-service`
  - `GetProfile`
  - `GetOtherProfile`
  - `SearchUser`
  - `UpdateProfile`
  - `UploadAvatar`
  - `GetQRCode`
  - `ParseQRCode`

### 账号安全相关
- Gateway → `auth-service`
  - `ChangePassword`
  - `ChangeEmail`
  - `ChangeTelephone`
  - `DeleteAccount`

### 好友 / 黑名单相关
- Gateway → `relation-service`
  - 发送好友申请
  - 获取申请列表
  - 处理申请
  - 获取好友列表
  - 设置备注/标签
  - 获取标签列表
  - 获取黑名单列表
  - 判断是否好友
  - 获取关系状态

### 设备相关
- Gateway → `auth-service`
  - `GetDeviceList`
  - `KickDevice`
  - `GetOnlineStatus`
  - `BatchGetOnlineStatus`

### 群组相关
- Gateway → `group-service`
  - 建群、查群、改群、成员操作

---

## 4.2 auth-service 的跨服务调用

### 场景 1：注册成功后初始化资料
- `auth-service` → `user-service.InternalProfileService.CreateProfile`

**说明**：
- `Register` 只负责创建 `user_account`
- 成功后同步调 `CreateProfile`，初始化默认 `user_profile`（nickname 默认为邮箱前缀）
- `CreateProfile` 失败时回滚 `user_account`，保证两者原子性

### 场景 2：登录成功后组装返回资料
- `auth-service` → `user-service.InternalProfileService.GetLoginProfile`

**用途**：
- 返回 `LoginResponse.user_info`
- 只拿展示所需最小字段：`user_uuid`、`nickname`、`avatar`

**不采用**：
- 直接调 `UserService.GetProfile`（字段过多，语义不准确）
- 直查 `user_profile` 表（跨服务越权访问数据）

### 场景 3：修改邮箱 / 手机前校验资料态或展示信息（可选）
- 一般不需要同步调用 `user-service`
- 若需要回显用户卡片，可调 `GetLoginProfile`

### 场景 4：注销账号
- `auth-service.DeleteAccount` 作为编排入口：
  1. 标记 `user_account` 为已注销/不可登录（本服务同步）
  2. 删除全部设备会话、Token、在线态（本服务同步）
  3. `auth-service` → `user-service.InternalProfileService.MarkProfileDeleted`
  4. `auth-service` → `relation-service.InternalRelationService.HandleAccountDeleted`
  5. `auth-service` → `group-service.InternalGroupService.HandleAccountDeleted`

**建议**：
- 第 1、2 步严格同步
- 第 3～5 步第一阶段同步调用；后续演进为事件驱动 + 补偿

### 场景 5：Connect 心跳 / 上下线同步
- `connect-service` → `auth-service.UpdateDeviceActive`
- `connect-service` → `auth-service.UpdateDeviceStatus`

---

## 4.3 relation-service 的跨服务调用

### 场景 1：好友申请列表富化申请人资料
- `relation-service` → `user-service.BatchGetUserCard`

### 场景 2：好友列表富化资料
- `relation-service` → `user-service.BatchGetUserCard`

### 场景 3：黑名单列表富化资料
- `relation-service` → `user-service.BatchGetUserCard`

### 场景 4：发送好友申请前检查对方账号是否可见/已注销
- `relation-service` → `user-service.InternalProfileService.BatchGetPublicProfile`

**说明**：
- 不调用 `UserService.GetOtherProfile`（对外接口，字段过多，不含注销态）
- `BatchGetPublicProfile` 只返回：`user_uuid`、是否存在、是否已注销

### 场景 5：删除账号后的关系清理
- `auth-service` → `relation-service.InternalRelationService.HandleAccountDeleted`

**处理内容**：
- 好友关系降级为无效
- 黑名单关系逻辑删除
- 申请记录归档或隐藏

---

## 4.4 group-service 的跨服务调用

### 场景 1：展示群成员资料
- `group-service` → `user-service.BatchGetUserCard`

### 场景 2：加群策略需要校验好友关系（可选）
- `group-service` → `relation-service.CheckIsFriend`

### 场景 3：群主注销、成员注销
- `auth-service` → `group-service.HandleAccountDeleted`（建议新增内部接口或事件消费）

**处理内容建议**：
- 普通成员：自动移出群
- 群主：转让群主或解散群，按产品规则定

### 场景 4：消息服务获取群成员列表
- `msg-service` → `group-service.GetGroupMemberIds`
- `connect-service` 在需要群扩散时也可通过 `msg-service` 间接依赖，避免过多横向调用

---

## 4.5 user-service 的跨服务调用

正常情况下 `user-service` 应尽量少主动依赖其他服务。

### 当前建议
- `user-service` 尽量只做主数据服务
- 不主动调用 `auth-service`、`relation-service`、`group-service`
- 如未来必须做资料聚合，优先放在 Gateway/BFF，而不是回流到 `user-service`

---

## 五、跨服务依赖图（同步 RPC 视角）

```text
Gateway
  ├── auth-service
  ├── user-service
  ├── relation-service
  └── group-service

connect-service
  └── auth-service

msg-service
  └── group-service

auth-service
  ├── user-service
  ├── relation-service（账号注销后处理）
  └── group-service（账号注销后处理）

relation-service
  └── user-service

group-service
  ├── user-service
  └── relation-service（可选策略校验）
```

原则：
- `user-service` 作为资料主数据服务，尽量只被调用，不主动调用别人。
- `auth-service` 允许作为账号生命周期编排者。
- `relation-service` 与 `group-service` 都只依赖最小资料接口。
- `msg-service` 如需群成员列表，只依赖 `group-service`，不直接依赖 `relation-service` 或 `user-service`。

---

## 六、现有代码拆分映射

## 6.1 目录结构变化

```text
apps/
├── auth/
│   ├── cmd/
│   └── internal/
│       ├── handler/         auth_handler.go + device_handler.go + account_handler.go
│       ├── service/         auth_service.go + device_service.go + account_service.go
│       └── repository/      auth_repository.go + device_repository.go
│
├── relation/
│   └── internal/
│       ├── handler/         friend_handler.go + blacklist_handler.go
│       ├── service/         friend_service.go + blacklist_service.go
│       └── repository/      friend_repository.go + blacklist_repository.go + apply_repository.go
│
├── user/
│   └── internal/
│       ├── handler/         user_handler.go
│       ├── service/         user_service.go
│       └── repository/      user_repository.go
│
└── group/
    └── internal/
        ├── handler/         group_handler.go
        ├── service/         group_service.go
        └── repository/      group_repository.go
```

## 6.2 现有接口迁移映射

| 现有接口 | 新归属服务 |
|---|---|
| `Register` | `auth-service` |
| `Login` / `LoginByCode` / `RefreshToken` / `Logout` / `ResetPassword` | `auth-service` |
| `GetDeviceList` / `KickDevice` / `GetOnlineStatus` / `BatchGetOnlineStatus` / `UpdateDeviceActive` / `UpdateDeviceStatus` | `auth-service` |
| `ChangePassword` / `ChangeEmail` / `ChangeTelephone` / `DeleteAccount` | `auth-service` |
| `GetProfile` / `GetOtherProfile` / `SearchUser` / `UpdateProfile` / `UploadAvatar` / `GetQRCode` / `ParseQRCode` / `BatchGetProfile` | `user-service` |
| `SendFriendApply` / `GetFriendApplyList` / `HandleFriendApply` / `GetFriendList` / `DeleteFriend` / `SetFriendRemark` / `SetFriendTag` / `GetTagList` / `CheckIsFriend` / `BatchCheckIsFriend` / `GetRelationStatus` | `relation-service` |
| `AddBlacklist` / `RemoveBlacklist` / `GetBlacklistList` / `CheckIsBlacklist` | `relation-service` |
| 所有群接口 | `group-service` |

---

## 七、proto 拆分与一次性全量正名

当前所有 proto 均在 `package user`。本次一次性改为：

| 新服务 | proto package | go_package |
|---|---|---|
| `auth-service` | `package auth` | `github.com/013677890/LCchat-Backend/apps/auth/pb` |
| `relation-service` | `package relation` | `github.com/013677890/LCchat-Backend/apps/relation/pb` |
| `user-service` | `package user` | `github.com/013677890/LCchat-Backend/apps/user/pb` |
| `group-service` | `package group` | `github.com/013677890/LCchat-Backend/apps/group/pb` |

附加说明：
- `AccountService` 定义在 `auth` proto package 中（与 `AuthService`、`DeviceService` 同文件或同目录）。
- `InternalProfileService` 定义在 `user` proto package 中，单独文件 `internal_profile_service.proto`，语义上区分对外接口。
- `relation-service` 和 `group-service` 同理，如有内部接口，单独建 `internal_xxx_service.proto`。
- Gateway、Connect、Msg 中所有 pb import 全量同步更新。
- 不保留旧 proto package 的兼容层。

---

## 八、主要困难与解决方案

## 8.1 困难：注册流程要跨 `auth-service` 与 `user-service`

### 方案
- 同步 RPC：`auth-service.Register` 成功创建 `user_account` 后，立即调用 `user-service.CreateProfile`
- 如第二步失败：
  - 方案一：回滚 `user_account`
  - 方案二：记录补偿任务重试

### 第一阶段建议
- 先采用同步 RPC + 失败回滚，保证行为简单清晰。

---

## 8.2 困难：登录需要返回昵称/头像

### 方案
- `auth-service` 调 `user-service.GetLoginProfile`
- `GetLoginProfile` 只返回登录所需展示字段

### 不采用
- 不直查 `user_profile` 表
- 不调用通用 `GetProfile`

---

## 8.3 困难：账号注销会影响多个服务

### 方案
`auth-service.DeleteAccount` 作为统一入口，执行编排：
1. 标记 `user_account` 删除态
2. 清理全部设备会话与 Token
3. 调 `user-service` 处理资料侧删除态
4. 调 `relation-service` 处理关系降级
5. 调 `group-service` 处理群成员身份

### 后续演进
- 第一阶段同步 RPC
- 第二阶段事件驱动 + 补偿机制

---

## 8.4 困难：Gateway 需要同时维护 4 个 gRPC Client

### 方案
新增环境变量：
- `AUTH_SERVICE_ADDR`
- `RELATION_SERVICE_ADDR`
- `USER_SERVICE_ADDR`
- `GROUP_SERVICE_ADDR`

并在 `apps/gateway/cmd/providers.go` 中分别提供 client provider。

---

## 8.5 困难：Wire、部署、监控全部要重建

每个新服务都需要独立：
- `cmd/providers.go`
- `cmd/wire.go`
- `cmd/app.go`
- `wire_gen.go`
- metrics 暴露
- health check
- docker-compose service 定义

---

## 九、建议拆分顺序

| 阶段 | 内容 | 理由 |
|---|---|---|
| **第 1 步** | 新建 `group-service` | 全新域，边界最清晰，对现有逻辑影响最小 |
| **第 2 步** | 拆出 `relation-service` | 现有 friend / apply / blacklist 代码边界清楚 |
| **第 3 步** | 将 `user_info` 拆为 `user_account` + `user_profile` | 为 `auth-service` 与 `user-service` 真正分离打基础 |
| **第 4 步** | 拆出 `auth-service`，同时迁移 `DeviceService` 与新增 `AccountService` | 依赖最多，放在数据边界理顺之后 |
| **第 5 步** | Gateway / Connect / Msg 全量切换到新 proto 与新地址 | 一次性完成全量正名与运行时切换 |
| **第 6 步** | 清理旧 `apps/user` 冗余代码与旧表、旧 proto 引用 | 不保留兼容层，收口彻底 |

---

## 十、最终边界原则

1. **一个服务只拥有自己表的写权限。**
2. **跨服务查询只调用最小必要视图接口。**
3. **账号安全与生命周期统一归 `auth-service`。**
4. **用户资料主数据统一归 `user-service`。**
5. **用户与用户的关系归 `relation-service`。**
6. **用户与群的关系归 `group-service`。**
7. **第一次拆分就完成命名、表名、proto 的统一，不保留兼容层。**

---

*文档更新时间：2026-04-14*