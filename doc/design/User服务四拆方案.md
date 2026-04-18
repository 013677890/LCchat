# User 服务四拆方案

> 基于现有 `apps/user` 代码与当前业务边界，将用户域拆分为四个独立服务，并同步完成数据拆表、proto 全量正名、表名统一与跨服务调用梳理。

---

## 零、风险与决策摘要（必读）

本方案遵循以下关键决策（详见各章节）：

| 决策项 | 选择 | 原因 | 风险提示 |
|---|---|---|---|
| 发布策略 | **按阶段执行，每阶段内彻底替换，不保留兼容 shim** | 收口彻底、减少长期维护负担 | 🔴 单阶段失败只能靠 DB 备份回滚，建议每个阶段前做完整备份 + 演练 |
| SearchUser 跨表 | **Gateway 聚合**：邮箱先查 auth 拿 UUID，再批查 user 资料 | 数据权威清晰，auth 独占 email 写权限 | 🟡 搜索链路多一跳 RPC，需要在 Gateway 做熔断 |
| 账号注销编排 | **auth 同步只做标删+清 token，其余走 Kafka `account.deleted` 异步消费 + 幂等补偿** | 避免超时传染、改善用户体验 | 🟡 需新增 Outbox/幂等表，最终一致需监控消费 lag |
| 数据库策略（Phase 1） | **共库 + 按表 DB 账号权限隔离** | 避免一步到位分库的复杂度 | 🟡 Phase 2 若分库需补齐 outbox/saga |
| Internal gRPC 隔离 | **拦截器校验 `x-internal-caller` metadata**，对外接口拒绝带此 header | 部署成本低、不新增监听端口 | 🟢 运维需确保 header 不会被外部可控代理注入 |
| 登录展示字段 | **`user_account` 冗余 `nickname`、`avatar` 到登录专用字段**；user 改资料时异步回写 auth | 登录热路径不依赖 user 服务可用性 | 🟡 写路径多一步异步回写，需幂等 + 失败可观测 |
| 拆分顺序 | **proto 拆包 → 拆表 → relation → auth+device → group → 清理**（见第九章） | 先解决现有痛点，降低单步风险 | 🟢 group 后置不影响其它域 |

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
| `auth-service` | `:9090` | `AuthService` + `DeviceService` + `AccountService`（新建） + `InternalAuthService`（新建，内部） | 登录、注册、验证码、Token、设备会话、账号安全、账号注销 |
| `relation-service` | `:9093` | `FriendService` + `BlacklistService`（+ 可选 Internal） | 好友申请、好友关系、标签、黑名单 |
| `user-service` | `:9094` | `UserService` + `InternalProfileService`（新建，内部） | 用户资料、公开资料、搜索、二维码名片 |
| `group-service` | `:9095` | `GroupService`（新建）（+ `InternalGroupService` 可选） | 群资料、群成员、群角色、入群审批 |

Gateway 当前连接 `user:9090` + `msg:9092`，拆分后改为连接 `auth:9090` + `user:9094` + `relation:9093` + `group:9095` + `msg:9092`（msg 保持不变）；详见第十二章端口规划表。

---

## 二、核心设计决策

### 2.1 数据层采用方案 B：拆分成两张表

原先 `user_info` 同时承载"认证字段"和"资料字段"，拆分后改为：

| 新表 | 归属服务 | 主要字段 |
|---|---|---|
| `user_account` | `auth-service` | `user_uuid`、`email`、`telephone`、`password_hash`、`status`、`deleted_at`、`last_login_at`、**`login_nickname`**、**`login_avatar`** |
| `user_profile` | `user-service` | `user_uuid`、`nickname`、`avatar`、`gender`、`birthday`、`signature`、`qrcode_token` |

说明：
- `user_uuid` 作为跨服务统一主键。
- `user_account` 与 `user_profile` 逻辑上为 1:1 关系。
- 不再保留 `user_info` 作为兼容表，不做向前兼容。
- 注册流程中由 `auth-service` 完成 `user_account` 创建，并通过 **Outbox** 驱动 user-service 创建默认 `user_profile`（见 8.1）。
- **`login_nickname` / `login_avatar`** 是冗余字段，专门服务于登录 / Token 刷新场景的最小展示需求，避免登录热路径强依赖 user-service；权威仍在 `user_profile`，user-service 改资料时通过 `InternalAuthService.UpdateLoginDisplay` 异步回写（见 2.6、4.5）。

### 2.2 按阶段彻底替换（不保留兼容 shim）

本次拆分分阶段进行（见第九章），**但每个阶段内**执行：

1. **proto package 重命名**
2. **go_package 重定向**
3. **表名统一**
4. **Gateway / Connect / 其他服务 import 全量更新**

**不保留旧 package 的 alias、不保留旧表的 view**，收口彻底。

> 🔴 **风险提示**：此决策意味着阶段内一旦部署失败，回退只能依赖：
> - 代码：回滚到阶段起点的 Git tag；
> - 数据：阶段开始前的 DB 全量备份。
> 每个阶段必须满足：① 上线前有备份 + 演练 ② 有灰度环境（staging）预演过 ③ 有回滚 runbook。

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

- `BatchGetUserCard`
- `BatchGetPublicProfile`
- `GetProfileStatus`
- `FindAccountByEmail` / `FindAccountByTelephone`（`InternalAuthService`）

避免服务之间通过通用 `GetProfile` / `GetOtherProfile` 形成耦合蔓延。

> 备注：原方案中的 `GetLoginProfile` 已被 2.6（`user_account` 冗余登录展示字段）取代，登录链路不再跨服务拉资料。

### 2.6 登录展示字段冗余到 `user_account`

