# P0 Redis补偿重试拓扑

**中文说明：** 展示 Redis 重试补偿拓扑：Repository 失败任务进入 Kafka，再由消费者重放。

## 过程讲解

1. 组件分层：图中从左到右展示调用方、业务服务和存储层，先看主链路再看旁路。
2. 数据流向：箭头表示请求或数据流方向，重点关注跨服务调用点与异步通道（如 Kafka）。
3. 关键依赖：底部存储节点体现最终落点，便于定位一致性边界、性能瓶颈和故障恢复路径。

```mermaid
flowchart LR
    Repo[User Repository] -->|redis write fail| TaskBuilder[Build RedisTask]
    TaskBuilder --> Producer[Kafka Producer]
    Producer --> Topic[(redis_retry_topic)]
    Topic --> Consumer[RedisRetryConsumer]
    Consumer --> Redis[(Redis)]
    Consumer -->|fail and retry<max| Topic
    Consumer -->|max retries| DeadLog[Error log or alert]
```
