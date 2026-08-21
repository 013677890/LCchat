# group 服务
group 服务拥有群资料、群成员、入群申请和群权限事实，负责群生命周期和群缓存投影事件生产。

## 职责
- 创建、解散、查询和更新群资料。
- 维护群公告、加群方式、全员禁言等群级设置。
- 管理群成员：邀请/添加成员、移除成员、主动退群、转让群主、更新管理员角色。
- 管理入群申请：申请、撤销、查询我的申请、管理员审批、待审批数量和审批历史。
- 管理成员展示和权限状态：群名片、单人禁言、成员 ID 列表。
- 在群写操作事务中写入 `group.cache` Outbox 事件，驱动 Redis 群缓存投影最终一致更新。

## 启动与核心目录
| 路径 | 说明 |
| --- | --- |
| `apps/group/cmd` | 服务启动、gRPC 服务、依赖注入和缓存投影消费者装配。 |
| `apps/group/internal/handler` | gRPC handler，负责参数转换和错误映射。 |
| `apps/group/internal/service` | 群业务规则和权限校验。 |
| `apps/group/internal/repository` | 共享协议：领域错误、DTO、状态常量、Redis 编解码与 Lua。 |
| `apps/group/internal/repository/store` | MySQL 权威写与回源读；事务内写业务表、`cache_version` 和 Outbox。 |
| `apps/group/internal/repository/cache` | 同步读缓存。展示列表与发送权限都走最终一致投影；权限点查使用 Hash field。 |
| `apps/group/internal/repository/projection` | 异步 `group.cache` 投影、版本化 Redis 写入、权威对账与 miss 修复调度。 |
| `apps/group/internal/repository/compose` | 把 store 写路径与 cache 读路径组合成 service 使用的 `IGroupRepository` 门面。 |
| `apps/group/internal/consumer` | `group.cache` 投影消费者与周期缓存对账任务。 |
| `proto/group` | GroupService gRPC 契约。 |
| `pkg/groupevent` | 群缓存事件 payload 编解码和 action 常量。 |
## 数据所有权
| 数据 | 存储 | 说明 |
| --- | --- | --- |
| 群资料 | MySQL 群资料表 | 群名称、头像、公告、群主、加群方式、全员禁言。 |
| 群成员 | MySQL 群成员表 | 成员角色、群名片、禁言截止时间等。 |
| 入群申请 | MySQL 入群申请表 | 待审批、已同意、已拒绝、已取消等状态。 |
| 群资料缓存 | Redis `group:info:{group_uuid}` | 读侧加速，非权威事实。 |
| 群成员缓存 | Redis `group:members:{group_uuid}` | message-push 群扩散依赖该投影；Hash 必须有 schema/version/complete 元数据。 |
| 用户群列表缓存 | Redis `group:user_groups:{user_uuid}` + `group:user_group_versions:{user_uuid}` | ZSet、逐群版本 tombstone 与 `__READY__` 共同组成完整投影；命中后按每用户 1 小时租约低频权威对账。 |
| 群待审批申请缓存 | Redis `group:join_requests:{group_uuid}` | 管理员审批列表读侧加速；Hash 必须有 schema/version/complete 元数据。 |
| Outbox 事件 | MySQL `outbox_events` | 事务内写事件，由 Debezium 路由到 `group.cache`。 |
| 幂等消费记录 | MySQL `idempotent_events` | 缓存投影消费者防重复。 |
## 暴露的 gRPC 服务
| 服务 | 能力 |
| --- | --- |
| `GroupService` | 群资料、群成员、入群申请、权限校验和成员 ID 查询。 |
关键 RPC 能力包括：
- `CreateGroup` / `DismissGroup` / `GetGroupInfo` / `UpdateGroupInfo` / `UpdateGroupNotice`。
- `ApplyJoinGroup` / `CancelJoinGroupApplication` / `ReviewJoinGroup` / `ListJoinRequests`。
- `AddMember` / `LeaveGroup` / `RemoveMember` / `GetMemberList` / `SearchGroupMembers` / `SearchGroups`。
- `UpdateMyGroupNickname` / `UpdateGroupMemberNickname` / `TransferGroupOwner` / `UpdateMemberRole` / `MuteGroupMember` / `UpdateGroupMuteSetting`。
- `GetGroupList` / `GetGroupMemberIds` / `CheckGroupMember` / `CheckGroupSendPermission`。

