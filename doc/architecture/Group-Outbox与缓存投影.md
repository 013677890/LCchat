# Group-Outbox 与缓存投影

group 服务使用 MySQL Outbox、Debezium、Kafka 和 Redis projector 维护群缓存。MySQL 是唯一权威事实；group Kafka projector 是正常写链路中唯一的 Redis 投影者，读 miss 与周期对账只允许用带数据库版本的完整快照修复缓存。同一 `group.cache` 还由 msg 的独立消费组维护成员会话投影，但它只写 msg 自己拥有的表，不参与 Redis 写入。

代码组织上，group 仓储按调用语义拆成子包：`repository/store` 只做 MySQL 权威写和回源读；`repository/cache` 做最终一致的同步读，发送权限走成员 Hash field 点查而不是 `HGETALL`；`repository/projection` 负责异步投影与对账；`repository/compose` 把 store 与 cache 组合成 service 使用的 `IGroupRepository`。共享错误、常量和 Redis 编解码留在父包 `repository`，避免父包再 import cache 造成循环依赖。

## 1. 一致性边界

- 群业务表、`groups.cache_version` 和 `outbox_events` 在同一个 MySQL 事务内提交或回滚。
- 每写一条 `group.cache` 事件，`cache_version` 都先递增一次；同一事务写两条事件时分别获得 `N+1`、`N+2`。
- `entity_id` 固定为 `group_uuid`，作为 Kafka key；同群事件哈希到同一 partition，由该 partition 上的单个 Reader 串行消费，保证严格有序。
- Redis 还会在 Lua 内比较 `projection_version`。因此重复事件、乱序事件、消费者部分成功后重试以及晚到的读回填都不能覆盖更高版本。
- 业务写请求提交后不再直接异步 patch Redis，避免“请求协程 patch”和 Kafka projector 两个无序写者互相覆盖。

## 2. 严格事件契约

`group.cache` 当前只接受 `schema_version=2`，基础字段如下：

| 字段 | 约束 |
| --- | --- |
| `schema_version` | 必须精确等于 `2`。缺失、旧版本和未来未知版本都拒绝。 |
| `projection_version` | 必须大于 `0`，取自同事务递增后的 `groups.cache_version`。 |
| `event_id` | 必填，用于 `idempotent_events` 消费幂等。 |
| `action` | 必须是当前代码声明的 action。 |
| `group_uuid` | 必填，同时作为 Outbox `entity_id` 和 Kafka key。 |

成员快照中的 `mute_until_unix_ms` 固定编码为十进制字符串，零值为 `"0"`。这样
Debezium/Kafka Connect 不会因为同一成员数组中同时出现零值和毫秒时间戳而推断出
冲突的整数 Schema。严格解码器不接受旧数字格式，也不做字符串/数字双读兼容。

解码器只接受顶层 JSON Object，并启用未知字段拒绝。它不再支持 JSON 字符串、`payload`、`after`、`data` 等旧包装，也不会把缺失版本解释为 `0`。Debezium EventRouter 因此固定配置：

```text
transforms.outbox.table.expand.json.payload=true
```

action 的终态语义由 `pkg/event.ValidateGroupCachePayload` 唯一定义，group Redis
projector 与 msg membership projector 共用该校验器；禁止消费者各自放宽字段、推导目标
集合或保留第二套契约。

常见 action：

| 动作 | Action | 主要 payload |
| --- | --- | --- |
| 建群 | `group_created` | 群快照、初始成员快照、与成员集合完全一致的 `user_uuids`。 |
| 新增或恢复成员 | `member_added` | 群快照、成员快照、与成员集合完全一致的 `user_uuids`。 |
| 移除成员或退群 | `member_removed` | 群快照、目标 `user_uuid`。 |
| 解散群 | `group_dismissed` | 终态群快照、活跃成员 UUID。 |
| 更新群资料 | `group_info_updated` | 最新群快照。 |
| 转让群主 | `owner_transferred` | 群快照、老群主和新群主最终成员快照；新群主角色必须为 owner，旧群主必须降为 member。 |
| 更新成员角色/名片/禁言 | `member_role_updated` / `member_profile_updated` / `member_muted` | 群快照、受影响成员最终快照。 |
| 更新全员禁言 | `group_mute_setting_updated` | 最新群快照。 |
| 创建/审批/撤销入群申请 | `join_request_created` / `join_request_reviewed` / `join_request_canceled` | 申请快照。 |

## 3. 完整链路

```text
group gRPC 写请求
  -> MySQL 事务
       -> groups / group_members / group_join_requests
       -> groups.cache_version = cache_version + 1
       -> outbox_events(event_type=group.cache, entity_id=group_uuid)
  -> Debezium EventRouter(expand JSON)
  -> Kafka group.cache (固定 3 partitions；key=group_uuid)
       ├─ consumer group: group-cache-projector-group
       │    -> N 个独立 Reader（默认 N=3，KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY）
       │    -> 不同 partition 并行；同 partition 串行 Fetch/Handle/Commit
       │    -> 严格校验 + idempotent_events
       │    -> 版本感知 Lua 投影 Redis
       │    -> commit offset
       └─ consumer group: msg-group-membership-projector-group
            -> N 个独立 Reader（默认 N=3，KAFKA_MSG_GROUP_MEMBERSHIP_PROJECTOR_CONCURRENCY）
            -> 不同 partition 并行；同 partition 串行；同群严格有序
            -> 严格校验 + 单群连续版本检查
            -> 事务内增量更新 conversation / group_conversation
            -> 事务内标记 idempotent_events
            -> commit offset
```

