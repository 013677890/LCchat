# Message Kafka 消费程序开发文档

> 文档状态：开发设计稿  
> 目标程序：消费 Kafka `msg.push` 的独立 Message Push-Job  
> 更新时间：2026-04-18

---

## 一、背景与目标

当前消息发送主链路已经具备：

1. `msg-service` 接收发送消息请求；
2. `message.Service.CreateMessage` 完成幂等检查、`conv_id` 计算、`msg_id` 生成、`seq` 分配和消息落库；
3. `conversation.Service` 更新会话列表或群会话热数据；
4. `mq.Producer.Publish` 将 `MsgPushEvent` 写入 Kafka `msg.push`；
5. 客户端可通过拉取消息接口进行最终补偿。

但当前仓库里尚未落地真正消费 `msg.push` 的业务消费者，导致实时下行链路还没有闭环：

```text
msg-service -> Kafka msg.push -> 消费程序待实现 -> connect-service gRPC -> WebSocket 客户端
```

本文档用于指导新增一个独立 Message Push-Job 程序，负责消费 Kafka 中的消息推送事件，并调用 `connect-service` 完成在线设备投递。

---

## 二、现状结论

### 2.1 已有能力

| 能力 | 当前状态 | 关键位置 |
|---|---|---|
| `msg.push` 生产 | 已实现 | `apps/msg/mq/producer.go` |
| 推送事件协议 | 已定义 | `proto/msg/msg_push_event.proto` |
| 通用 Kafka Consumer | 已有基础封装 | `pkg/kafka/consumer.go` |
| connect gRPC 推送入口 | 已实现 | `apps/connect/internal/grpc/server.go`、`proto/connect/connect.proto` |
| connect 本机连接投递 | 已实现 | `apps/connect/internal/manager/connection_manager.go` |
| 独立 Push-Job | 未实现 | 待新增 |
| 用户路由表写入 | 未落地 | 本方案约定由 `connect-service` 负责写入，键为 `user:routing:{uuid}` |
| 用户路由表读取 | 未落地 | 由 Push-Job 的 `RouteRepository` 读取 |
| 群成员扩散实现 | 未完整落地 | 待与 group 能力对接 |

### 2.2 当前架构边界

`connect-service` 的定位是纯通道：

- 负责 WebSocket 连接管理；
- 负责本机在线连接索引；
- 负责接收 gRPC 推送请求；
- 负责把消息入队到本机 WebSocket 连接；
- **负责维护自身所持有连接对应的用户路由表 `user:routing:{uuid}`（写侧）**；
- 不直接消费 Kafka；
- 不做群扩散；
- 不做全局用户路由查询（只写自己、不读他人）；
- 不做消息业务判断。

路由表写入是连接生命周期的一部分，与"在线索引"语义一致：既然 connect 是唯一清楚"本节点持有哪些连接、自己的 gRPC 地址是什么"的服务，就由 connect 在 `OnConnect / OnHeartbeat / OnDisconnect` 三个时机写 Redis；路由表的读取仍归 Push-Job，二者解耦。完整规约见第八章。

因此，Kafka 消费、扩散、路由查询和调用 connect gRPC 应由独立 Push-Job 承担。

---

## 三、目标职责

Message Push-Job 的核心职责如下：

1. 从 Kafka `msg.push` topic 消费 `MsgPushEvent`；
2. 按 `event.type` 解码业务载荷：
   - `MSG_PUSH` -> `MsgItem`；
   - `MSG_RECALL` -> `RecallNotice`；
   - `MSG_MARK_READ` -> `MarkReadNotice`；
3. 按 `event.conv_type` 判断扩散策略：
   - P2P：推送给对端用户；
   - GROUP：查询群成员并按成员扩散；
4. 多端同步：给 `from_uuid` 下除 `event.device_id` 之外的在线设备投递一份；
5. 查询 Redis 用户路由表，定位用户所在 connect 节点；
6. 调用目标 connect 节点 gRPC：
   - 单设备精确投递用 `PushToDevice`；
   - 用户本节点多设备投递用 `PushToUser`；
   - 多用户批量投递可按节点聚合后使用 `BroadcastToUsers`；
7. 记录投递结果、失败原因和关键指标；
8. 保证消费程序可优雅启动、关闭、重平衡和扩容。

---

## 四、非目标

第一阶段不做以下事项：

1. 不让 `connect-service` 直接消费 Kafka；
2. 不改变 `msg-service` 的发送成功语义；
3. 不要求消息实时推送强必达；
4. 不在 Push-Job 中落消息表；
5. 不在 Push-Job 中修改会话未读数；
6. 不实现复杂离线推送；
7. 不新增与当前业务无关的通用调度框架。

消息持久化仍以 `msg-service` 为准，Push-Job 只负责在线下行投递。投递失败时，客户端仍可通过 PullMessages 自愈。

---

## 五、端到端链路

### 5.1 新消息发送链路

```text
客户端 / gateway
  -> msg-service SendMessage
  -> message.Service.CreateMessage
  -> conversation.Service.UpsertForMessage / UpsertGroupConv
  -> Kafka topic: msg.push, key = conv_id, value = MsgPushEvent
  -> Message Push-Job 消费
  -> 查询目标用户 / 群成员 / 在线路由
  -> 调用 connect-service PushToUser / PushToDevice / BroadcastToUsers
  -> ConnectionManager.SendToUser / SendToDevice
  -> WebSocket 客户端
```

### 5.2 撤回通知链路

```text
msg-service RecallMessage
  -> DB 更新消息状态
  -> Kafka msg.push 写入 MsgPushEvent{type="MSG_RECALL"}
  -> Push-Job 解码 RecallNotice
  -> 按会话成员扩散
  -> connect-service 下行 MSG_RECALL
```

### 5.3 已读通知链路

```text
msg-service MarkRead
  -> 会话 read_seq 单调更新
  -> Kafka msg.push 写入 MsgPushEvent{type="MSG_MARK_READ"}
  -> Push-Job 解码 MarkReadNotice
  -> 主要投递给同用户其他在线设备
  -> connect-service 下行 MSG_MARK_READ
```

---

## 六、事件协议

### 6.1 Kafka topic 与 key