- 登录、刷新 Token 等热路径只返回 `user_uuid`、`nickname`、`avatar`，且仅这三项。
- 这三项在 `user_account` 上冗余为 `login_nickname` / `login_avatar`（`user_uuid` 本身就是主键）。
- 权威数据仍在 `user_profile.nickname` / `user_profile.avatar`。
- user-service `UpdateProfile` / `UploadAvatar` 成功后，通过 `InternalAuthService.UpdateLoginDisplay(user_uuid, nickname, avatar)` **异步回写** auth；该调用必须：
  - 幂等（以 user_uuid + 最后更新时间戳做乐观锁）；
  - 失败走 user-service 侧本地 Outbox 重试；
  - 失败期间用户再次登录拿到的是"上次同步的旧展示"，不影响主流程。
- 当前仓库使用的字段（如资料详情展示、他人资料页）完全不受影响，仍走 `user_profile`。

### 2.7 数据库策略：Phase 1 共库 + 按表权限隔离

- **Phase 1**（本次拆分落地阶段）：所有服务共用一个 MySQL 实例 / 一个库，但：
  - 每个服务使用独立的 DB 账号；
  - 账号仅被 GRANT 对自己拥有的表的 DML / DDL 权限；
  - 跨表查询走 RPC，禁止 JOIN 其他服务的表；
  - 避免 `SELECT * FROM user_account JOIN user_profile` 这类写法。
- **Phase 2**（未来若单库成为瓶颈）：拆库。此时 Outbox / Saga / 对账任务必须完整上线后才能拆。
- **本文档第一阶段不讨论分库**。

### 2.8 内部 RPC 接口鉴权：metadata 拦截器

- 所有 `InternalXxxService` 与对外 service 注册在**同一个 gRPC Server** / 同一端口；
- 新增统一拦截器 `InternalCallerInterceptor`：
  - 对 `InternalXxxService` 全部方法，要求 metadata 中携带 `x-internal-caller: <service-name>` 且值在白名单内；
  - 对对外 service（如 `UserService`、`AuthService`）**拒绝**带 `x-internal-caller` 的调用，避免逻辑混淆；
- 白名单在 `pkg/grpcx/internal_caller.go` 中维护，按 service 粒度配置。
- 生产部署再辅以网络 ACL，保证外部入站流量不会带上 `x-internal-caller`。

### 2.9 跨服务异步解耦：账号注销走 Kafka 事件

- `auth-service.DeleteAccount` 同步阶段只做两件事：① 标记 `user_account.status=deleted` + `deleted_at` ② 清理所有该用户的设备会话 / access_token / refresh_token / 在线态；
- 同事务内写 `outbox_event(type=account_deleted, user_uuid, ...)`，由 outbox worker 投递到 Kafka topic `account.deleted`；
- `user-service` / `relation-service` / `group-service` 独立订阅 + **幂等消费** + 失败重试 + DLQ；
- 用户接口返回"已受理"，不等待下游完成。
- 详细失败模式见第十五章。

---

## 三、各服务职责与承接内容

## 3.1 auth-service（认证 + 设备 + 账号生命周期）

**承接**：`AuthService` + `DeviceService` + `AccountService`（新建）

### Repository
- `IAuthRepository`：注册、登录、验证码、Token、密码、邮箱/手机绑定、登录展示字段（冗余 nickname/avatar）
- `IDeviceRepository`：设备会话、在线状态、活跃时间、踢设备、设备级 Token 管理
- 可选新增 `IAccountLifecycleRepository`：注销、删除标记、生命周期审计、Outbox 事件入表

### 数据归属
- `user_account`（含 `login_nickname`、`login_avatar` 冗余字段，见 2.6）
- `device_sessions`
- `outbox_events`（至少 `account.deleted`、`user.created` 两类）
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

### 内部接口（面向其他服务，定义为 `InternalAuthService`，单独 proto 文件）

```text
InternalAuthService:
  FindAccountByEmail      ← Gateway 聚合 SearchUser 时使用
  FindAccountByTelephone  ← 手机号找账号（后续保留扩展）
  UpdateLoginDisplay      ← user-service 异步回写登录冗余字段（nickname/avatar）
  BatchCheckAccountStatus ← 批量检查账号是否可见/已注销（供 relation、group）
```

- 所有 `InternalAuthService` 方法必须通过 2.8 的 `x-internal-caller` 拦截器鉴权；
- 定义在 `auth` proto package 中，单独文件 `internal_auth_service.proto`。

### 职责边界
负责：
- 账号创建与认证
- 登录态签发与刷新（直接从 `user_account` 拿登录展示字段，不依赖 user-service）
- 设备会话与在线状态
- 账号安全相关操作
- 账号注销编排（同步最小动作 + Outbox 异步通知）

不负责：
- 用户公开资料主数据
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

### 内部接口（面向其他服务，单独定义为 `InternalProfileService`）

```text
InternalProfileService:
  CreateProfile         ← user-service 侧本身实现；被 auth 的 Outbox worker 调用（幂等）
  BatchGetUserCard      ← relation / group 批量获取头像+昵称用户卡片
  BatchGetPublicProfile ← relation 校验账号是否可见/已注销（对齐 InternalAuthService.BatchCheckAccountStatus，组合使用）
```

**移除**：
- `GetLoginProfile`：登录冗余字段已迁入 `user_account`（见 2.6），auth 无需再跨服务拉 profile。
- `MarkProfileDeleted`：改为 user-service 订阅 Kafka `account.deleted` 事件处理（见 2.9）。

> `InternalProfileService` 与 `UserService` 定义在同一个 `user-service` 进程中，但 proto 文件单独拆开（如 `internal_profile_service.proto`），注册在同端口，通过 2.8 的拦截器做内部鉴权。

### user-service 的反向调用（新增）