不是 `Kafka -> message-push -> group`。`group.cache` 由 group-service 和 msg-service 使用不同 consumer group 各自完整消费；message-push 只消费 `msg.push` / `realtime.push`。

并行投影不改变一致性边界：

- 同群全部事件仍因 `entity_id=group_uuid` 进入同一 partition，Reader 内串行处理；
- 不同群可跨 partition 并行；
- Redis Lua 版本栅栏与 msg 连续 `projection_version` 校验保持不变；
- 任一 partition worker 致命退出会取消同进程其余 worker 并等待收敛，禁止静默降级并发；
- 禁止应用启动时 `alter topic --partitions`；已有积压 topic 扩分区必须停写排空后重建或迁移。

### 3.1 msg 成员会话投影

msg 不复制 `group_members` 全表，只保存会话读模型所需字段：

| 事件 | msg 写入 |
| --- | --- |
| `group_created` / `member_added` | 激活事件携带的成员 `conversation` 行，记录成员版本和入群时间。 |
| `member_removed` | 将目标成员写为 Left tombstone，保留个人 read/mute/pin/status。 |
| `group_dismissed` | 将一条 `group_conversation.group_status` 写为 Dismissed。 |
| 其他 action | 不改成员集合，只推进单群 `projection_version`。 |

成员变更、版本推进和幂等标记在同一 MySQL 事务内完成。`projection_version <= current`
视为重复/旧事件；`projection_version != current+1` 视为链路缺口并进入 msg 死信，
禁止跳过缺口。群消息发送只更新发送方个人行和一条共享群热数据，不再查询全群成员。

## 4. Redis v2 投影格式

| Key | 类型 | v2 元数据与写入策略 |
| --- | --- | --- |
| `group:info:<group_uuid>` | String | 固定为 `2|<projection_version>|<json>`；负缓存唯一合法格式为 `2|0|__NOT_FOUND__`。 |
| `group:members:<group_uuid>` | Hash | 保留 field `__SCHEMA__=2`、`__VERSION__=<version>`、`__COMPLETE__=1`；业务 field 为 `user_uuid`。 |
| `group:join_requests:<group_uuid>` | Hash | 同样保留 schema/version/complete；业务 field 为 `apply_id`。 |
| `group:user_groups:{<user_uuid>}` | ZSet | member 为 `group_uuid`，score 为群 `updated_at` 毫秒值；花括号是 Redis Cluster hash tag。 |
| `group:user_group_versions:{<user_uuid>}` | Hash | 每个 `group_uuid` 保存最后成员关系版本；删除后仍保留 tombstone，并用 `__READY__=1` 表示群列表已经完整对账。 |
| `group:user_groups_reconcile_lease:{<user_uuid>}` | String | 用户群 READY 缓存命中后的权威对账租约，固定 TTL 1 小时；仅限频，不是业务投影。 |

旧 JSON String、缺少 `__SCHEMA__` / `__VERSION__` / `__COMPLETE__=1` 的 Hash、只有 ZSet 没有版本 Hash 的用户群列表都视为无效缓存：点查 Lua 会在同一原子操作中删除，完整集合读按 miss 回源并交给权威对账覆盖；任何路径都不会读取旧结构继续运行。`__COMPLETE__` 使增量 Lua 能以 O(1) 判断 Hash 是否由一次完整重建产生，禁止把仅有元数据或局部 field 的 Hash 提升为可信全集。

### 4.1 Lua 原子性

- 群资料：一段 Lua 完成格式校验、版本比较、`SET` 和 TTL。
- 成员/申请 Hash：一段 Lua 完成版本比较、整表替换或批量 field patch、空集合哨兵和 TTL。
- 同一事件影响多个成员时一次传给 Lua。例如群主转让不能循环执行两次，否则第一次推进版本后第二次会被判重。
- 用户群索引：一段 Lua 同时更新 ZSet 和版本 Hash；两种 Key 使用同一 hash tag。
- 成员权限点查由一段 Lua 同时读取 Hash 的 schema、version 和目标 field；用户群列表也由一段 Lua 同时读取 ZSet、`__READY__` 和各群版本，禁止用 Pipeline 观察联动状态。
- Kafka 增量脚本只在 `incoming_version > stored_version` 时写入；相等视为重复，小于视为乱序。
- MySQL 权威对账允许 `incoming_version == stored_version` 时重写，用于修复“版本仍正确但业务值被篡改/部分丢失”；仍严格拒绝更低版本。

## 5. 读 miss 与对账

读路径缓存 miss 后先返回本次 MySQL 查询结果，再异步触发修复。修复任务只携带 `group_uuid` 或 `user_uuid`，不会把请求线程刚读到的对象直接写 Redis：

