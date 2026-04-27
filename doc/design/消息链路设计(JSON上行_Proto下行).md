# 消息链路设计（JSON 上行 + Proto 下行）

## 1. 目标与原则

本方案采用“外松内严”的协议策略：

- 外部上行（客户端 -> Gateway）：使用 JSON，便于多端联调与排错。
- 内部服务间（Gateway -> Message、Message -> Connect）：使用 Protobuf，提升性能与类型约束。
- 下行推送（Connect -> 客户端）：使用 WebSocket Binary 帧承载 Protobuf，降低带宽与解析开销。

该方案适用于 IM 场景下“上行易开发、下行高吞吐”的需求。

## 2. 端到端链路

```text
Client(JSON) 
  -> Gateway(HTTP/WS入口，鉴权、校验、JSON->Proto)
  -> Message Service(gRPC，业务处理、落库)
  -> Kafka(Proto事件分发)
  -> Connect Service(消费Kafka，在线路由)
  -> Client(WebSocket Binary + Proto)
```

## 3. 各阶段协议与职责

### 3.1 上行阶段（开发友好）

客户端向 Gateway 发送 JSON 消息（HTTP 上行 JSON ）：

```json
{
  "type": "message",
  "data": {
    "client_msg_id": "c-uuid-001",
    "conversation_id": "conv-1001",
    "receiver_id": "user-1002",
    "content_type": "text",
    "content": "Hello"
  }
}
```

Gateway 负责：

1. 鉴权与参数校验（token、device_id、字段合法性）。
2. 生成/透传 `trace_id`。
3. JSON 映射为 Protobuf 请求（建议使用 `protojson`）。
4. 通过 gRPC 调用 Message Service。

### 3.2 处理与分发阶段（高吞吐）

Message Service 负责：

1. 幂等校验（`user_uuid + device_id + client_msg_id`）。
2. 消息落库与业务规则处理（风控、审核、会话更新等）。
3. 生成下行事件并写入 Kafka（建议值为 Protobuf 二进制）。

Kafka 消息建议包含：

- 路由信息：`receiver_user_uuid`、`receiver_device_id`（可选）。
- 消息标识：`server_msg_id`、`client_msg_id`。
- 业务负载：`payload`（bytes）。
- 消息标号：`sequence` (全局递增序列号，便于调试和顺序保证)。
- 追踪字段：`trace_id`、`server_ts`。

### 3.3 下行阶段（性能优先）

Connect Service 负责：

1. 消费 Kafka 下行事件。
2. 根据路由信息调用连接管理器：
   - 单设备：`SendToDevice(user_uuid, device_id, msg)`
   - 多设备：`SendToUser(user_uuid, msg)`
3. 通过 WebSocket Binary 帧将 Protobuf 发送给客户端。

客户端负责：

1. 接收 Binary 帧。
2. 按协议解码（先外层 Envelope，再内层业务 payload）。
3. 更新 UI，并按需回 ACK（已达/已读）。

## 4. 建议的消息封装

建议统一外层 Envelope，便于扩展和多业务复用：

- `command`：消息命令字（例如 `MSG_PUSH`、`KICKOUT`、`NOTICE`）。
- `payload`：业务消息 bytes（Protobuf）。
- `trace_id`：链路追踪。
- `server_ts`：服务端时间戳。
- `version`：协议版本（便于灰度演进）。

## 5. 关键设计点（必须落实）

### 5.1 幂等

- 上行必须带 `client_msg_id`。
- Message Service 以 `(sender, device, client_msg_id)` 去重，防止重试导致重复消息。

### 5.2 ACK 语义

建议统一 ACK 结构：

- `client_msg_id`
- `server_msg_id`
- `code`（成功/失败/离线等）
- `message`

### 5.3 顺序保证

- Kafka 分区键建议按 `conv_id`，以保证单会话的消息绝对有序下发。
- 需要明确“单会话有序”还是“单用户有序”。

### 5.4 离线策略

- Connect 下发失败（不在线、队列满）时，必须有明确处理：
  - connect只负责读和推

### 5.5 可观测性

建议最少监控：

- Gateway：上行请求量、gRPC 调用耗时与失败率。
- Message：落库耗时、Kafka 投递失败率、幂等命中率。
- Connect：在线连接数、下行入队成功率、队列满丢弃数、WS 断线率。

## 6. 与当前项目的对齐说明

当前仓库中：

- Connect 已具备 WebSocket 接入、连接管理与握手限流能力。
- Connect 的 `message` 上行处理仍是占位逻辑（需要接入真实消息链路）。
- `apps/connect/pb/connect.proto` 已定义 `PushToDevice/PushToUser/KickConnection`，建议尽快落地对应 gRPC server 实现。

## 7. 落地顺序建议

1. 先打通“上行 JSON -> Message 入库 -> Kafka 事件”。
2. 再打通“Connect 消费 Kafka -> WS Binary 推送”。
3. 最后补齐 ACK、离线重试、监控告警与压测。

---

本设计文档用于统一客户端、Gateway、Message、Connect 四侧协议与职责边界。后续若调整字段，需同步更新 `.proto` 与本文档版本记录。

