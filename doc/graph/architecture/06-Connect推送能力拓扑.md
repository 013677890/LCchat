# P1 Connect推送能力拓扑

**中文说明：** 展示 Connect 推送能力拓扑：外部服务通过 gRPC 调用后，由连接索引路由到设备连接。

## 过程讲解

1. 组件分层：图中从左到右展示调用方、业务服务和存储层，先看主链路再看旁路。
2. 数据流向：箭头表示请求或数据流方向，重点关注跨服务调用点与异步通道（如 Kafka）。
3. 关键依赖：底部存储节点体现最终落点，便于定位一致性边界、性能瓶颈和故障恢复路径。

```mermaid
flowchart LR
    MsgSvc[Msg Service] -->|Push RPC| ConnectGRPC
    UserSvc[User Service] -->|Kick or status RPC| ConnectGRPC
    BizSvc[Other Biz Services] -->|Broadcast RPC| ConnectGRPC

    ConnectGRPC --> CM[ConnectionManager bucket index]
    CM --> D1[Device connection]
    CM --> D2[Device connection]
    CM --> DN[Device connection]
```