## 事件
| 事件 | 方向 | Topic | 说明 |
| --- | --- | --- | --- |
| `group.cache` | 生产 | `group.cache` | 群资料、成员、申请等写操作产生缓存投影事件。 |
| `group.cache` | 消费 | `group.cache` | 本服务内分区级并行投影消费者应用 Redis 缓存更新。 |

msg-service 还会用另一个 consumer group 完整消费同一 topic，维护它自己拥有的
`conversation` 群成员会话投影。group 仍是成员权威源；message-push 不中转该事件。

`group.cache` 固定 3 partitions；本服务默认启动 3 个独立 Reader（`KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY`），
不同 partition 并行、同 partition 串行；同群因 Kafka key=`group_uuid` 严格有序。
显式 workers 必须为 `1～64`；Kafka rebalance 自动分配 partition，多余 Reader 正常 idle。
projector 作为 API 旁路，Pool 致命失败会整池收敛并在 GroupApp 内持续告警、退避重启，不中断 gRPC。
常见 `group.cache` action：
| action | 触发场景 | 影响缓存 |
| --- | --- | --- |
| `group_created` | 创建群 | 群资料、成员、用户群列表。 |
| `group_info_updated` | 更新群资料 | 群资料。 |
| `group_dismissed` | 解散群 | 群资料、成员、用户群列表。 |
| `member_added` | 添加成员 | 成员集合、用户群列表。 |
| `member_removed` | 移除成员/退群 | 成员集合、用户群列表。 |
| `owner_transferred` | 转让群主 | 群资料、成员角色。 |
| `member_role_updated` | 设置/取消管理员 | 成员角色。 |
| `member_profile_updated` | 更新群名片 | 成员展示字段。 |
| `member_muted` | 单人禁言 | 成员禁言字段。 |
| `group_mute_setting_updated` | 全员禁言 | 群资料权限字段。 |
| `join_request_created` | 创建入群申请 | 待审批申请缓存。 |
| `join_request_reviewed` | 审批入群申请 | 申请缓存、成员缓存。 |
| `join_request_canceled` | 撤销申请 | 待审批申请缓存。 |
## 协作关系
- Gateway 通过 GroupService 暴露 HTTP 群接口。
- msg 发送群消息前调用 group 权限能力校验成员身份、单人禁言、全员禁言和群状态。
- message-push 群聊下行时依赖 `group:members:{group_uuid}` 或 group 成员能力拆分群成员。
- user 资料变更后，如需要更新成员展示字段，应通过事件或内部能力更新群成员投影。

## 缓存投影链路
1. group 写操作在 MySQL 事务中修改业务表。
2. 每条事件在同一事务递增 `groups.cache_version`，再调用 `outbox.InsertEvent` 写 `schema_version=2` 和 `projection_version`。
3. Debezium EventRouter 以 `expand.json.payload=true` 输出顶层 JSON Object，并路由到 Kafka `group.cache`。
4. group 缓存投影消费者严格校验 schema/version、action 必需字段及最终状态语义，再查 `idempotent_events`。
5. 未处理事件调用 `ApplyGroupCacheEvent`，由 Lua 比较版本并原子更新 Redis。
6. 投影成功后写 `idempotent_events` 并提交 offset；周期对账按群从 MySQL 完整快照修复漂移。

## 降级策略
- 群写操作以 MySQL 事务为准；缓存投影失败不会回滚已提交业务数据。
- Redis 可重试错误返回消费者重试；非法 payload 首轮写 `dead_events`，死信成功后才提交。
- 读路径如果缓存未命中会回源 MySQL，并以重新读取的 `cache_version` 快照异步修复；不把请求线程的旧对象直接晚写缓存。

## 不变量
- 群事实以 MySQL 为准，Redis 只是投影缓存。
- 群写操作必须在事务内写业务表和 Outbox 事件，不能先写缓存再写库。
- 群主不能直接退群；必须转让群主或解散群。
- 管理员不能越权处理群主或更高权限成员。
- message-push 使用群成员缓存做扩散时，必须能容忍缓存最终一致延迟。
