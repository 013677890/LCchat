# 消息发送、下行与 ACK 链路

涉及发送、撤回、已读、推送或 WebSocket ACK 时读取本文件；HTTP 入口见 api-map.md，事件投递语义见 outbox-and-consumers.md。

## 主发送路径

1. 客户端调用 POST /api/v1/auth/messages/send，Gateway 从认证上下文取得身份/设备信息并转给 Msg。
2. Msg 发送工作流先做关系或群成员权限校验：单聊拒绝自发、双向黑名单并要求好友关系；群聊要求 Group.CheckGroupMember。
3. Message domain 分配消息 ID、进行幂等判定并从 Redis 分配会话 seq；Redis INCR 先成功、MySQL 插入失败时可能留下 seq 空洞。
4. Msg 在同一个 MySQL 事务写 message、会话派生状态和 msg.push Outbox。这里提交后消息事实成立。
5. Debezium CDC 按 event_type 把 Outbox 投递到 Kafka msg.push，而不是由 Msg 成功直写 Kafka 才算发送成功。
6. Message-push 消费 msg.push：单聊查询 Redis 在线路由；群聊通过 Group 取得目标成员，再按 Connect 节点分组调用内部推送 RPC。
7. Connect 将 protobuf envelope 写到 WebSocket。客户端对 MSG_PUSH 的 ACK 更新 Redis ACK 水位。
8. 推送失败或离线不回滚消息事实；客户端用 PullMessages 或 BatchSyncMessages 按 seq 自愈。

## 会话与群成员投影

- P2P 会话 ID 为 p2p-加两个排序 UUID；群会话以群 UUID 为目标。
- Group 成员资格来自 Msg 对 group.cache 的独立消费投影，不由发送时批量建所有成员会话。
- Group 发送只更新发送者个人会话行和共享 group_conversation；未读由 max_seq - read_seq 导出。
- group.cache 与 Msg projector 之间有延迟，成员变更、群发送权限和下行目标必须分别检查其权威服务/投影语义。

## 撤回与已读

- 撤回的消息状态数据库更新是权威事实；MSG_RECALL 下行是 best effort。
- 已读的数据库更新是权威事实；MSG_MARK_READ 用于同账号设备同步，MSG_READ_RECEIPT 用于对端提醒。
- 这些推送失败都不回滚数据库状态；客户端应以后续拉取或状态查询为准。

## 可靠性边界

- MSG_PUSH 有 seq，客户端可以通过缺口拉取修复。
- MSG_RECALL、已读事件和 realtime.push 不一定有同等的 seq 补洞路径，设计客户端交互时要提供查询/刷新兜底。
- Message-push 采用有限本地重试，耗尽后让出 offset 并不写 dead_events；这保护消费分区可用性，但不承诺实时下行至少一次。
- ACK 的适用范围和 conv_id 约束见 Connect 服务说明与 risks/watchlist.md。

## 先查代码

- Gateway：apps/gateway/internal/router、apps/gateway/internal/service/msg_service.go
- Msg：apps/msg/internal/handler/msg_handler.go、internal/usecase、internal/domain/message、internal/domain/conversation
- 事件：pkg/outbox、scripts/cdc/register_outbox_connector.sh、proto/msg/msg_push_event.proto
- 下行：apps/message-push/internal/consumer、apps/connect/internal/grpc/server.go、apps/connect/internal/svc/ack.go
