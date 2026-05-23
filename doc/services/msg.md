# msg 服务

msg 服务拥有消息、会话和已读位点事实，负责消息落库、会话维护、消息拉取、撤回和已读同步事件生产。

## 职责

- 单聊和群聊统一发送入口，分配消息 ID 和会话内递增 `seq`。
- 基于 `(from_uuid, device_id, client_msg_id)` 做发送幂等，避免弱网重试重复落库。
- 拉取历史消息、按消息 ID 批量反查、撤回消息。
- 维护会话列表、最后消息预览、未读数、免打扰、置顶和逻辑删除位点。
- 标记会话已读，并投递多端同步通知。
- 将新消息事件写入 MySQL `outbox_events`，由 Debezium CDC 路由到 Kafka `msg.push`，再由 message-push 做下行投递。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/msg/cmd` | 服务启动、gRPC 服务、Kafka Producer、依赖注入。 |
| `apps/msg/internal/handler` | gRPC handler，薄适配层。 |
| `apps/msg/internal/domain/message` | 消息领域：消息落库、seq 分配、撤回、拉取。 |
| `apps/msg/internal/domain/conversation` | 会话领域：会话列表、已读、删除、设置。 |
| `apps/msg/internal/usecase` | 跨领域工作流：发送、撤回、已读。 |
| `apps/msg/internal/groupcli` | 群权限校验客户端。 |
| `apps/msg/mq` | Kafka Producer（撤回、已读链路仍在迁移中）。 |
| `proto/msg` | MsgService、MsgItem、MsgPushEvent 契约。 |

## 分层规则

| 层 | 规则 |
| --- | --- |
| handler | 只做参数转换和错误映射，不写业务规则。 |
| message domain | 只处理消息事实，不依赖 conversation domain。 |
| conversation domain | 只处理会话事实，不依赖 message domain。 |
| usecase | 唯一允许协调多个 domain；新消息 Kafka 下行事件通过 message/outbox 同事务生产。 |
| repository | 负责 MySQL/Redis 访问，不做跨领域编排。 |

## 数据所有权

| 数据 | 存储 | 说明 |
| --- | --- | --- |
| 消息 | MySQL 消息表 | 消息内容、状态、发送者、seq。 |
| 会话 | MySQL 会话表 | 会话归属、未读、置顶、免打扰、清理位点。 |
| 会话 seq | Redis `msg:seq:{conv_id}` | 使用 `INCR` 分配会话内递增序号。 |
| 消息幂等 | Redis `msg:idempotent:{from_uuid}:{device_id}:{client_msg_id}` | TTL 10 分钟，保存首次发送结果。 |
| 消息下行 Outbox | MySQL `outbox_events` | 新消息与 `msg.push` 事件同事务落库。 |

## 暴露的 gRPC 服务

| 服务 | 能力 |
| --- | --- |
| `MsgService` | 发送消息、拉取消息、消息反查、撤回、会话列表、已读、删除会话、更新设置。 |

关键 RPC：

- `SendMessage`：单聊/群聊统一发送入口。
- `PullMessages`：按 `conv_id + anchor_seq + direction` 拉取历史。
- `GetMessagesByIds`：按消息 ID 批量反查。
- `RecallMessage`：撤回消息。
- `GetConversations`：全量或增量获取会话列表。
- `MarkRead`：更新已读位点和未读数。
- `DeleteConversation`：当前用户视角逻辑删除会话。
- `UpdateConversationSettings`：更新免打扰/置顶设置。

## 事件

| 事件类型 | Topic | data payload | 说明 |
| --- | --- | --- | --- |
| `MSG_PUSH` | `msg.push` | `msg.MsgItem` | 新消息下行，由 msg Outbox 经 CDC 生产。 |
| `MSG_RECALL` | `msg.push` | `msg.RecallNotice` | 消息撤回通知。 |
| `MSG_MARK_READ` | `msg.push` | `msg.MarkReadNotice` | 当前账号多端已读同步。 |
| `MSG_READ_RECEIPT` | `msg.push` | 业务约定 payload | 对端已读回执，下游 message-push 支持分发。 |

## 发送消息链路

1. Gateway 从 JWT 和 `X-Device-ID` 注入 `from_uuid` 和 `device_id`。
2. msg usecase 调用权限检查：
   - 单聊校验好友/黑名单关系；
   - 群聊校验群成员、群状态、禁言等权限。
3. message domain 做幂等检查、分配 seq，并在同一 MySQL 事务内写入 `message` 与 `outbox_events(event_type=msg.push)`。
4. conversation domain 更新发送方会话。
5. 单聊更新接收方会话；群聊更新群会话热数据并初始化成员会话行。
6. 消息与 outbox 提交成功即返回发送成功；会话 Upsert 失败只记录 Warn，不回滚消息。
7. Debezium 监听 `outbox_events`，将 `msg.push` protojson 事件投递到 Kafka。

## 非阻断策略

| 步骤 | 失败处理 | 原因 |
| --- | --- | --- |
| 消息落库 | 阻断返回错误 | 消息事实未成立。 |
| 会话 Upsert | Warn，不阻断 | 后续消息或拉取可自愈。 |
| CDC/Kafka 投递 | 不在请求内等待 | 客户端可通过 PullMessages 自愈。 |
| 群成员会话初始化 | Warn，不阻断 | 下次会话更新可补偿。 |

## 不变量

- 消息事实以 MySQL 为准，Redis 只用于 seq 和幂等缓存。
- 发送成功的判定点是“消息落库成功”，不是“下行推送成功”。
- `seq` 只在单个会话内有序，不能跨会话比较。
- `client_msg_id` 必须由客户端在重试时复用。
- 撤回后客户端渲染应优先看 `status`，不要继续展示原 `content`。