1. 后台重新开启 MySQL 事务；
2. 对 `groups` 行持共享锁，一致性读取群、`cache_version`、全部历史成员和待审批申请；
3. 用该版本执行完整 Lua 投影；
4. 若期间已有更高版本 Kafka 事件落 Redis，Lua 拒绝旧快照。

`CacheReconciler` 启动时立即执行一轮，之后按 ID keyset 分批扫描所有群并调用 `ReconcileGroupCache(group_uuid)`。后续轮次在上一轮完成后，以配置周期为基准重新生成 ±20% 抖动的等待时间；默认 `6h` 基准对应 `4h48m`～`7h12m`，既打散多实例同频扫描，也避免单轮耗时超过周期时积压或重叠。它会修复群资料、有效成员、待审批申请，并根据历史成员记录为退出/被踢用户补写反向索引 tombstone。单轮仍会继续扫描失败群之后的目标，但只保留 20 条逐群错误样本和遗漏计数，避免 Redis 全局故障时错误对象按群数量无限增长。软删除群会投影为带正版本的不可用终态，不会因数据库 `status=0` 被缓存复活。

按用户触发的完整对账还会读取当前版本 Hash 作为脏 UUID 提示，以 MySQL 事实删除“缓存里存在、成员历史中不存在”的多余群。它先发现候选群并按 UUID 排序锁定全部 `groups` 行，再在同一事务读取权威 `group_members`；发现锁集合外的新关系时扩充候选并重试，禁止拼接“旧成员关系 + 新 cache_version”。并发新事件拥有更高版本，不会被旧快照删除。

缓存 miss 会立即触发上述用户对账；结构合法的 READY 命中也会尝试取得每用户 1 小时 Redis 租约，取得后在后台对账。因此即便缓存错误加入了一个该用户在 DB 中从未加入的群，修复路径仍然可达，又不会让每次列表请求都访问 MySQL。

配置：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `GROUP_CACHE_RECONCILE_INTERVAL` | `6h` | 相邻轮次之间的基准等待时间；每轮加入 ±20% 抖动。Go duration，只接受带单位的正值。 |
| `GROUP_CACHE_RECONCILE_BATCH_SIZE` | `100` | 每批群数量，只接受正整数。 |

显式配置非法时 group-service 启动失败，不静默采用其他格式或旧变量。

## 6. 失败与死信

| 失败 | 处理 |
| --- | --- |
| JSON、schema、版本或必填字段非法 | handler 返回 `kafka.Permanent`，第一次失败立即写 `dead_events`；死信成功后才提交 offset。 |
| 幂等检查失败 | 普通可重试错误，不提交 offset。 |
| Redis / MySQL 短暂失败 | 普通可重试错误，原地重试；预算耗尽后写死信。 |
| 死信落地失败 | 不提交 offset，继续阻塞重试，保证原消息不丢。 |
| 写幂等记录失败 | 不提交 offset；Redis Lua 和版本栅栏保证重试安全。 |
| offset 提交失败 | 停留在当前消息，只重试提交；不重复执行 handler，也不重复插入死信。 |

msg projector 使用自己的 `dead_events(source=msg-service:group-membership)` 和
`idempotent_events(event_type=group.cache:msg-membership-projector)`；它与 group Redis
projector 的消费进度、死信和幂等命名空间完全独立。

## 7. 不兼容升级顺序

v2 有意不兼容旧事件和旧 Redis 格式，发布时必须协调切换：

1. 暂停 group 写流量和旧 projector；
2. 执行 `scripts/migration/005_group_cache_projection_version.sql`，已有群初始化为版本 `1`；
3. 更新 Debezium connector，使 `expand.json.payload=true`；
   `register_outbox_connector.sh` 更新已有 connector 前先等待 `STOPPED` 且 tasks 清空，再 PUT、resume；只有新 generation 的 connector 与 task 0 都为 `RUNNING` 时才成功，任一状态为 `FAILED` 立即失败；
4. 部署 group-service v2；
5. 检查旧 backlog 产生的 `dead_events`，由首轮全量对账按 MySQL 当前事实重建缓存；
6. 恢复写流量并观察 consumer lag、死信和对账日志。

禁止在新旧 group-service 之间滚动混跑；旧生产者生成的无版本事件会被 v2 消费者明确拒绝。

## 8. 维护要求

- 新增群写动作时，必须同时补 action、严格 payload 校验、事务内版本领取、projector 分支、乱序测试和本文档。
- 新 action 若改变成员集合或群可见状态，还必须同步更新 msg membership projector 分支与测试；不改变集合的 action 也必须推进 msg 单群版本。
- 改动缓存格式时必须提高 schema 版本并采用协调切换，不增加旧格式 fallback。
- 改动 Redis Key 或 TTL 时同步更新 [Redis-Key 设计](../datas/Redis-Key设计.md)。
- 改动 Outbox 或 Debezium 路由时同步更新 [Kafka 事件](../datas/Kafka事件.md) 与 [事件驱动与最终一致性](./事件驱动与最终一致性.md)。