| 项 | 约定 |
|---|---|
| Topic | `msg.push` |
| 配置字段 | `KafkaConfig.MsgPushTopic` |
| 环境变量 | `KAFKA_MSG_PUSH_TOPIC` |
| 默认值 | `msg.push` |
| Key | `conv_id` |
| Value | `MsgPushEvent` Protobuf bytes |

Kafka key 使用 `conv_id`，目的是保证同一会话内事件进入同一 Partition，从而在单消费者组内尽量保持会话内顺序。

### 6.2 `MsgPushEvent` 字段语义

| 字段 | 语义 |
|---|---|
| `receiver_uuid` | P2P 场景为对端用户 UUID；GROUP 场景为群 UUID |
| `device_id` | 发送方当前设备 ID，多端同步时用于排除本设备 |
| `type` | `MSG_PUSH` / `MSG_RECALL` / `MSG_MARK_READ` |
| `conv_type` | `CONV_TYPE_P2P` / `CONV_TYPE_GROUP`；**`MSG_MARK_READ` 允许为空**（详见 12.2 校验规则） |
| `data` | 按 `type` 序列化后的业务 Protobuf bytes |
| `trace_id` | 链路追踪 ID，当前生产侧字段存在但未完全填充 |
| `server_ts` | 服务端事件生成时间，Unix 毫秒 |
| `from_uuid` | 发送方 UUID，多端同步使用 |
| `seq` | **新增字段**，消息序列号。`MSG_PUSH` 填 `MsgItem.seq`；`MSG_RECALL` / `MSG_MARK_READ` 填 0。Push-Job 直接透传到 `MessageEnvelope.seq` |

### 6.3 `data` 解码规则

| `type` | `data` 类型 | 推送含义 |
|---|---|---|
| `MSG_PUSH` | `MsgItem` | 新消息下行 |
| `MSG_RECALL` | `RecallNotice` | 撤回通知 |
| `MSG_MARK_READ` | `MarkReadNotice` | 已读同步通知 |

### 6.4 协议改动清单（本方案新增）

本方案要求对 `proto/msg/msg_push_event.proto` 做一次兼容式扩展：

```protobuf
message MsgPushEvent {
  string receiver_uuid = 1;
  string device_id     = 2;
  string type          = 3;
  ConvType conv_type   = 4;
  bytes data           = 5;
  string trace_id      = 6;
  int64 server_ts      = 7;
  string from_uuid     = 8;
  int64 seq            = 9; // 新增：envelope.seq 透传字段
}
```

字段语义与填充规则：

| `type` | `seq` 取值 | 说明 |
|---|---|---|
| `MSG_PUSH` | `MsgItem.seq` | 与业务 payload 中的 seq 冗余但必填，便于消费者不解包即可获得顺序键 |
| `MSG_RECALL` | 0 | 撤回无新 seq |
| `MSG_MARK_READ` | 0 | 已读无新 seq |

改动影响面：

1. `proto/msg/msg_push_event.proto` 新增 field 9；
2. 生成代码 `apps/msg/pb/` 同步刷新；
3. `apps/msg/internal/usecase/send_message_workflow.go` 构造 `MsgPushEvent` 时额外写入 `Seq: msg.Seq`；
4. `apps/msg/internal/usecase/recall_message_workflow.go`、`mark_read_workflow.go` 不必改动（默认 0）；
5. Push-Job 构造 `MessageEnvelope` 时直接 `envelope.Seq = event.Seq`，无需解包 `MsgItem`；
6. protobuf 字段 9 为新增 tag，旧消费者（暂无）收到新事件时字段未知会被忽略，向前兼容。

**重要约束**：ConvType 的填写由**生产侧**负责。`SendMessageWorkflow` / `RecallMessageWorkflow` 写 Kafka 前必须显式填 `CONV_TYPE_P2P` 或 `CONV_TYPE_GROUP`；`MarkReadWorkflow` 可以不填（默认 `CONV_TYPE_UNSPECIFIED`）。禁止 Push-Job 用 `conv_id` 字符串前缀反推 ConvType。

---

## 七、扩散策略

### 7.1 P2P 消息

P2P 新消息投递目标包含两类：

1. 接收方 `receiver_uuid` 的在线设备；
2. 发送方 `from_uuid` 的其他在线设备，用于多端同步，但必须排除 `device_id` 对应的当前发送设备。

建议处理流程：

```text
event.conv_type == CONV_TYPE_P2P
  -> targets = [receiver_uuid]
  -> self_sync_targets = from_uuid excluding event.device_id
  -> 查询 targets 和 self_sync_targets 路由
  -> 分节点调用 connect gRPC
```

注意：

- 接收方可调用 `PushToUser`；
- 发送方多端同步如需排除当前设备，不能简单调用 `PushToUser`，应查询设备级路由后对非当前设备调用 `PushToDevice`，或在 connect 侧新增支持排除设备的接口；
- 第一阶段可优先实现接收方 `PushToUser` 和发送方其他设备 `PushToDevice`。

### 7.2 GROUP 消息

GROUP 新消息的 `receiver_uuid` 表示群 UUID。Push-Job 需要查询群成员，拆分为多个用户再投递。

建议处理流程：

```text
event.conv_type == CONV_TYPE_GROUP
  -> group_uuid = event.receiver_uuid
  -> 查询群成员列表
  -> 过滤不应接收的成员
  -> 对每个成员查询路由
  -> 按 connect 节点聚合
  -> 调用 BroadcastToUsers 或多次 PushToUser
  -> 对发送者当前设备做排除
```

第一阶段可先支持小规模群的直接 fan-out（扇出），后续再按群规模做优化：

| 群规模 | 建议策略 |
|---|---|
| 小群 | 直接查询全部成员并投递 |
| 中群 | 分批查询路由，分批调用 connect |
| 大群 | 后续引入批量任务、限流和分片投递 |

### 7.3 撤回事件

撤回通知要投递到会话内所有相关在线设备：

- P2P：双方在线设备；
- GROUP：群内在线成员；
- 发送者当前设备是否需要排除，由客户端交互决定。建议不排除，因为撤回操作发起端也需要本地状态确认。

### 7.4 已读事件

已读通知通常用于同用户多端同步：

