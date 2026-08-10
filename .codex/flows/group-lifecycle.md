# 群生命周期、投影与实时提醒

Group 是群事实写模型；Redis 群缓存和 Msg 侧群成员资格都是由 Group 事件导出的投影。

## 建群与资料变更

1. 客户端通过 Gateway 的 /api/v1/auth/groups 路由调用 Group。
2. Group 在一个 MySQL 事务内写群、群主成员和对应 group.cache Outbox 事件。
3. group.cache 使用 group_uuid 作为 Kafka key，保持同一群事件的分区内顺序。
4. Group 自身 consumer 投影 Redis；Msg 的独立 consumer group 投影群消息/会话所需的成员资格。
5. 需要提醒在线成员时，Group 尽力异步发布 realtime.push；该提醒不属于 Outbox 提交保证。

群名称、头像、公告、入群模式等权限由 Group 在写路径决定；消费者只投影已经提交的事实，不应重复权限判断。

## 加入、审批和成员变化

- add_mode=0 直接加入，add_mode=1 产生待审批的入群申请。
- 管理员或群主可审批；群主可转让、解散和变更角色，管理员/群主可进行成员管理的相应操作。
- 退群和移除是软删成员，后续邀请或审批可恢复；群主不能直接退群。
- 成员、角色、禁言、解散和申请变化会产生 group.cache 事实事件，并视情况产生 realtime.push 提醒。
- Message-push 在群目标推送时会调用 Group 获取目标成员；投影有延迟时不能把实时推送当作群权限的权威判定。

## 缓存投影与修复层次

1. 正常路径：Group Outbox -> CDC -> group.cache -> Group Redis projector 与 Msg membership projector。
2. 读 miss 修复：Group repository 按群或用户从 MySQL 权威快照异步重建相应缓存。
3. 周期修复：CacheReconciler 默认每 6 小时按 ID 游标扫描 groups 表，默认批 100，并加正负 20% 抖动。

第二、三层用于修复缓存丢失、事件投影失败和脏数据，不能拿来替代 Outbox 的事务写入。周期对账发生后才修复的窗口由配置与扫描进度决定。

## 分区、并发与死信

- group.cache 固定 3 分区；Group 与 Msg 各自默认 3 个 reader。高于分区数的 worker 只会空闲。
- 同群按 key 在分区内有序，不同群可并行；不要由运行中的应用扩分区。
- Group projector 的契约/负载错误会写 group-service:group.cache dead_events；Msg projector 对应 source 为 msg-service:group-membership。
- Msg 成员资格投影把业务更新、版本推进与幂等标记放在一个 MySQL 事务，降低重复消费带来的状态漂移。

## 改动检查

- 改群成员、角色或禁言时，检查 Group 写模型、group.cache event mapper、两个投影、Message-push 扩散和客户端刷新事件。
- 改 Kafka 并发时，只调 worker 数不能增加单群并行；需要先评估固定分区、键和顺序语义。
- 改实时通知时，必须保留查询权威状态的回退路径，因为 realtime.push 是尽力而为。
