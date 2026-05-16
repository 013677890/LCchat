# P1 Connect内部推送流程

**中文说明：** 展示 Connect 内部推送能力：单播、多播、广播和踢线统一经 ConnectionManager 路由。

## 过程讲解

1. 入口阶段：请求先进入图中的入口组件（客户端、Gateway 或 Connect），完成基础参数与上下文准备。
2. 核心处理：中间组件按时序执行鉴权、校验、路由、批处理或状态更新，关键分支在图中用 `alt/loop` 标注。
3. 结果输出：最终将数据写入目标存储或返回调用方；异常场景通常走降级、重试或丢弃保护逻辑。

```mermaid
sequenceDiagram
    participant S as InternalService
    participant CG as Connect.gRPC
    participant M as ConnectionManager
    participant CL as ClientDevices

    S->>CG: PushToDevice/PushToUser/Broadcast/KickConnection
    CG->>M: locate connection(s)
    alt push
        M->>CL: enqueue websocket frame
        CG-->>S: delivered count
    else kick
        M->>CL: close connection
        CG-->>S: success flag
    end
```