- 目标应以 `from_uuid` 的其他在线设备为主；
- 如果后续需要对对端展示“已读”，再扩展对会话对端的通知；
- 第一阶段建议仅做同用户多端同步，避免引入额外社交语义。

---

## 八、路由模型

### 8.1 职责划分

用户路由表 `user:routing:{uuid}` 采用「**connect 直写 + Push-Job 只读**」的分工：

| 角色 | 行为 |
|---|---|
| `connect-service`（写方） | 在 `OnConnect / OnHeartbeat / OnDisconnect` 三个时机维护本节点所持连接的路由记录 |
| Push-Job（读方） | 通过 `RouteRepository` 批量读取，不持有写权 |
| `user-service` | 不参与路由表维护，只维护自己的在线状态视图（见 8.4.1） |

选择 connect 直写的理由：

1. 只有 connect 真正知道「本节点当前持有哪些连接」以及「自己对外可达的 gRPC 地址」；
2. 若由 user-service 或其它服务间接维护，必然需要跨服务把"grpc_addr"回传，增加不必要的耦合；
3. 写路由在语义上与 connect 已经维护的本机连接索引（`ConnectionManager`）一一对应，属于"在线索引的持久化投影"。

### 8.2 路由表数据结构

采用 Redis Hash：

```text
key   : user:routing:{user_uuid}
field : {device_id}
value : {connect_grpc_addr}|{active_unix_ms}
```

示例：

```text
HSET user:routing:u_001 d_001 "10.0.1.12:9091|1713420000000"
HSET user:routing:u_001 d_002 "10.0.1.13:9091|1713420001000"
EXPIRE user:routing:u_001 180
```

优点：

- 可按用户一次取出所有在线设备；
- 可按设备精确排除发送方当前设备；
- 可按 connect 地址聚合后减少 gRPC 调用；
- `active_ts` 内嵌到 value，Push-Job 只依赖一类主索引完成在线候选过滤与节点定位；
- TTL 可降低脏路由长期存在的风险（见 8.2.3）。

#### 8.2.1 connect 自身 gRPC 地址的获取

connect 实例启动时必须知道自己对外可达的 gRPC 地址（容器内监听地址可能与对外地址不同）。优先级从高到低：

1. 显式配置环境变量 `CONNECT_SELF_GRPC_ADDR`（推荐，k8s/compose 场景由编排注入，例如 `10.0.1.12:9091` 或 `connect-0.connect.default.svc.cluster.local:9091`）；
2. 若未显式配置，回退到 `CONNECT_GRPC_ADDR` 拼接 `POD_IP` / `HOST_IP`；
3. 若仍无法确定，启动失败并告警，禁止用 `0.0.0.0:9091` 这种无法被其它节点回拨的地址写入路由。

#### 8.2.2 connect 写路由时机

| 时机 | Redis 操作 | 备注 |
|---|---|---|
| `OnConnect` | `HSET user:routing:{uuid} {device_id} "{addr}|{now_ms}"` + `EXPIRE user:routing:{uuid} TTL` | 用 Pipeline 或 Lua 保证 HSET 与 EXPIRE 原子 |
| `OnHeartbeat` | 刷新 `active_ts` 与 TTL | **经 `pkg/deviceactive.Syncer` 节流**，避免每心跳都写 Redis |
| `OnDisconnect` | `HDEL user:routing:{uuid} {device_id}`；若 Hash 为空则 `DEL user:routing:{uuid}` | 推荐用 Lua 保证「HDEL + 空 Hash 清理」原子 |

心跳刷新节流：复用现网已有的 `pkg/deviceactive/cache.go`（见 `DefaultUpdateInterval = 3 min` + `DefaultFlushInterval = 1 min`），由后台批量刷 Redis，而不是在每个 WebSocket 心跳直接调 Redis。

#### 8.2.3 TTL 与心跳周期的关系

- **TTL 必须 > 心跳刷新周期 × 容忍倍数**，否则长期在线用户的路由会被误删；
- 当前心跳相关默认值：`UpdateInterval=3min`、`FlushInterval=1min`；
- 推荐 `MESSAGE_PUSH_ROUTE_TTL_SECONDS=180`（3 倍 FlushInterval）；
- 若后续把心跳调密（如 `UpdateInterval=30s`），需同步评估 TTL；
- 反之，异常断连（客户端 crash / 网络中断）未触发 `OnDisconnect` 时，TTL 到期后脏路由被自动回收。

#### 8.2.4 connect 启动/关闭时的路由清理

- **启动时**：不主动扫全表清理（代价过大），依赖 TTL 自愈；
- **优雅关闭时**：遍历 `ConnectionManager` 在线连接并批量 `HDEL`（与 `ConnectionManager.Shutdown` 同阶段），清理掉本节点留下的路由；
- **崩溃退出**：走 TTL 自愈路径。

### 8.3 路由读取接口

Push-Job 内建议抽象 `RouteRepository`：

```go
type DeviceRoute struct {
    UserUUID        string
    DeviceID        string
    ConnectGRPCAddr string
    LastActiveMs    int64
}

type RouteRepository interface {
    ListUserRoutes(ctx context.Context, userUUID string) ([]DeviceRoute, error)
    ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]DeviceRoute, error)
}
```

说明：

- 这是 Push-Job 的内部接口，不建议直接污染 connect 或 msg 的 domain；
- Redis 不可用时，Push-Job 应记录降级日志并跳过实时投递；
- 不应因为路由查询失败反向影响 `msg-service` 的发送成功结果。

### 8.4 在线状态来源与推送索引决策

当前系统里，`user-service` 已经维护了“共享在线状态视图”，核心由两部分组成：

1. Redis 设备活跃集合 `user:devices:active:{user_uuid}`；
2. 设备会话表中的设备状态 `status`。

其中展示型在线状态的现有判定逻辑是：

```text
设备在线 = DB status = Online
       且 Redis active_ts 在在线窗口内
```

这套逻辑适合：

- 好友在线状态展示；
- 设备列表展示；
- 小批量用户在线状态查询；
- 最近活跃时间展示。

但对 Push-Job 的群消息大扇出场景，不建议把“`user-service` 在线状态查询接口”作为主读取路径，原因如下：

