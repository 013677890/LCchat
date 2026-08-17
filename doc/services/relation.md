# relation 服务

relation 服务拥有好友和黑名单关系事实，负责关系写入、关系判断、好友列表和申请流程。

## 职责

- 发送、查询、处理好友申请。
- 好友列表读取和增量同步。
- 删除好友、设置备注、设置标签。
- 拉黑、取消拉黑、黑名单列表和拉黑判断。
- 为 msg 提供发送前关系权限判断。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/relation/cmd` | gRPC 服务和后台消费者装配。 |
| `apps/relation/internal/service` | 好友、黑名单业务逻辑。 |
| `apps/relation/internal/repository` | MySQL 和 Redis 关系缓存访问。 |
| `apps/relation/internal/consumer` | 账号注销等事件消费者。 |
| `proto/relation` | FriendService、BlacklistService 契约。 |

## 数据所有权

| 数据 | 存储 |
| --- | --- |
| 好友、拉黑、删除关系 | MySQL `user_relations`。 |
| 好友关系缓存 | Redis `user:relation:friend:{user_uuid}`。 |
| 黑名单缓存 | Redis `user:relation:blacklist:{user_uuid}`。 |
| 好友申请待处理缓存 | Redis `user:apply:pending:{target_uuid}`。 |
| 好友申请未读计数 | Redis `user:notify:friend_apply:unread:{uuid}`。 |

## 暴露的 gRPC 服务

| 服务 | 能力 |
| --- | --- |
| `FriendService` | 好友申请、好友列表、同步、备注、标签、关系判断。 |
| `BlacklistService` | 拉黑、取消拉黑、黑名单列表、拉黑判断。 |

## 事件

| 事件 | 方向 | 说明 |
| --- | --- | --- |
| `account.deleted` | 消费 | 账号注销后清理或标记相关关系。 |

## Kafka 消费编排

- `account.deleted` 与 `relation.redis.invalidate` 各使用独立 `ManualConsumerPool` 和 consumer group。
- `KAFKA_RELATION_ACCOUNT_DELETED_CONSUMER_CONCURRENCY`、`KAFKA_RELATION_REDIS_RETRY_CONSUMER_CONCURRENCY` 分别配置本进程 Reader 数；默认 3，显式值必须为 `1～64`。
- workers 由 Kafka rebalance 分配 partition；领域事件与安全 DEL 补偿只有成功或死信落地后才提交，未知字段和写回命令不会执行。
- Pool 致命失败在 RelationApp 内隔离、计数并退避重启，不拖死 gRPC。

## 协作关系

- Gateway 调用 relation 暴露好友和黑名单 HTTP API。
- msg 发送单聊前调用 relation 校验好友和黑名单，避免绕过关系权限。
- relation 聚合用户展示信息时不应直接依赖 auth 账号表。

## 不变量

- 关系事实以 relation 的 MySQL 表为准，Redis 只是缓存。
- 不能添加自己为好友，不能拉黑自己。
- 黑名单判断会影响消息发送权限和关系展示。
