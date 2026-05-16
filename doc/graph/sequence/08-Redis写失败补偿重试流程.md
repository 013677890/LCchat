# P0 Redis写失败补偿重试流程

**中文说明：** 展示 Redis 写失败补偿：失败任务写入 Kafka，消费者重试执行，达到上限后告警并丢弃。

## 过程讲解

1. 入口阶段：请求先进入图中的入口组件（客户端、Gateway 或 Connect），完成基础参数与上下文准备。
2. 核心处理：中间组件按时序执行鉴权、校验、路由、批处理或状态更新，关键分支在图中用 `alt/loop` 标注。
3. 结果输出：最终将数据写入目标存储或返回调用方；异常场景通常走降级、重试或丢弃保护逻辑。

```mermaid
sequenceDiagram
    participant RE as UserRepository
    participant RM as mq.manager
    participant KP as KafkaProducer
    participant KT as KafkaTopic
    participant KC as RedisRetryConsumer
    participant R as Redis

    RE->>R: write command
    alt write failed
        RE->>RM: Build RedisTask + Send
        RM->>KP: produce task
        KP->>KT: publish
        KC->>KT: consume task
        KC->>R: retry command
        alt failed and retry<count
            KC->>KT: re-publish with retry+1
        else max retries
            KC->>KC: log and discard
        end
    end
```