- 该路径依赖 DB 设备状态查询，更适合展示型查询，不适合高频推送型 fan-out；
- 当前批量在线查询接口更偏向用户级状态，不直接返回设备级可投递路由；
- 群消息推送最终仍然需要知道“哪些设备在线”以及“这些设备归哪个 connect 节点管理”；
- 大群消息场景下，DB 不应成为实时推送主链路的高频依赖。

因此本文明确区分两类在线语义：

#### 8.4.1 展示型在线状态

定义：

```text
展示型在线状态 = DB device status + Redis active
```

用途：

- `GetOnlineStatus`
- `BatchGetOnlineStatus`
- 设备管理页
- 好友在线展示

#### 8.4.2 推送型在线候选状态

定义：

```text
推送型在线候选状态 = Redis routing 存在
                  且 route 中的 active_ts 未过期
                  且最终由 connect 本机连接管理确认
```

用途：

- Push-Job P2P / GROUP 实时投递
- 多端同步
- 撤回 / 已读实时通知

也就是说，Push-Job 的主索引应当是“设备级 routing 表”，而不是 user-service 的展示型在线状态接口。

### 8.5 不采用在线 bitmap 作为推送主索引

本方案明确不新增“用户在线状态 bitmap”作为 msg 推送服务的主在线结构，原因如下：

1. 当前项目主标识为 UUID，bitmap 对连续整数 ID 更友好，引入 bitmap 需要额外维护 `uuid -> bit offset` 映射；
2. bitmap 只能较粗粒度表达“用户是否在线”，不适合表达多设备在线状态；
3. bitmap 无法直接表达设备所属 connect 节点，仍需额外查询 routing；
4. bitmap 不具备设备级 TTL 和最近活跃时间语义，脏状态自愈能力较弱；
5. 对 Push-Job 来说，真正关键的不是“用户是否在线”，而是“设备是否可投递”与“设备归属哪个 connect 节点”；
6. 引入 bitmap 会增加 active、routing、bitmap 三套状态之间的一致性维护复杂度。

因此，当前推荐方案为：

```text
群成员缓存 + 设备级 routing 表 + route 内 active_ts / TTL
```

而不是：

```text
群成员缓存 + 用户在线 bitmap + 二次查询 routing
```

如果后续压测证明超大群场景下“用户在线粗筛”本身成为瓶颈，可再评估 bitmap 作为辅助过滤索引，但不作为权威在线来源，也不作为精准推送依据。

### 8.6 推送型读取链路建议

群消息实时投递时，推荐读取链路如下：

```text
group:members:{group_uuid}
  -> 取群成员
  -> 批量读取 user:routing:{user_uuid}
  -> 过滤 active_ts 过期或 route 缺失的设备
  -> 按 connect_grpc_addr 聚合
  -> 调用对应 connect 节点 PushToUser / PushToDevice / BroadcastToUsers
```

如果早期实现希望复用现有 active 集合作为辅助过滤，可采用：

```text
group members
  -> Redis active 过滤近期活跃用户/设备
  -> Redis routing 获取 connect 节点
  -> connect 本机内存最终确认
```

但从长期维护和读性能看，更推荐把 `active_ts` 直接合并进设备级 routing value，使 Push-Job 只依赖一类主索引完成在线候选过滤与节点定位。

---

## 九、connect gRPC 调用策略

### 9.1 可用接口

`connect-service` 已提供：

| 接口 | 用途 |
|---|---|
| `PushToDevice` | 向指定用户的指定设备投递 |
| `PushToUser` | 向某个用户在当前 connect 节点上的所有在线设备投递 |
| `BroadcastToUsers` | 向多个用户在当前 connect 节点上的在线设备广播相同消息 |
| `KickConnection` | 主动断开连接，Push-Job 不使用 |

### 9.2 MessageEnvelope 构造

Push-Job 消费 `MsgPushEvent` 后，应构造 connect 的 `MessageEnvelope`：

| envelope 字段 | 来源 | 说明 |
|---|---|---|
| `type` | `event.type` | 直接透传 |
| `data` | `event.data` | 保持原始业务 bytes，不二次改写 |
| `seq` | `event.seq` | **直接透传顶层 seq，不再解包 `MsgItem`** |
| `server_ts` | `event.server_ts` | 直接透传 |
| `trace_id` | `event.trace_id` | 直接透传（若为空则由 connect 侧保持空） |
| `ack_required` | 固定 `false` | 第一阶段不启用 ack |

原则：Push-Job 不二次改写业务载荷，只负责把 `MsgPushEvent.data` 原样装入 connect 下行信封，并把顶层元数据（`seq / server_ts / trace_id`）映射到 envelope 顶层字段。

### 9.3 gRPC Client 管理

建议新增 `ConnectClientManager`：

- 按 `connect_grpc_addr` 缓存 gRPC 连接；
- 支持连接复用；
- 支持超时控制；
- 支持关闭时统一释放；
- 对失败连接做短时间熔断，避免持续打爆异常节点。

建议默认调用超时（connect 侧 `Enqueue` 为非阻塞入队，正常路径亚毫秒级；下列数值按"覆盖 P99 网络抖动但能快速暴露节点 hang"的思路给出）：

| 场景 | 超时 |
|---|---|
| 单设备投递 `PushToDevice` | 100ms |
| 单用户投递 `PushToUser` | 150ms |
| 批量广播 `BroadcastToUsers` | 500ms |

实际 connect 节点出现稳定抖动再按需放宽，不建议一开始就给宽裕阈值，以免掩盖"节点 hang"这种需要重点关注的故障。

---

## 十、消费语义与 offset 提交

### 10.1 不复用 `pkg/kafka/consumer.go`

当前 `pkg/kafka/consumer.go` 的实现仅适合 Redis 重试类任务：

1. `FetchMessage` 失败 `continue` 直接空转，缺少退避；
2. `handler` 返回的错误被丢弃，无法驱动 commit 决策；
3. handler 与 `CommitMessages` 共用同一个 ctx — 优雅关闭时 ctx 已 cancel，**最后一批消息的 offset 提交会失败，导致重启后重复消费**。

这些对 `msg.push` 的实时推送场景不可接受，因此 **Push-Job 不复用该封装**，而是在 `apps/message-push/internal/consumer/push_consumer.go` 内封装一个独立的消费循环，规格如下：

