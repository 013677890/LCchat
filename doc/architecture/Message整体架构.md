# Message 整体架构

msg 服务负责消息与会话领域，不负责 WebSocket 连接和最终下行投递。它的职责边界是：校验发送权限、生成消息事实、维护会话读模型，并在业务事务内写入下行 outbox 事件。

## 分层结构

| 层 | 目录 | 职责 |
| --- | --- | --- |
| handler | `apps/msg/internal/handler` | gRPC 薄层，参数转换、上下文读取、错误映射。 |
| usecase | `apps/msg/internal/usecase` | 跨领域编排，例如发送、撤回、标记已读。 |
| message domain | `apps/msg/internal/domain/message` | 消息幂等、msg_id、conv_id、seq、落库、撤回、拉取。 |
| conversation domain | `apps/msg/internal/domain/conversation` | 会话列表、未读、read_seq、clear_seq、置顶免打扰、群成员投影。 |
| consumer | `apps/msg/internal/consumer` | 独立消费 `group.cache`，维护 msg 本地群成员会话投影。 |
| cli | `apps/msg/internal/*cli` | 调用 relation、group 等外部权限服务。 |

domain 不直接依赖其他 domain；msg-service 不直接写 `msg.push` Kafka，跨领域动作由 usecase 编排，Kafka 投递由 Debezium CDC 完成。

## 消息发送主流程

1. gateway 调用 msg `SendMessage`。操作者身份（`from_uuid`/`device_id`）经 gRPC metadata 传递，请求体不含身份字段；msg handler 从 `ctxmeta` 解析后显式传给 usecase/domain。
2. usecase 调用权限检查器：
   - 单聊校验好友和黑名单。
   - 群聊校验成员、角色、全员禁言、单人禁言。
3. message domain 执行幂等检查。
4. 生成 `msg_id`、`conv_id`，通过 Redis `msg:seq:{conv_id}` 分配会话内 `seq`。
5. 在同一 MySQL 事务内写入 `message` 表和 `outbox_events(event_type=msg.push)`。
6. conversation domain 更新发送方会话。
7. 单聊写扩散更新接收方会话；群聊只更新发送方个人会话和一条 `group_conversation` 热数据，不扫描群成员。
8. Debezium 监听 `outbox_events`，将 `MsgPushEvent` protojson 路由到 Kafka `msg.push`。
9. 返回 `msg_id`、`seq`、`conv_id`、`send_time`。

群成员会话不属于发送消息主流程。group-service 的建群、加人、退群和解散事务产生
`group.cache`；msg 使用独立 consumer group 增量维护 `conversation.membership_*` 和
`group_conversation.group_status/projection_version`。message-push 不参与这条投影链路。

## 幂等设计

| 维度 | 说明 |
| --- | --- |
| 客户端 ID | `client_msg_id` 由客户端生成，建议 UUID。 |
| 幂等三元组 | `from_uuid + device_id + client_msg_id`。 |
| Redis Key | `msg:idempotent:{from_uuid}:{device_id}:{client_msg_id}`，TTL 10 分钟。 |
| DB 约束 | `message` 表对发送方、设备、客户端 ID 建唯一约束。 |
| 命中语义 | 返回首次发送生成的 `msg_id`、`seq`、`conv_id`。 |

## 会话模型

| 字段 | 说明 |
| --- | --- |
| `conv_id` | 会话 ID，单聊可由双方 UUID 排序生成，群聊通常为群 UUID。 |
| `owner_uuid` | 会话归属用户，单聊双方各一条，群聊每个成员一条。 |
| `target_uuid` | 单聊为对端，群聊为群 UUID。 |
| `max_seq` | 会话当前最大 seq。 |
| `read_seq` | 当前用户已读最大 seq。 |
| `clear_seq` | 删除会话时的清理位点，拉取时过滤旧消息。 |
| `mute`、`pin` | 免打扰和置顶设置。 |
| `status` | 用户个人会话状态；与群成员资格独立。 |
| `membership_status` | 群成员投影：1 有效，2 已退出；P2P 固定为 0。 |
| `membership_version` | 最后应用到该成员行的 `group.cache projection_version`。 |

`group_conversation` 保存群共享 `max_seq/last_msg_*`，以及 `group_status` 和单群连续
`projection_version`。全量会话列表只返回有效成员且群状态正常的行；增量列表会把退群、
群解散和个人删除统一映射为 `status=1` tombstone，供离线客户端删除本地会话。
消息写入用 seq 作为整组更新栅栏，只有更大的 seq 才能同时推进
`max_seq/last_msg_*/updated_at`，迟到的旧 workflow 不能回退群预览和排序。
删除群会话时 `clear_seq/read_seq` 取共享 `group_conversation.max_seq`；删除后若共享
`max_seq > clear_seq`，列表视图会以 `status=0` 重新出现，无需给每个成员写一条激活更新。

## Outbox 与 CDC 非阻断策略

消息落库成功即代表发送成功。以下失败只记录 Warn，不回滚消息事实：

- 更新发送方或接收方会话失败。
- 更新群会话热数据失败。
- CDC / Kafka 投递延迟或短暂失败。

自愈方式：客户端后续通过单会话 PullMessages、多会话 BatchSyncMessages 和会话同步拿到权威状态。

`group.cache` 投影不采用上述“下一条消息补偿”语义：它按事件 ID 幂等、按单群版本连续
推进；非法事件或版本缺口进入 msg 自己的 `dead_events`，禁止跳过版本拼出不完整成员视图。

## 撤回和已读

| 操作 | 权威写入 | 下行事件 |
| --- | --- | --- |
| RecallMessage | 更新消息 `status=1`，内容改为撤回提示 | `MSG_RECALL`。 |
| MarkRead | `read_seq = max(old, req.read_seq)`，更新未读数 | `MSG_MARK_READ` 同步自己其他设备；P2P 额外发 `MSG_READ_RECEIPT` 给对端。 |

## 与下游关系

- msg 只写 `msg.push` outbox，不查在线路由。
- msg 以独立消费组读取 `group.cache`，只写自己拥有的会话投影表；group 仍拥有成员权威事实。
- message-push 消费 `msg.push` 后查 Redis 路由并调用 connect。
- connect 只负责把 `MessageEnvelope` 写入本地 WebSocket 连接。