- user-service `UpdateProfile` / `UploadAvatar` 成功后，**异步**调用 `auth-service.InternalAuthService.UpdateLoginDisplay`（见 4.5、2.6）；
- 通过 user-service 本地 Outbox 保证最终一致：资料更新事务内写 `outbox_event(type=profile_display_changed)`，worker 后台重试直至成功。

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
  - `UpdateProfile`
  - `UploadAvatar`
  - `GetQRCode`
  - `ParseQRCode`

### 搜索用户（需要 Gateway 聚合）
- Gateway 判断 keyword 是否为邮箱：
  - **是邮箱**：`auth-service.InternalAuthService.FindAccountByEmail(email)` → 拿到 `user_uuid`（或空）→ `user-service.BatchGetProfile([uuid])`
  - **非邮箱**：`user-service.SearchUser(keyword)` 按 nickname / uuid 前缀匹配
- Gateway 的聚合逻辑统一封装在 `apps/gateway/internal/service/search_service.go`（新增），避免 handler 层散落该规则

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

### 场景 1：注册成功后初始化资料（Outbox 异步）
- **同步阶段**（auth 在本地事务内完成）：
  1. `INSERT user_account`（含 `login_nickname`=邮箱前缀、`login_avatar`=系统默认头像）
  2. `INSERT outbox_events(type=user_created, user_uuid, default_nickname, default_avatar)`
- **异步阶段**：auth 的 Outbox worker 读事件 → 调用 `user-service.InternalProfileService.CreateProfile(user_uuid, nickname, avatar)`
- user-service 侧 `CreateProfile` 必须幂等（UPSERT by `user_uuid`），重复投递安全
- **注册接口返回**不等待 profile 创建完成；用户首次查看资料时若 profile 尚未到达，前端用 auth 的登录冗余字段兜底展示

### 场景 2：登录成功后组装返回资料（无跨服务调用）
- `auth-service.Login` 直接从 `user_account` 读 `login_nickname`、`login_avatar`，**不调用 user-service**
- 返回字段：`user_uuid`、`nickname`、`avatar`
- 客户端如需完整资料，登录成功后另发 `GET /user/profile`

**不采用**：
- 直接调 `UserService.GetProfile`（字段过多，语义不准确）
- 直查 `user_profile` 表（跨服务越权访问数据）
- 原方案 `GetLoginProfile`（登录热路径不应跨服务）

### 场景 3：Gateway 聚合搜索（SearchUser 时被调用）
- Gateway → `auth-service.InternalAuthService.FindAccountByEmail`
- 返回：`user_uuid` + `status`（是否已注销）；找不到返回空

### 场景 4：修改邮箱 / 手机
- 本地修改 `user_account.email` / `telephone`，不需要调用 `user-service`

### 场景 5：注销账号（同步最小动作 + Kafka 事件）
- **同步**（一个 DB 事务内）：
  1. 标记 `user_account.status=deleted` + `deleted_at=NOW()`
  2. 删除所有设备会话 / Token / Redis 在线态（Redis 删除放在事务提交后）
  3. `INSERT outbox_events(type=account_deleted, user_uuid)`
- **异步**（Outbox worker 投递至 Kafka topic `account.deleted`）：
  - `user-service` 订阅 → 标记资料为注销态、失效 QR token
  - `relation-service` 订阅 → 好友关系降级、黑名单清理、申请归档
  - `group-service` 订阅 → 普通成员自动移出、群主按产品规则转让或解散
- **接口语义**：`DeleteAccount` 返回"已受理"；同步完成后用户立即无法登录；下游清理按最终一致保证
- 失败模式见第十五章

### 场景 6：Connect / Gateway 的设备活跃与状态同步
- `connect-service` → `auth-service.DeviceService.UpdateDeviceActive`
- `connect-service` → `auth-service.DeviceService.UpdateDeviceStatus`
- `gateway` → `auth-service.DeviceService.UpdateDeviceActive`（批量聚合，见 `apps/gateway/cmd/app.go` 的 `InitDeviceActiveSyncer`）
- auth-service 内要求：DeviceService handler 与 AuthService handler 在 gRPC 拦截器链/连接池层面相对隔离，避免心跳洪峰打爆登录链路（见 8.6）

---

## 4.3 relation-service 的跨服务调用

### 场景 1：好友申请列表富化申请人资料
- `relation-service` → `user-service.BatchGetUserCard`

### 场景 2：好友列表富化资料
- `relation-service` → `user-service.BatchGetUserCard`

### 场景 3：黑名单列表富化资料
- `relation-service` → `user-service.BatchGetUserCard`

### 场景 4：发送好友申请前检查对方账号是否可见/已注销
- `relation-service` → `auth-service.InternalAuthService.BatchCheckAccountStatus`（取账号可见性/注销态）
- `relation-service` → `user-service.InternalProfileService.BatchGetPublicProfile`（取昵称/头像用于 UI 展示）

**说明**：
- 不调用 `UserService.GetOtherProfile`（对外接口，字段过多，不含注销态）
- "是否已注销"由 auth 权威（账号态），"昵称/头像"由 user 权威
- 两者组合使用即可，避免单一接口承担"既 profile 又 account"混合语义

### 场景 5：删除账号后的关系清理（订阅 Kafka）
- `relation-service` 订阅 `account.deleted` Kafka topic，独立消费
- **不再提供** `InternalRelationService.HandleAccountDeleted` RPC（避免 auth 成为同步编排者）

**处理内容**：
- 好友关系降级为无效
- 黑名单关系逻辑删除
- 申请记录归档或隐藏
- 消费者必须幂等：按 `user_uuid` 幂等表去重

---

## 4.4 group-service 的跨服务调用

### 场景 1：展示群成员资料
- `group-service` → `user-service.BatchGetUserCard`

### 场景 2：加群策略需要校验好友关系（可选）
- `group-service` → `relation-service.CheckIsFriend`

### 场景 3：群主注销、成员注销（订阅 Kafka）
- `group-service` 订阅 `account.deleted` Kafka topic，独立消费
- **不再提供** `InternalGroupService.HandleAccountDeleted` RPC