```go
// 伪代码示意，实际以 apps/message-push/internal/consumer/push_consumer.go 为准
for {
    // 1. 消费退出信号：消费循环只受启动时传入的 ctx 控制
    if ctx.Err() != nil {
        return ctx.Err()
    }

    // 2. 拉消息：FetchMessage 出错做指数退避（100ms → 1s 封顶）
    msg, err := reader.FetchMessage(ctx)
    if err != nil {
        if errors.Is(err, context.Canceled) {
            return err
        }
        backoff.Wait()
        continue
    }
    backoff.Reset()

    // 3. 处理消息：由 handler 返回值决定是否 commit
    handleCtx, cancel := context.WithTimeout(ctx, handleTimeout)
    shouldCommit := handler(handleCtx, msg)
    cancel()

    // 4. 提交 offset 使用独立 context，避免关闭时丢 commit
    if shouldCommit {
        commitCtx, commitCancel := context.WithTimeout(context.Background(), 3*time.Second)
        _ = reader.CommitMessages(commitCtx, msg)
        commitCancel()
    }
}
```

要点：

- `handler` 返回 `bool` 表示「是否可以提交 offset」；
- commit 使用 `context.Background()` 派生的带超时 ctx，关闭链路时也能完成最后一次提交；
- `FetchMessage` 错误做指数退避，避免 broker 抖动时 CPU 空转；
- 该实现是 Push-Job 的**内部封装**，不污染 `pkg/kafka`。

### 10.2 第一阶段 commit 策略

采用「有限重试 + 失败提交 + 指标告警」：

| 单条消息场景 | handler 返回 | 备注 |
|---|---|---|
| 反序列化失败 | `true`（提交） | 记 `decode_failed_total` |
| 协议类型未知 | `true`（提交） | 记 Warn |
| `conv_type` 缺失且 `type ≠ MSG_MARK_READ` | `true`（提交） | 属于生产侧违规，记 `decode_failed_total` |
| 路由为空 | `true`（提交） | 视为离线，记 `route_miss_total` |
| 路由存在但 `delivered=false` | `true`（提交） | 视为路由过期，记 `route_stale_total`，不重查 |
| Redis 路由查询失败 | 有限重试（2 次）后仍失败 → `true`（提交） | 记 `consume_failed_total` |
| connect 调用失败 | 有限重试（2 次）后仍失败 → `true`（提交） | 记 `connect_failed_total` |
| ctx canceled（关闭） | `false`（不提交） | 下次启动从上次提交点继续 |

原因：

- 当前产品语义是消息已落库即可发送成功；
- 实时推送不是强必达；
- 客户端 PullMessages 可以补偿；
- 不应让坏消息或异常节点阻塞整个 partition。

### 10.3 后续增强语义

如果后续要求推送更可靠，可演进为：

1. 新增 DLQ topic，例如 `msg.push.dlq`；
2. 新增 retry topic，例如 `msg.push.retry`；
3. 对 connect 暂时失败事件做延迟重试；
4. 对协议坏消息直接进入 DLQ；
5. 将消费失败与 `trace_id`、`msg_id`、`conv_id` 关联报警。

---

## 十一、建议目录结构

建议新增独立程序目录：

```text
apps/message-push/
  cmd/
    main.go
    app.go
    providers.go
    wire.go
    wire_gen.go
  internal/
    consumer/
      push_consumer.go
      event_handler.go
    route/
      repository.go
      redis_repository.go
    group/
      repository.go
      client_repository.go
    connectcli/
      client_manager.go
      sender.go
    fanout/
      dispatcher.go
      target_resolver.go
    metrics/
      metrics.go
  README.md（可选，若已有 doc 则不强制）
```

如果希望命名更短，也可使用：

```text
apps/push-job/
```

本文推荐 `apps/message-push`，原因：

- 语义明确，专指消息下行；
- 避免未来系统内出现多个 push job 时命名冲突；
- 与 `apps/msg`、`apps/connect` 保持边界清晰。

---

## 十二、核心模块设计

### 12.1 `PushConsumer`

职责：

- 在 `apps/message-push/internal/consumer/push_consumer.go` 内封装一套独立消费循环（**不复用 `pkg/kafka.Consumer`**，原因见 10.1）；
- 基于 `github.com/segmentio/kafka-go` 的 `kafka.Reader` 直接实现 FetchMessage + 指数退避 + 独立 commit context；
- 绑定 topic、groupID；
- 按 handler 返回值决定是否提交 offset；
- 优雅停止：关闭 ctx 后允许当前消息完成处理与 commit，再关闭 reader；
- 将消息 value 交给 `EventHandler`。

建议消费组：

```text
message-push-consumer-group
```

需要新增环境变量，避免复用 `KAFKA_RETRY_GROUP_ID`：

```text
KAFKA_MSG_PUSH_GROUP_ID=message-push-consumer-group
```

### 12.2 `EventHandler`

职责：

- 反序列化 `MsgPushEvent`；
- 校验必要字段；
- 按 `type` 解码 `data`；
- 构造下行信封；
- 调用 fanout 层。

校验规则：

| 字段 | 要求 |
|---|---|
| `type` | 必须为已知类型（`MSG_PUSH` / `MSG_RECALL` / `MSG_MARK_READ`） |
| `conv_type` | **`type = MSG_PUSH / MSG_RECALL` 时必须为 `CONV_TYPE_P2P` 或 `CONV_TYPE_GROUP`**；**`type = MSG_MARK_READ` 允许为空**（已读只推给 `from_uuid` 其他设备，不关心会话扩散语义） |
| `data` | 不可为空 |
| `receiver_uuid` | `MSG_PUSH / MSG_RECALL` 必填；`MSG_MARK_READ` 必填（定位目标用户 = `from_uuid` 自身） |
| `from_uuid` | 三种 `type` 均必填 |
| `seq` | `MSG_PUSH` 必须 > 0；`MSG_RECALL` / `MSG_MARK_READ` 固定为 0 |

校验失败统一按「坏消息处理」（记 `decode_failed_total`，提交 offset）。

### 12.3 `TargetResolver`

职责：

