# 好友、黑名单与实时提醒流程

Relation 是好友和黑名单的唯一写服务。客户端通过 Gateway 访问，Msg 只通过 Relation RPC 判断发送权限。

## 好友申请与处理

1. 调用 POST /api/v1/auth/friend/apply，Gateway 将已认证上下文交给 Relation.SendFriendApply。
2. Relation 写入好友申请事实，并更新申请相关缓存；目标用户的未读申请计数使用其 Redis ZSet/计数相关键。
3. 申请创建成功后，Relation 尽力异步发布 FriendApplyCreated 到 realtime.push，目标是申请接收方。
4. 接收方调用 POST /api/v1/auth/friend/apply/handle。接受操作在同一事务创建双方的单向关系；拒绝只改变申请状态。
5. 处理完成后，Relation 尽力发布 FriendApplyHandled 和好友关系变化提醒。

realtime.push 不是申请事实的可靠投递通道。发布、编码或异步执行失败只记日志；客户端应通过申请列表、未读数和好友列表查询自愈。

## 好友关系维护

- GET list、POST sync、POST check、POST relation 都读 Relation 的关系事实/缓存。
- SyncFriendList 有 2 秒游标回退以降低边界漏读；调用方必须接受重复项并按版本/主键去重。
- POST delete 只删除当前用户指向对方的单向关系；产品上不能把它理解成自动双向删除。
- remark 与 tag 修改也是关系本域事实更新，并会尝试发送关系变化实时提醒。
- GetTagList 尚未实现，设计新客户端功能时不能假定已有查询 API。

## 黑名单

1. POST /api/v1/auth/blacklist 或 DELETE /api/v1/auth/blacklist/:userUuid 调用 Relation 写单向黑名单。
2. 黑名单变化会影响关系状态，并尝试向当前用户发送关系变化提醒。
3. Msg 的单聊发送权限需要以 Relation 的当前检查结果为准；Redis 缓存或消息侧短时读取可能使写后可见性晚于 MySQL 提交。

因此拉黑/解除拉黑不是全局同步栅栏：它提供最终一致的缓存修复，不能承诺所有节点在同一瞬间切换。对安全或强阻断需求应在 Msg 的权威发送校验处施加规则。

## 缓存失败补偿

- 关系写后的缓存任务可异步运行；协程池拒绝或 Redis 最终写失败时，Relation 尝试发布 relation.redis.invalidate。
- Kafka consumer 对任务严格解码后仅执行整键 DEL，成功后提交 offset；永久负载错误或预算耗尽进入 dead_events。
- Kafka 本身不可用时补偿任务没有持久后备，只会告警。MySQL 关系事实仍为权威，后续读 miss/失效应回源修复。

## 注销清理

Auth 的 account.deleted 事件由 Relation 消费，软删用户关联的关系与申请并失效缓存。它不能跨服务删除账号或资料表。

## 改动检查

- 任何好友申请去重/重试改动必须联合检查唯一约束、事务、Gateway 超时和 gRPC 重试。
- 任何发送权限改动必须联合检查 Relation 检查 RPC、Msg 调用点与缓存时效。
- 不要为了修复缓存而加入旧写命令回放；整键 DEL 是防止旧状态覆盖新事实的设计边界。