**处理内容**：
- 普通成员：自动移出群
- 群主：按产品规则转让群主或解散群
- 必须幂等，按 `user_uuid` 幂等表去重

### 场景 4：消息服务获取群成员列表
- `Push-Job`（见第十三章归属）→ `group-service.GetGroupMemberIds`
- `connect-service` **不**直接调用 `group-service`，群扩散语义统一由 Push-Job 承担

---

## 4.5 user-service 的跨服务调用

### 场景 1：资料更新后回写 auth 的登录冗余字段（唯一主动跨服务调用）
- user-service `UpdateProfile` / `UploadAvatar` 成功后：
  1. 在 user-service 本地事务内 INSERT `outbox_events(type=profile_display_changed, user_uuid, nickname, avatar)`
  2. user-service 的 Outbox worker 调用 `auth-service.InternalAuthService.UpdateLoginDisplay(user_uuid, nickname, avatar)`
- 必须幂等（auth 侧以 `user_uuid` + `updated_at` 做乐观锁，旧版本丢弃）
- 回写失败期间用户再登录拿到的是"上次同步的展示字段"，不阻断主流程

### 其他原则
- `user-service` 尽量只做主数据服务
- 不主动调用 `relation-service`、`group-service`
- 资料聚合优先放在 Gateway / BFF 层，不回流到 `user-service`

---

## 五、跨服务依赖图

### 5.1 同步 RPC 视角

```text
Gateway
  ├── auth-service              (登录/注册/注销/设备/InternalAuth.FindAccountByEmail)
  ├── user-service              (资料/搜索)
  ├── relation-service          (好友/黑名单)
  └── group-service             (群)

connect-service
  └── auth-service              (DeviceService 心跳/状态/WS 鉴权兜底)

Push-Job
  ├── group-service             (GetGroupMemberIds 群扩散)
  └── connect-service           (PushToDevice / PushToUser)

auth-service Outbox worker
  └── user-service              (InternalProfileService.CreateProfile 异步注册补偿)

user-service Outbox worker
  └── auth-service              (InternalAuthService.UpdateLoginDisplay 异步回写登录冗余)

relation-service
  ├── auth-service              (InternalAuth.BatchCheckAccountStatus 账号态)
  └── user-service              (InternalProfile.BatchGetUserCard 昵称/头像)

group-service
  ├── user-service              (InternalProfile.BatchGetUserCard)
  └── relation-service          (可选，加群策略校验 CheckIsFriend)
```

### 5.2 异步事件视角（Kafka）

```text
auth-service ──► Kafka topic: account.deleted ──► user-service (消费)
                                             └──► relation-service (消费)
                                             └──► group-service (消费)

msg-service ──► Kafka topic: msg.push ──► Push-Job (消费，现有拓扑不变)
```

### 5.3 原则

- **`user-service` 作为资料主数据服务**：只被调用 + 异步回写登录冗余字段（唯一主动出站）。
- **`auth-service` 作为账号生命周期源头**：同步只做标删 + 清 token；其余用 Kafka `account.deleted` 扩散。
- **`relation-service` 与 `group-service`** 依赖最小视图：账号态（auth）+ 展示字段（user）组合使用。
- **Push-Job** 是群扩散的唯一承载者；`connect-service` 保持纯管道，不调用除 auth 以外的服务。
- **Outbox 模式** 是唯一允许的跨服务最终一致机制：事务内写事件，worker 重试。

---

## 六、现有代码拆分映射

## 6.1 目录结构变化

```text
apps/
├── auth/
│   ├── cmd/                 main.go + app.go + providers.go + wire.go + wire_gen.go
│   └── internal/
│       ├── handler/         auth_handler.go + device_handler.go + account_handler.go + internal_auth_handler.go
│       ├── service/         auth_service.go + device_service.go + account_service.go
│       ├── repository/      auth_repository.go + device_repository.go + outbox_repository.go
│       └── outbox/          worker.go（消费 outbox_events 投 Kafka 或调 user.CreateProfile）
│
├── relation/
│   ├── cmd/
│   └── internal/
│       ├── handler/         friend_handler.go + blacklist_handler.go（+ internal_relation_handler.go 可选）
│       ├── service/         friend_service.go + blacklist_service.go
│       ├── repository/      friend_repository.go + blacklist_repository.go + apply_repository.go
│       └── consumer/        account_deleted_consumer.go（订阅 account.deleted）
│
├── user/
│   ├── cmd/
│   └── internal/
│       ├── handler/         user_handler.go + internal_profile_handler.go
│       ├── service/         user_service.go
│       ├── repository/      user_repository.go + outbox_repository.go
│       ├── outbox/          worker.go（调 auth.UpdateLoginDisplay）
│       └── consumer/        account_deleted_consumer.go
│
└── group/
    ├── cmd/
    └── internal/
        ├── handler/         group_handler.go（+ internal_group_handler.go 可选）
        ├── service/         group_service.go
        ├── repository/      group_repository.go + member_repository.go
        └── consumer/        account_deleted_consumer.go
```

## 6.2 现有接口迁移映射