- 根据 `conv_type` 和 `type` 计算目标用户；
- P2P 场景返回对端 + 发送方其他设备；
- GROUP 场景查询群成员；
- 生成设备排除规则。

建议输出结构：

```go
type DeliveryTarget struct {
    UserUUID          string
    ExcludeDeviceIDs map[string]struct{}
}
```

### 12.4 `Dispatcher`

职责：

- 查询路由；
- 按 connect 节点聚合；
- 选择 `PushToDevice` / `PushToUser` / `BroadcastToUsers`；
- 记录成功、失败、离线数量。

聚合示例：

```text
user_uuid -> routes
routes -> connect_grpc_addr -> users/devices
connect_grpc_addr -> gRPC call
```

### 12.5 `RouteRepository`

职责：

- 从 Redis 读取 `user:routing:{uuid}`；
- 解析设备和 connect 地址；
- 过滤过期路由；
- 支持批量读取。

### 12.6 `GroupRepository`

职责：

- 查询群成员列表；
- 支持后续过滤禁言、退群、黑名单等业务规则；
- 第一阶段可先接入既有 group 能力或预留接口。

注意：当前仓库还未完成 group-service 拆分落地，第一阶段若没有群成员来源，可先把 GROUP 消费记录为“暂不支持实时群扩散”，但不能误投递。

---

## 十三、错误处理策略

| 错误类型 | 处理方式 | 是否提交 offset |
|---|---|---|
| Kafka 读取失败 | consumer 内指数退避重试（100ms → 1s 封顶） | 不涉及 |
| `MsgPushEvent` 反序列化失败 | 记录错误与原始长度 | 是 |
| `type` 未知 | 记录 Warn | 是 |
| `conv_type` 对 `MSG_PUSH / MSG_RECALL` 缺失 | 按坏消息记录，属生产侧违规 | 是 |
| `data` 解码失败 | 记录 Warn | 是 |
| 路由为空（用户无任何在线设备） | 视为离线，记 `route_miss_total` | 是 |
| 路由命中但 `PushToDevice / PushToUser` 返回 `delivered=false / delivered_count=0` | **视为路由过期**，记 `route_stale_total`，**不重查路由、不重试**，由客户端 PullMessages 补偿 | 是 |
| Redis 路由查询失败 | 记录 Warn，有限重试（2 次） | 是 |
| 群成员查询失败 | 记录 Warn，有限重试（2 次） | 是 |
| connect 调用超时/RPC error | 按节点有限重试（2 次） | 是 |
| context canceled | 停止消费，当前消息不提交 | 否，由 shutdown 决定 |

第一阶段不因为单条消息失败阻塞 partition。所有失败必须有可观测指标。

---

## 十四、日志与指标

### 14.1 日志原则

- 日志使用中文消息；
- 字段 key 使用英文；
- 不记录消息正文 `content`；
- 不记录 token、密码、验证码；
- 降级路径使用 `Warn`；
- 单条成功投递默认不打 Info，避免高 QPS 下日志爆炸。

### 14.2 推荐日志字段

| 字段 | 说明 |
|---|---|
| `trace_id` | 链路追踪 ID |
| `event_type` | `MSG_PUSH` / `MSG_RECALL` / `MSG_MARK_READ` |
| `conv_type` | 会话类型 |
| `conv_id` | 可从 `MsgItem` / Notice 中取 |
| `msg_id` | 新消息或撤回消息 ID |
| `from_uuid` | 发送方 |
| `receiver_uuid` | 接收方用户或群 |
| `connect_addr` | 目标 connect 节点 |
| `delivered_count` | 成功入队设备数 |
| `error` | 错误摘要 |

### 14.3 推荐指标

| 指标 | 类型 | 说明 |
|---|---|---|
| `message_push_consume_total` | Counter | 消费事件数 |
| `message_push_consume_failed_total` | Counter | 消费处理失败数 |
| `message_push_decode_failed_total` | Counter | 协议解码失败数 |
| `message_push_route_miss_total` | Counter | 路由为空次数（用户无任何在线设备） |
| `message_push_route_stale_total` | Counter | 路由存在但 connect 返回 delivered=0，视为路由脏数据 |
| `message_push_connect_call_total` | Counter | connect 调用数 |
| `message_push_connect_failed_total` | Counter | connect 调用失败数 |
| `message_push_delivered_devices_total` | Counter | 成功入队设备数 |
| `message_push_handle_duration_ms` | Histogram | 单条事件处理耗时 |
| `message_push_lag` | Gauge | Kafka 消费延迟 |

---

## 十五、配置设计

### 15.1 复用配置

复用现有 `config.KafkaConfig`：

| 字段 | 用途 |
|---|---|
| `Brokers` | Kafka broker 列表 |
| `MsgPushTopic` | 消费 topic |
| `ConsumerConfig` | consumer 基础参数 |

### 15.2 建议新增配置

当前 `KafkaConsumerConfig.GroupID` 默认来自 `KAFKA_RETRY_GROUP_ID`，这是 Redis retry 专用语义。Message Push-Job 不应复用该环境变量。

建议新增：

```text
# Kafka 消费组
KAFKA_MSG_PUSH_GROUP_ID=message-push-consumer-group

# 并发与处理
MESSAGE_PUSH_WORKER_COUNT=8
MESSAGE_PUSH_HANDLE_TIMEOUT_MS=2000
MESSAGE_PUSH_RETRY_TIMES=2

# connect gRPC 调用超时（见 9.3）
MESSAGE_PUSH_CONNECT_TIMEOUT_DEVICE_MS=100
MESSAGE_PUSH_CONNECT_TIMEOUT_USER_MS=150
MESSAGE_PUSH_CONNECT_TIMEOUT_BROADCAST_MS=500

# 路由表 TTL（与 connect 心跳周期匹配，见 8.2.3）
MESSAGE_PUSH_ROUTE_TTL_SECONDS=180
```

同时 `connect-service` 侧新增：

```text
# connect 自身对外可达的 gRPC 地址（写入路由表 value），见 8.2.1
CONNECT_SELF_GRPC_ADDR=10.0.1.12:9091
```

如短期不修改全局配置结构，也可在 `apps/message-push/cmd/providers.go` 和 `apps/connect/cmd/providers.go` 内独立读取这些变量。

