# Message 整体架构

msg 服务负责消息与会话领域，不负责 WebSocket 连接和最终下行投递。它的职责边界是：校验发送权限、生成消息事实、维护会话读模型，并在业务事务内写入下行 outbox 事件。

## 分层结构

| 层 | 目录 | 职责 |
| --- | --- | --- |
| handler | `apps/msg/internal/handler` | gRPC 薄层，参数转换、上下文读取、错误映射。 |
| usecase | `apps/msg/internal/usecase` | 跨领域编排，例如发送、撤回、标记已读。 |
| message domain | `apps/msg/internal/domain/message` | 消息幂等、msg_id、conv_id、seq、落库、撤回、拉取。 |
| conversation domain | `apps/msg/internal/domain/conversation` | 会话列表、未读、read_seq、clear_seq、置顶免打扰。 |
| cli | `apps/msg/internal/*cli` | 调用 relation、group 等外部权限服务。 |

domain 不直接依赖其他 domain；msg-service 不直接写 `msg.push` Kafka，跨领域动作由 usecase 编排，Kafka 投递由 Debezium CDC 完成。

## 消息发送主流程

1. gateway 调用 msg `SendMessage`。
2. usecase 调用权限检查器：
   - 单聊校验好友和黑名单。
   - 群聊校验成员、角色、全员禁言、单人禁言。
3. message domain 执行幂等检查。
4. 生成 `msg_id`、`conv_id`，通过 Redis `msg:seq:{conv_id}` 分配会话内 `seq`。
5. 在同一 MySQL 事务内写入 `message` 表和 `outbox_events(event_type=msg.push)`。
6. conversation domain 更新发送方会话。
7. 单聊写扩散更新接收方会话；群聊更新 `group_conversation` 热数据，并尝试为群成员初始化会话行。
8. Debezium 监听 `outbox_events`，将 `MsgPushEvent` protojson 路由到 Kafka `msg.push`。
9. 返回 `msg_id`、`seq`、`conv_id`、`send_time`。

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

## Outbox 与 CDC 非阻断策略

消息落库成功即代表发送成功。以下失败只记录 Warn，不回滚消息事实：

- 更新发送方或接收方会话失败。
- 更新群会话热数据失败。
- 初始化群成员会话行失败。
- CDC / Kafka 投递延迟或短暂失败。

自愈方式：客户端后续通过 PullMessages 和会话同步拿到权威状态。

## 撤回和已读

| 操作 | 权威写入 | 下行事件 |
| --- | --- | --- |
| RecallMessage | 更新消息 `status=1`，内容改为撤回提示 | `MSG_RECALL`。 |
| MarkRead | `read_seq = max(old, req.read_seq)`，更新未读数 | `MSG_MARK_READ` 同步自己其他设备；P2P 额外发 `MSG_READ_RECEIPT` 给对端。 |

## 与下游关系

- msg 只写 `msg.push` outbox，不查在线路由。
- message-push 消费 `msg.push` 后查 Redis 路由并调用 connect。
- connect 只负责把 `MessageEnvelope` 写入本地 WebSocket 连接。