| 现有接口 | 新归属服务 | 备注 |
|---|---|---|
| `Register` | `auth-service` | 改为 Outbox 异步驱动 user.CreateProfile（见 8.1） |
| `Login` / `LoginByCode` / `RefreshToken` / `Logout` / `ResetPassword` | `auth-service` | 直接读 `user_account.login_nickname/login_avatar`，不跨服务 |
| `GetDeviceList` / `KickDevice` / `GetOnlineStatus` / `BatchGetOnlineStatus` / `UpdateDeviceActive` / `UpdateDeviceStatus` | `auth-service`（DeviceService） | handler 需与 AuthService 隔离（见 8.6） |
| `ChangePassword` / `ChangeEmail` / `ChangeTelephone` | `auth-service`（AccountService，新 proto） | 无跨服务调用 |
| `DeleteAccount` | `auth-service`（AccountService，新 proto） | 改为同步 + Outbox + Kafka（见 8.3） |
| `GetProfile` / `GetOtherProfile` / `UpdateProfile` / `UploadAvatar` / `GetQRCode` / `ParseQRCode` / `BatchGetProfile` | `user-service` | UpdateProfile / UploadAvatar 成功后异步回写 auth 冗余字段 |
| `SearchUser` | **Gateway 聚合**（见 4.1） | 邮箱场景先调 auth.FindAccountByEmail，再调 user.BatchGetProfile |
| `SendFriendApply` / `GetFriendApplyList` / `HandleFriendApply` / `GetFriendList` / `DeleteFriend` / `SetFriendRemark` / `SetFriendTag` / `GetTagList` / `CheckIsFriend` / `BatchCheckIsFriend` / `GetRelationStatus` | `relation-service` | — |
| `AddBlacklist` / `RemoveBlacklist` / `GetBlacklistList` / `CheckIsBlacklist` | `relation-service` | — |
| 所有群接口 | `group-service` | 新建域 |

---

## 七、proto 拆分与命名统一

当前所有 proto 均在 `package user`。拆分后改为：

| 新服务 | proto package | go_package |
|---|---|---|
| `auth-service` | `package auth` | `github.com/013677890/LCchat-Backend/apps/auth/pb` |
| `relation-service` | `package relation` | `github.com/013677890/LCchat-Backend/apps/relation/pb` |
| `user-service` | `package user` | `github.com/013677890/LCchat-Backend/apps/user/pb` |
| `group-service` | `package group` | `github.com/013677890/LCchat-Backend/apps/group/pb` |

附加说明：
- `AccountService` 定义在 `auth` proto package 中（与 `AuthService`、`DeviceService` 同文件或同目录）。
- `InternalProfileService` 定义在 `user` proto package 中，单独文件 `internal_profile_service.proto`。
- `InternalAuthService` 定义在 `auth` proto package 中，单独文件 `internal_auth_service.proto`（承载 `FindAccountByEmail`、`FindAccountByTelephone`、`UpdateLoginDisplay`、`BatchCheckAccountStatus`）。
- `relation-service` 和 `group-service` 同理，如有内部接口，单独建 `internal_xxx_service.proto`。
- Gateway、Connect、Msg 中所有 pb import 全量同步更新。
- 不保留旧 proto package 的兼容层。

### 7.1 Internal proto 的鉴权约定

- 所有 `InternalXxxService` 对应的 `.proto` 文件名统一以 `internal_` 前缀；
- 在 `pkg/grpcx/internal_caller.go` 新建 `InternalCallerInterceptor(whitelist map[string][]string)`：
  - `whitelist` 的 key 是 full method（如 `auth.InternalAuthService/FindAccountByEmail`），value 是允许的 caller 名称（如 `["gateway", "relation-service"]`）；
  - 对 whitelisted methods，要求 metadata 里有 `x-internal-caller` 且值在白名单内；
  - 对非 whitelisted methods（即对外接口），出现 `x-internal-caller` 直接拒绝，避免通过该 header 绕过业务鉴权；
- 各服务调用内部接口前统一通过 outgoing metadata 注入 `x-internal-caller: <my-service-name>`；
- 所有内部 proto 服务名建议统一以 `Internal` 开头，便于白名单正则匹配。

---

## 八、主要困难与解决方案

## 8.1 困难：注册流程要跨 `auth-service` 与 `user-service`

### 方案：Outbox 最终一致

- **同步阶段**：auth-service 在本地事务内 `INSERT user_account` + `INSERT outbox_events(type=user_created)`；
- **异步阶段**：auth Outbox worker 拉取事件，调用 `user-service.InternalProfileService.CreateProfile(user_uuid, default_nickname, default_avatar)`；
- `CreateProfile` 必须幂等：`ON DUPLICATE KEY UPDATE` / `INSERT ... WHERE NOT EXISTS`，重复消费安全；
- 失败无限重试 + 指数退避；超阈值后进入 DLQ 报警；
- **兜底展示**：用户注册后立即登录，若此刻 profile 尚未到达，auth 的 `login_nickname` / `login_avatar` 已经有默认值可以展示。

### 不采用
- 同步 RPC + 回滚 `user_account`：回滚无法撤销副作用（验证码已消费、已发 Token）；
- 同步 RPC + 忽略失败：会形成"账号存在但无资料"永久不一致。

详细失败模式见第十五章。

---

## 8.2 困难：登录需要返回昵称/头像

### 方案：`user_account` 冗余登录展示字段（见 2.6）

- auth-service 直接从本服务 DB 读 `login_nickname` / `login_avatar`，无跨服务调用；
- user-service 改资料时，通过自己的 Outbox 调 `InternalAuthService.UpdateLoginDisplay` 异步回写；
- 最终一致延迟通常在秒级，用户感知极小。

### 不采用
- 不调用 `user-service.GetLoginProfile`（登录热路径不跨服务）；
- 不直查 `user_profile` 表（跨服务越权）；
- 不在 auth 侧做 Redis 缓存替代冗余字段（字段少且稳定，冗余字段比缓存更可控）。

---

## 8.3 困难：账号注销会影响多个服务

### 方案：同步最小动作 + Kafka 事件异步补偿

**同步阶段**（auth 本地事务）：
1. 标记 `user_account.status=deleted`、`deleted_at=NOW()`
2. 清理所有 `device_sessions`
3. 清理所有 Redis Token / 在线态（事务提交后执行）
4. `INSERT outbox_events(type=account_deleted, user_uuid)`

