# Group 服务

## 角色

- 拥有群资料、成员、角色、入群申请、内部成员检查/成员 ID 查询和群缓存投影事实。
- 不维护消息、会话或 WebSocket；Msg 和 Message-push 通过 RPC/事件使用群事实。

## 修改前优先阅读

- apps/group/cmd/providers.go
- apps/group/internal/service/group_service.go
- apps/group/internal/service/group_join_service.go
- apps/group/internal/service/group_member_service.go
- apps/group/internal/service/realtime_notify.go
- apps/group/internal/repository/group_repository.go
- apps/group/internal/repository/group_event_mapper.go
- apps/group/internal/repository/group_cache_projection_store.go
- apps/group/internal/consumer/cache_projector.go
- apps/group/internal/consumer/cache_reconciler.go
- proto/group/group_service.proto

## 稳定业务行为

- 建群会创建群主的有效成员记录，角色值为 2；请求中的成员 UUID 会去重、去空白并排除群主。
- 权限矩阵：解散、转让和角色变更仅群主；名称/头像/公告由管理员或群主；入群模式仅群主；加成员由管理员或群主。
- 群主不能直接退群；解散只变更群状态，不会批量物理删除成员。
- 退出/移除成员采用软删，可被邀请或审批重新激活。
- CheckGroupMember 先验证群状态，不能只信缓存。
- add_mode 为 0 时直接入群，为 1 时生成待审批申请；重复待审批申请返回已存在语义；审批动作 1=同意、2=拒绝。

## group.cache 投影与对账

- 每次群写在同一 MySQL 事务里写群事实和 group.cache Outbox 事件；group_uuid 是 Kafka key。
- group.cache 固定 3 分区。Group 缓存投影和 Msg 成员资格投影使用不同 consumer group，各自默认启动 3 个手动提交 reader；不能由应用在线扩分区。
- Group 侧 projector 只消费已提交事实，不重复做权限判断；非法事件按永久错误进入 dead_events，source 为 group-service:group.cache。
- 缓存读 miss 会调度按群或按用户的 MySQL 快照修复；此外 CacheReconciler 默认每 6 小时扫描群表并带正负 20% 抖动，批次默认 100。配置由 GROUP_CACHE_RECONCILE_INTERVAL 和 GROUP_CACHE_RECONCILE_BATCH_SIZE 控制，非法显式值会使启动失败。
- 对账能修复事件漏投、缓存丢失和部分等版本脏数据；它是最终一致修复，不可替代写路径的事务与事件。

## 实时事件

- 群资料、成员、角色、禁言、入群申请等变化会尽力异步写 realtime.push。
- 这条实时路径不进 Outbox，失败只告警；客户端收到事件后仍应以群查询结果为准。

## 修改陷阱

- 增量缓存事件一般只在缓存已存在时补丁更新；不要假设任意事件都能完整重建缓存。
- 修改群成员语义必须同时检查 Group 投影、Msg 的成员资格投影和 Message-push 的群扩散。
