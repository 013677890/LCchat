# Connect 服务 Kafka 消费者设计方案

> 目标：实现消息下行链路，使 msg-service 写入 Kafka 的 `MsgPushEvent` 最终投递到在线 WebSocket 客户端。

---

## 一、背景与现状

```
现在（断裂）：
  Client ──HTTP──► Gateway ──gRPC──► msg-service ──Kafka──► 🕳️ 没有消费者

目标（闭环）：
  msg-service ──Kafka──► Connect(consumer) ──WebSocket──► Client
```

**关键约束**：
- 当前是**单节点 Connect**（无路由表、无 Push-Job），Kafka consumer 内嵌在 Connect 进程中
- `ConnectionManager` 已提供 `SendToUser` / `SendToDevice` / `GetOnlineDevices`，直接可用
- `pkg/kafka.Consumer` 已封装好，直接复用
- **群组功能尚未实现**（user-service pb 中无 `GetGroupMembers`），群聊扩散暂时跳过

---

## 二、架构

```
Connect 进程
├── HTTP Server        (WebSocket 接入)
├── gRPC Server        (PushToUser 接口，供未来多节点 Push-Job 调用)
└── Kafka Consumer     ← 新增
    └── handleEvent
        ├── P2P 单聊  → SendToUser(receiver_uuid, frame)
        ├── 群聊      → 暂跳过（GetGroupMembers 未实现）
        └── 多端同步  → SendToDevice(from_uuid, 非发送设备, frame)
```

---

## 三、WebSocket 下行帧格式

复用现有 `Envelope` 结构，`data` 为 **proto bytes（base64 编码后包含在 JSON 中）**：

```json
{
  "type": "MSG_PUSH",
  "data": "<MsgItem proto bytes>"
}
```

| `type` | `data` 对应的 proto 类型 |
|---|---|
| `MSG_PUSH` | `MsgItem` |
| `MSG_RECALL` | `RecallNotice` |
| `MSG_MARK_READ` | `MarkReadNotice` |

客户端按 `type` 字段选择对应的 proto message 解码 `data`。

---

## 四、新增/改动文件

### [NEW] `apps/connect/mq/consumer.go`

```go
type PushConsumer struct {
    consumer    *kafka.Consumer
    connManager *manager.ConnectionManager
    connectSvc  *svc.ConnectService
}

func (c *PushConsumer) Start(ctx context.Context) error {
    return c.consumer.Start(ctx, c.handleEvent)
}

func (c *PushConsumer) handleEvent(ctx context.Context, raw []byte) error {
    var event msgpb.MsgPushEvent
    if err := proto.Unmarshal(raw, &event); err != nil {
        logger.Warn(ctx, "MsgPushEvent 反序列化失败，跳过", logger.ErrorField("error", err))
        return nil // 不 return err，避免死循环重试
    }

    // 组装下行帧：data 直接透传 proto bytes
    frame, err := c.connectSvc.MarshalEnvelope(event.Type, event.Data)
    if err != nil { return nil }

    switch event.ConvType {
    case msgpb.ConvType_CONV_TYPE_P2P:
        c.pushP2P(ctx, &event, frame)
    case msgpb.ConvType_CONV_TYPE_GROUP:
        // TODO: 需 user-service 实现 GetGroupMembers 后补全
        logger.Warn(ctx, "群聊推送暂未实现", logger.String("group_uuid", event.ReceiverUuid))
    }

    c.pushSelfSync(ctx, &event, frame)
    return nil
}

// pushP2P 单聊投递：指定设备或广播所有设备
func (c *PushConsumer) pushP2P(ctx context.Context, event *msgpb.MsgPushEvent, frame []byte) {
    if event.DeviceId != "" {
        c.connManager.SendToDevice(event.ReceiverUuid, event.DeviceId, frame)
    } else {
        c.connManager.SendToUser(event.ReceiverUuid, frame)
    }
}

// pushSelfSync 多端同步：向 from_uuid 其他在线设备投递，排除发送设备
func (c *PushConsumer) pushSelfSync(ctx context.Context, event *msgpb.MsgPushEvent, frame []byte) {
    if event.FromUuid == "" || event.FromUuid == event.ReceiverUuid {
        return
    }
    for _, deviceID := range c.connManager.GetOnlineDevices(event.FromUuid) {
        if deviceID == event.DeviceId {
            continue // 跳过发送方当前设备
        }
        c.connManager.SendToDevice(event.FromUuid, deviceID, frame)
    }
}

func (c *PushConsumer) Close() error { return c.consumer.Close() }
```

### [MODIFY] `apps/connect/cmd/providers.go`

新增三个环境变量 provider 和 `PushConsumer` 构造 provider：

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker 地址（逗号分隔） |
| `KAFKA_MSG_PUSH_TOPIC` | `msg.push` | 消息推送事件 topic |
| — | `connect-push-consumer` | consumer group ID（固定） |

```go
func providePushConsumer(brokers, topic, groupID ..., connManager, connectSvc) *mq.PushConsumer
```

将 `providePushConsumer` 加入 `connectProviderSet`。

### [MODIFY] `apps/connect/cmd/app.go`

```go
type ConnectApp struct {
    // ... 现有字段 ...
    pushConsumer *mq.PushConsumer  // 新增
}

// Run 中新增：
go func() {
    if err := a.pushConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
        errCh <- fmt.Errorf("Kafka 消费者异常退出: %w", err)
    }
}()

// Shutdown 中新增：
if a.pushConsumer != nil { _ = a.pushConsumer.Close() }
```

### [MODIFY] `apps/connect/cmd/wire.go` + 重新生成 `wire_gen.go`

在 Wire set 中加入新 provider，`PushConsumer` 注入 `ConnectApp`。

---

## 五、实现步骤

1. 新建 `apps/connect/mq/consumer.go`
2. 修改 `apps/connect/cmd/providers.go`，新增 Kafka provider
3. 修改 `apps/connect/cmd/app.go`，接入 `PushConsumer`
4. 修改 `apps/connect/cmd/wire.go`，加入 Wire 装配
5. 执行 `wire gen ./apps/connect/cmd/`，重新生成 `wire_gen.go`
6. `go build ./apps/connect/...` 验证

---

## 六、遗留问题

| 问题 | 何时解决 |
|---|---|
| 群聊成员扩散 | 等 user-service 实现 `GetGroupMembers` gRPC 接口 |
| 多节点 Connect 路由 | 后续扩节点时引入 Redis 路由表 + 独立 Push-Job |