**异步阶段**：
- auth Outbox worker → Kafka topic `account.deleted`
- `user-service` / `relation-service` / `group-service` 各自订阅并幂等消费；
- 每个服务维护本地 `account_deleted_idempotent(user_uuid, processed_at)` 表去重；
- 消费失败重试 + DLQ；

**接口语义**：`DeleteAccount` 返回 "已受理"（不等下游），用户立即无法登录，清理按最终一致保证。

---

## 8.4 困难：Gateway 需要同时维护 4 个 gRPC Client

### 方案

环境变量：
- `AUTH_SERVICE_ADDR`（默认 `:9090`）
- `USER_SERVICE_ADDR`（默认 `:9094`）
- `RELATION_SERVICE_ADDR`（默认 `:9093`）
- `GROUP_SERVICE_ADDR`（默认 `:9095`）

在 `apps/gateway/cmd/providers.go` 抽一个公共工厂：

```text
grpcClientFactory(serviceName string, addr string) (*grpc.ClientConn, Breaker, RetryPolicy)
```

工厂内部按服务画像配置：
- auth：超时短（100-300ms）、熔断阈值严
- user：超时中（200-500ms）、熔断阈值中
- relation / group：超时中（300-500ms）
- ServiceConfig 的 retry 必须针对具体 `FullMethod`（如 `auth.AuthService/*`），避免继承到其他服务

---

## 8.5 困难：Wire、部署、监控全部要重建

每个新服务都需要独立：
- `cmd/providers.go`
- `cmd/wire.go` / `wire_gen.go`
- `cmd/app.go`
- metrics 暴露（端口见第十二章）
- health check
- docker-compose service 定义

降低重复工作：
- 抽取共用 provider 到 `pkg/grpcx/bootstrap.go`（gRPC Server 组装、拦截器链、metrics）；
- 本地开发支持 "Monolith Mode"（详见第十四章）。

---

## 8.6 困难：DeviceService 迁入 auth-service 后 QPS 陡增

### 背景
- 现有 `apps/gateway/cmd/app.go:178-205` 中 `InitDeviceActiveSyncer` 以每设备心跳频率 × 用户量的 QPS 批量调 `DeviceService.UpdateDeviceActive`；
- `connect-service` 每个 WS 连接每次心跳也会触发 `UpdateDeviceActive` / `UpdateDeviceStatus`；
- 这些 QPS 可能是登录 QPS 的 100 倍以上。

### 方案
- auth-service 内 DeviceService handler 与 AuthService handler **物理上同进程，逻辑上隔离**：
  - 使用独立的 handler goroutine pool；
  - gRPC Server 可以注册两个 `*grpc.Server` 实例，监听同端口不同 service（或两端口）；
  - 监控按 `/auth.AuthService/*` vs `/auth.DeviceService/*` 分维度告警；
- 若压测后 DeviceService 成为瓶颈，演进为独立进程（与 auth-service 共库），对外仍保持 `auth` proto package。

---

## 九、建议拆分顺序（评审后版本）

| 阶段 | 内容 | 理由 | 上线前置 |
|---|---|---|---|
| **第 1 阶段** | 新 proto package（`auth` / `relation` / `user` / `group`）生成 + 同进程内注册 4 个 gRPC service 到当前 `apps/user` 进程 | 只做代码结构调整，对外仍是单进程、单端口，零运行时风险 | proto/pb 全部产出、单测全通过 |
| **第 2 阶段** | 拆表：`user_info` → `user_account` + `user_profile`；引入 `login_nickname` / `login_avatar` 冗余字段 | 数据边界先于进程边界；同进程下双 repository 可独立回退 | 双写演练、差异对账、备份 |
| **第 3 阶段** | 拆出 `relation-service`（friend + blacklist + apply） | 业务最独立、跨域最少、风险最低；第一次真正意义上的进程拆分，积累经验 | gateway 新增 `RELATION_SERVICE_ADDR`、relation 独立 Wire / metrics / healthcheck |
| **第 4 阶段** | 拆出 `auth-service`，同时迁移 `DeviceService` 与新增 `AccountService`；落地 `InternalAuthService`、`account.deleted` Kafka topic | 依赖最多、最敏感，放在相对独立的 relation 完成后 | outbox 表、Kafka topic、幂等消费表、压测 DeviceService QPS |
| **第 5 阶段** | 新建 `group-service`（新增域） | 不阻塞任何旧路径，新域最后做反而最稳 | proto 定义、新表 DDL |
| **第 6 阶段** | Gateway / Connect / Msg 全量切换到最终地址；移除 `apps/user` 内已迁移的代码；停写旧表；删旧 proto | 每阶段已分别切换过客户端，这里是收口 | 所有旧路径流量为 0、旧表观察期已满 |

### 每阶段通用验收
- 🟢 CI 全绿 + 新加 RPC 接口单测覆盖率 ≥ 85%
- 🟢 本地 `docker-compose.dev.yml` 能一键起完整链路
- 🟢 指标 / 日志 / trace 可跨服务串联
- 🟢 失败回滚 runbook 已写入 `doc/runbook/`（本文档外单独维护）

> ⚠️ 当前方案保留"每阶段内彻底替换、不保留 shim"的决策。上线前每阶段必须完成：数据库全量备份 + Staging 演练 + 回滚脚本准备。

---

## 十、最终边界原则

1. **一个服务只拥有自己表的写权限**（Phase 1 靠 DB 账号权限 GRANT 控制）。
2. **跨服务查询只调用最小必要视图接口**（`InternalXxxService`），禁用通用 `GetProfile` / `GetOtherProfile`。
3. **账号安全与生命周期统一归 `auth-service`**；跨域清理走 Kafka `account.deleted`。
4. **用户资料主数据统一归 `user-service`**；登录展示字段冗余到 `user_account`，通过 Outbox 异步回写。
5. **用户与用户的关系归 `relation-service`**。
6. **用户与群的关系归 `group-service`**。
7. **每个阶段内彻底替换命名、表名、proto，不保留兼容 shim**；阶段之间独立发布与回滚。
8. **内部 RPC 必须通过 `x-internal-caller` metadata 校验**；对外接口拒绝带该 header 的调用。
9. **服务间跨域最终一致只允许使用 Outbox + Kafka**；禁止同步同事务同库双写。