---

## 十六、启动与关闭

### 16.1 启动流程

```text
main.go
  -> initializeMessagePushApp
  -> 初始化 logger
  -> 初始化 Redis
  -> 初始化 Kafka consumer
  -> 初始化 RouteRepository
  -> 初始化 GroupRepository
  -> 初始化 ConnectClientManager
  -> 初始化 EventHandler / Dispatcher
  -> app.Run(ctx)
```

### 16.2 关闭流程

```text
收到 SIGINT / SIGTERM
  -> cancel context
  -> 停止 Kafka consumer 拉取
  -> 等待正在处理的消息完成或超时
  -> 关闭 Kafka reader
  -> 关闭 connect gRPC 连接池
  -> 关闭 Redis
  -> flush logger
```

要求：

- 关闭过程必须有总超时；
- 不应在 shutdown 时启动新投递；
- 已经进入处理的消息尽量完成后再退出；
- 超时后允许退出，依赖 Kafka 重平衡或补偿机制。

---

## 十七、并发模型

推荐第一阶段采用“单 consumer loop + worker pool”模型：

```text
Kafka FetchMessage
  -> 投递到 bounded channel
  -> worker pool 并发处理
  -> 处理完成后提交 offset
```

但要注意会话内顺序：

- Kafka 已按 `conv_id` 分区；
- 如果同一 partition 内再无序并发处理，可能破坏同会话顺序；
- 第一阶段可先单 goroutine 顺序处理，保证简单正确；
- 后续如需并发，应按 `conv_id` 做一致性哈希分发到固定 worker。

推荐演进：

| 阶段 | 模型 | 说明 |
|---|---|---|
| 第一阶段 | Kafka reader 顺序处理 | 简单，保证 partition 内顺序；单消息处理完立即 commit |
| 第二阶段 | 按 `conv_id` 分 worker + per-partition offset tracker | 提升吞吐，同时保证同会话顺序与不跳 offset |
| 第三阶段 | 多实例 + 多 partition | 水平扩容 |

**第二阶段 offset 提交注意事项**：

并发 worker 下，若每个 worker 处理完就各自 commit，会出现"高 offset 先提交、低 offset 尚未完成"的情况；一旦进程崩溃，Kafka 会以高水位记录，**中间尚未完成的消息会被静默跳过**。

为避免该问题，采用「per-partition offset tracker」：

```text
每个 partition 维护一个有序窗口 [committed_offset, ..., inflight_max]
   ↓
worker 完成消息后，向 tracker 标记该 offset "done"
   ↓
tracker 只提交「连续已完成」的最大 offset
  （遇到中间空洞则等待，不越过空洞提交）
```

第一阶段因顺序处理可天然保证 commit 序，不需要 tracker；进入第二阶段时，再补这一层。

---

## 十八、测试计划

### 18.1 单元测试

| 模块 | 测试点 |
|---|---|
| `EventHandler` | 反序列化、类型分发、坏消息处理 |
| `TargetResolver` | P2P、GROUP、撤回、已读目标计算 |
| `RouteRepository` | Redis hash 解析、过期过滤、空路由 |
| `Dispatcher` | 按 connect 地址聚合、排除设备、失败重试 |
| `ConnectClientManager` | 连接复用、关闭、超时 |

### 18.2 集成测试

1. 启动 Kafka、Redis、connect-service；
2. 写入一条 `MSG_PUSH` 事件；
3. Push-Job 消费事件；
4. 模拟 Redis 路由指向 connect；
5. 验证 connect gRPC 被调用；
6. 验证 WebSocket 客户端收到 `MessageEnvelope`。

### 18.3 回归测试

必须验证：

- `msg-service` 原发送接口不受影响；
- Kafka 投递失败仍不影响发送成功；
- connect 不直接依赖 Kafka；
- Push-Job 停止时不影响用户拉消息补偿。

---

## 十九、分阶段落地计划

### 阶段 1：最小闭环

目标：P2P 新消息可实时推送给对端在线设备。

范围：

1. **协议改动**：`proto/msg/msg_push_event.proto` 新增 `seq` 字段并刷新生成代码；
2. **生产侧**：`SendMessageWorkflow` 构造 `MsgPushEvent` 时填 `Seq = msg.Seq`；
3. **connect 侧**：在 `OnConnect / OnHeartbeat（经 deviceactive.Syncer 节流）/ OnDisconnect` 写 `user:routing:{uuid}`；`CONNECT_SELF_GRPC_ADDR` 启动校验；优雅关闭时清理本节点路由；
4. 新增 `apps/message-push` 程序骨架；
5. 在 `apps/message-push/internal/consumer` 内自研 Kafka 消费循环（不复用 `pkg/kafka.Consumer`）；
6. 支持 `MSG_PUSH` + `CONV_TYPE_P2P`；
7. 读取用户路由（对端 + `PushToUser`）；
8. 添加基础日志和指标（含 `route_miss_total` / `route_stale_total`）。

验收：

- 发送 P2P 消息后，对端在线 WebSocket 能收到；
- 对端离线时不报错、不阻塞；
- 路由表在 connect 上下线时正确写入/清理；TTL 到期可自愈；
- Push-Job 优雅关闭时无"末尾一批丢 commit"现象；
- 无新增 lint 错误。

### 阶段 2：多端同步与撤回

目标：发送方其他设备、撤回通知可实时同步。

范围：

1. 支持发送方其他设备投递；
2. 支持 `MSG_RECALL`；
3. 支持设备排除；
4. 完善 `PushToDevice` 使用路径。

验收：

- 同账号多设备登录时，除发送设备外其他设备能收到自己发出的消息；
- 撤回后相关在线设备能收到撤回通知。

### 阶段 3：已读同步

目标：同用户多端已读状态实时同步。

范围：

1. 支持 `MSG_MARK_READ`；
2. 投递给同用户其他在线设备；
3. 增加已读同步指标。

验收：

- 一个设备标记已读后，其他在线设备红点可清除。

### 阶段 4：群消息扩散

目标：群消息在线成员实时推送。

范围：

1. 接入群成员查询；
2. 支持 `CONV_TYPE_GROUP`；
3. 按 connect 节点聚合调用；
4. 增加限流和批量投递。