---

## 十一、Redis Key Ownership Matrix

> 所有新增 key 必须更新此表，同步修改 `consts/redisKey/keys.go`；PR 评审时缺表项的新 key 拒绝合入。

| Key 前缀 | 所有者（写） | 读者 | 用途 | TTL |
|---|---|---|---|---|
| `auth:at:{uuid}:{device}` | auth-service | auth-service、connect-service（只读） | access_token MD5（供 WS 鉴权兜底） | 与 access_token 生命周期一致 |
| `auth:rt:{uuid}:{device}` | auth-service | auth-service | refresh_token | 长期 |
| `auth:code:{channel}:{target}` | auth-service | auth-service | 验证码（邮箱/手机） | 分钟级 |
| `auth:login_lock:{uuid}` | auth-service | auth-service | 登录错误次数限制 | 分钟级 |
| `device:active:{uuid}:{device}` | auth-service（DeviceService） | gateway、connect、auth | 设备活跃时间戳 | 小时级 |
| `device:status:{uuid}:{device}` | auth-service（DeviceService） | connect、auth | 设备在线态 | 与会话一致 |
| `profile:card:{uuid}` | user-service | user-service（可由 Internal API 间接读） | 用户卡片缓存 | 分钟级 |
| `profile:qr:{qrcode_token}` | user-service | user-service | 二维码 token → uuid 映射 | 产品定义 |
| `relation:friend:{uuid}` | relation-service | relation-service | 好友列表缓存 | 分钟级 |
| `relation:apply_unread:{uuid}` | relation-service | relation-service | 好友申请未读数 | 持久 |
| `relation:blacklist:{uuid}` | relation-service | relation-service | 黑名单缓存 | 分钟级 |
| `group:members:{gid}` | group-service | group-service、Push-Job（只读） | 群成员 ID 列表 | 分钟级 |
| `group:info:{gid}` | group-service | group-service | 群基础信息缓存 | 分钟级 |
| `gw:ip_blacklist` | gateway | gateway | IP 黑名单（中间件） | 持久 |
| `gw:ratelimit:ip:{ip}` | gateway | gateway | IP 限流计数 | 秒级 |
| `gw:ratelimit:user:{uuid}` | gateway | gateway | 用户限流计数 | 秒级 |

### 生产加固（可选）
- 按 key 前缀配置 Redis ACL：每服务独立 Redis 账号，只允许读写自己前缀 + 只读其它前缀；
- 建立"谁能读 / 谁能写"的巡检脚本，定期扫描 keyspace 与实际代码不一致的情况。

---

## 十二、端口规划表

| 服务 | 业务 gRPC | Metrics HTTP | 备注 |
|---|---|---|---|
| gateway | — | `:8081`（当前 `:metrics` 路径） | 自身是 HTTP，无独立 gRPC |
| auth-service | `:9090` | `:9190` | 承载 `AuthService` / `DeviceService` / `AccountService` / `InternalAuthService` |
| relation-service | `:9093` | `:9193` | `FriendService` / `BlacklistService` + Internal |
| user-service | `:9094` | `:9194` | `UserService` + `InternalProfileService` |
| group-service | `:9095` | `:9195` | `GroupService` + Internal |
| msg-service | `:9092` | `:9192` | 现有不变 |
| connect-service | `:9091`（gRPC） + `:8082`（WS） | `:9191` | 现有不变，gRPC 承载 `ConnectService` 推送 |

### 规则
- 业务端口 `909X`；对应 metrics 端口 `919X`（+100）。
- Internal service 与对外 service 复用业务端口，通过拦截器鉴权区分（见 2.8）。
- 所有服务暴露 `/health`（HTTP 或 gRPC Health）、`/metrics`。
- 环境变量统一命名：`{SERVICE}_GRPC_ADDR`、`{SERVICE}_METRICS_ADDR`。

---

## 十三、Push-Job 归属与群消息扇出

### 背景
- `apps/connect` 注释明确："纯管道，不对接 Kafka、不做业务判断，上游 Push-Job 查 Redis 路由表后通过 gRPC 精确调用"；
- 原方案未指定 Push-Job 归属与实现位置；
- `msg-service` 现有 `send_message_workflow.go` 里已经区分 P2P / GROUP，但只写 Kafka `msg.push`，未做扩散。

### 方案：Push-Job 作为独立服务 `apps/push-job/`

| 职责 | 说明 |
|---|---|
| 消费 Kafka `msg.push` | 由 `msg-service` 生产 |
| P2P 扩散 | 查 Redis 路由表（`device:*`），按目标 UUID + device_id 调 `connect.ConnectService.PushToDevice` |
| GROUP 扩散 | 调 `group-service.GetGroupMemberIds(conv_id)` 拿成员列表，循环调 `connect.ConnectService.PushToUser` |
| 多端同步 | 对发送方自己的其它设备调 `PushToUser` 并带 `exclude_device_id` |
| 离线兜底 | 消息已经落库（msg），push 失败仅打 Warn，不影响 ACK |

### 依赖
- 下游：`connect-service`、`group-service`
- 上游：`msg-service`（通过 Kafka）
- 不依赖 `auth-service`、`user-service`、`relation-service`

### 为什么独立
- 与 `msg-service` 在业务语义上耦合（消息扩散）但在运维语义上应独立（可横向扩展，承担突发流量）；
- 保持 `connect-service` 纯管道原则不变；
- 如果放 `msg-service` 内，msg 写 DB 的路径会被 push 阻塞，违反"消息落库成功即视为发送成功"的非阻断策略（`apps/msg/internal/usecase/send_message_workflow.go` 头注释）。