验收：

- 小群消息可实时推送给在线群成员；
- 发送者当前设备按规则排除或同步；
- 群成员查询失败不会阻塞消费进程。

### 阶段 5：可靠性增强

目标：提升可观测性和失败处理能力。

范围：

1. DLQ；
2. retry topic；
3. Kafka lag 监控；
4. connect 节点熔断；
5. 路由脏数据清理。

---

## 二十、风险与决策

| 风险 | 等级 | 说明 | 建议 |
|---|---|---|---|
| 路由表写入主体 | **已闭环** | connect 直写 `user:routing:{uuid}` | 见第八章，阶段 1 同步落地 |
| `MsgPushEvent.seq` 映射不明 | **已闭环** | 原先需要 Push-Job 解包 `MsgItem` 取 seq | 6.4 新增顶层 `seq` 字段 |
| `pkg/kafka.Consumer` 关闭时丢 commit | **已闭环** | handler 与 commit 共 ctx 导致末批 offset 丢失 | Push-Job 自研独立消费循环（10.1） |
| `MSG_MARK_READ` 缺 `conv_type` 被误杀 | **已闭环** | 原校验要求 `conv_type` 必须 P2P/GROUP | 12.2 放宽为 MSG_MARK_READ 可空 |
| 群成员来源未确定 | 高 | GROUP fan-out 无法实现 | 群消息放到阶段 4，先 P2P 闭环 |
| `CONNECT_SELF_GRPC_ADDR` 获取失败 | 中 | connect 无法写出可被回拨的地址 | 启动强制校验，非法时拒绝启动（8.2.1） |
| 同会话顺序被并发破坏 | 中 | 并发消费可能导致乱序 | 第一阶段顺序处理；第二阶段按 `conv_id` 固定 worker + per-partition offset tracker（十七章） |
| `trace_id` 生产侧未填充 | 中 | 跨 Kafka 链路追踪不完整 | 后续补齐 `ctxmeta` 到 `MsgPushEvent.TraceId` |
| connect `PushToUser` 无法排除设备 | 中 | 多端同步可能推回发送设备 | 对发送者同步使用设备级路由 + `PushToDevice` |
| 路由表脏数据 | 中 | TTL 未过期但连接已断 | `delivered=0` 时记 `route_stale_total`，依赖 TTL 自愈，不重查重试（十三章） |
| 实时推送非强必达 | 低 | 依赖 PullMessages 补偿 | 产品和客户端需明确该语义 |

---

## 二十一、推荐第一阶段开发任务拆分

1. **协议**：修改 `proto/msg/msg_push_event.proto` 新增顶层 `seq` 字段（field 9），重生成 `apps/msg/pb/`；
2. **msg-service 生产侧**：`SendMessageWorkflow` 填 `Seq = msg.Seq`；`RecallMessageWorkflow` 明确填 `ConvType`（禁止走 `conv_id[:4]=="p2p-"` 反推）；
3. **connect 路由写入**：在 `apps/connect/internal/svc/lifecycle.go` 的 `OnConnect / OnHeartbeat / OnDisconnect` 里接入 Redis 路由读写，复用 `pkg/deviceactive.Syncer` 做心跳节流；`Shutdown` 时批量清理本节点路由；`CONNECT_SELF_GRPC_ADDR` 启动校验；
4. 新增 `apps/message-push/cmd` 启动骨架；
5. 在 `apps/message-push/internal/consumer/push_consumer.go` 内**自研独立消费循环**（不复用 `pkg/kafka.Consumer`，实现细节见 10.1）；
6. 新增 `EventHandler`，完成 `MsgPushEvent` 解码和 `MSG_PUSH` 分发；按 12.2 校验规则处理坏消息；
7. 新增 Redis `RouteRepository`，读取 `user:routing:{uuid}`；
8. 新增 connect gRPC `ClientManager`（按 addr 复用连接、超时采用 9.3 新值）；
9. 新增 `Dispatcher`，支持 P2P 对端 `PushToUser`；`delivered=0` 时记 `route_stale_total` 不重试；
10. 增加基础 metrics（含 `route_miss_total / route_stale_total`）；
11. 增加单元测试（`EventHandler / RouteRepository / Dispatcher`）；
12. 本地联调 Kafka -> Push-Job -> connect -> WebSocket；
13. 阶段验收通过后，再补多端同步、撤回和已读。

---

## 二十二、关键代码参考

| 目的 | 文件 |
|---|---|
| msg-service 发送后写 Kafka | `apps/msg/internal/usecase/send_message_workflow.go` |
| msg.push producer 封装 | `apps/msg/mq/producer.go` |
| 推送事件协议 | `proto/msg/msg_push_event.proto` |
| Kafka 配置 | `config/kafka.go` |
| connect 连接生命周期 | `apps/connect/internal/svc/lifecycle.go` |
| 心跳活跃节流复用 | `pkg/deviceactive/cache.go` |
| connect gRPC server | `apps/connect/internal/grpc/server.go` |
| connect 连接管理 | `apps/connect/internal/manager/connection_manager.go` |
| connect 协议 | `proto/connect/connect.proto` |
| 通用 Kafka Consumer（仅作为反面样本参考，不复用） | `pkg/kafka/consumer.go` |
| 现有 consumer 参考（Redis 重试，非本方案目标语义） | `apps/user/mq/redis_consumer.go` |
| 服务启动模式参考 | `apps/msg/cmd`、`apps/user/cmd`、`apps/connect/cmd` |

---

## 二十三、最终推荐

推荐按"独立 Push-Job + connect 纯通道 + connect 直写路由表"的方向开发，不建议把 Kafka consumer 放入 `connect-service`。

第一阶段只做 P2P 新消息实时推送闭环，先证明：

```text
msg-service 写 Kafka（含顶层 seq）
  -> Push-Job 消费（自研独立消费循环）
  -> 读 user:routing:{uuid}（connect 直写）
  -> connect gRPC PushToUser
  -> WebSocket 收到
```

闭环稳定后，再逐步加入多端同步、撤回、已读、群扩散和 DLQ。这样可以保持实现简单，避免一开始同时引入群成员、重试队列、批量投递等多个不确定点。