> 注：Push-Job 的落地可以延后到 group-service 之后或与其并行；当前文档先把职责与依赖写清楚。

---

## 十四、本地开发指南

### 14.1 完整分布式模式

使用 `docker-compose.dev.yml`（本次拆分过程中维护）一键起：
- MySQL + Redis + Kafka（基础设施）
- gateway + auth + user + relation + group + msg + connect + push-job（7 + 1 业务进程）

### 14.2 Monolith Mode（单进程多 service，开发/集成测试用）

在 `apps/monolith/` 新增一个入口（非生产），通过环境变量控制注册哪些 service：

```text
ENABLE_SERVICES=auth,user,relation,group  # 所有 service 注册在同一个 gRPC Server
# 或
ENABLE_SERVICES=auth                       # 只起 auth-service
```

### 14.3 生产不允许 Monolith
- Monolith Mode 仅限开发 / 集成测试；
- CI 流水线 **默认以完整分布式方式** 跑 E2E；
- 生产镜像不编译 `apps/monolith`。

### 14.4 Trace 串联
- 所有服务必须通过 `ctxmeta` 透传 `trace_id`、`user_uuid`、`device_id`、`client_ip`；
- 跨 Outbox / Kafka 时，事件 payload 必须携带 `trace_id`（`headers` 或 payload field）；
- 拆分后端到端 trace 期望跨越：gateway → auth → Kafka → user / relation / group。

---

## 十五、关键流程失败模式表

### 15.1 注册（见 8.1）

| 失败点 | 用户体验 | 系统动作 | 最终状态 |
|---|---|---|---|
| `user_account` INSERT 失败 | "注册失败" | 整个事务回滚，无副作用 | 无账号、无资料 |
| Outbox 事件 INSERT 失败 | "注册失败" | 事务回滚（与 user_account 同一事务） | 无账号、无资料 |
| Outbox worker 调 `CreateProfile` 失败 | 用户已注册成功，但首次进入资料页可能拿不到 profile | 无限重试 + 指数退避；用 auth 冗余字段兜底展示 | 秒~分钟级内自愈 |
| `CreateProfile` 幂等性失效（重复消费成功） | 用户无感 | UPSERT 天然幂等 | 无副作用 |

### 15.2 登录（见 8.2）

| 失败点 | 用户体验 | 系统动作 | 最终状态 |
|---|---|---|---|
| `user_account` 查询失败 | "登录失败，请稍后重试" | 不写任何 token；返回 5xx | 无副作用 |
| 密码错 / 验证码错 | 明确错误码 | 正常业务错误 | 无副作用 |
| `login_nickname` / `login_avatar` 为空（历史数据） | 展示默认头像 / 邮箱前缀 | 前端兜底；后台打 Warn | 不阻断 |
| user → auth 的 `UpdateLoginDisplay` 尚未到达 | 展示旧昵称/头像 | 正常登录流程；后续异步回写到位 | 秒级自愈 |

### 15.3 注销（见 8.3）

| 失败点 | 用户体验 | 系统动作 | 最终状态 |
|---|---|---|---|
| auth 同步阶段事务失败 | "注销失败" | 整个事务回滚 | 账号完整、Token 未清 |
| auth 同步阶段事务成功但 Redis 删 token 失败 | 注销成功但极短窗口内旧 token 仍可用 | Token 在数据库已失效（下一次 auth 校验会拒绝）；Redis 补偿定期扫描 | 秒级收敛 |
| Outbox worker 投 Kafka 失败 | 用户无感 | worker 重试 | 分钟级内自愈 |
| `user-service` / `relation-service` / `group-service` 消费失败 | 用户无感 | 幂等重试 + DLQ 告警 | 需要人工介入的情况有 DLQ 监控 |

---

## 十六、表迁移 4 步走（针对第 2 阶段 `user_info` → `user_account` + `user_profile`）

> 本章节给出数据迁移的标准动作，适用于未来任何表的拆分。

### Step 1: 双写
- 新表 `user_account` / `user_profile` DDL 创建完成；
- 代码层同时写 `user_info` 和新两张表，旧表仍是读主；
- 新表配置 `login_nickname` / `login_avatar` 冗余字段；
- 运行 N 天（建议 ≥ 3 天），期间监控新旧差异。

### Step 2: 全量回填 + 对账
- 用离线 job 把旧表 `user_info` 历史数据回填到新表（幂等 UPSERT）；
- 启动差异对账 job：定期按 UUID 比对新旧，告警差异 > 阈值；
- 对账连续 0 差异 ≥ 1 天后进入下一步。

### Step 3: 切读
- 代码层改读新表，旧表仍然保持双写；
- 灰度按 UUID 哈希切流（10% → 50% → 100%）；
- 任何问题立刻切回旧表（切流开关；本阶段是唯一能靠开关回退的窗口）。

### Step 4: 停写旧表 + 删表
- 切读 100% 并观察 N 天；
- 停写旧表；
- 再观察 N 天；
- DROP 旧表（本次拆分的"不保留兼容层"在此阶段生效）。

### 4 步走的回滚点
| 步骤 | 回滚方式 | 成本 |
|---|---|---|
| Step 1 失败 | 停止双写，旧表未受影响 | 几乎零成本 |
| Step 2 失败 | 重新回填，不影响业务 | 小 |
| Step 3 失败 | 切回旧表读，双写仍在 | 小 |
| Step 4 失败 | 已 DROP 旧表，只能从备份恢复 | 大（所以 Step 4 前必须全备） |

---

*文档更新时间：2026-04-17*
*评审版本：v2（基于 2026-04-14 初稿，按评审决策整合 Outbox / Kafka / 冗余字段 / 权限矩阵等）*