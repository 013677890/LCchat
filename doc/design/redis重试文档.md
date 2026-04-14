# Redis 重试机制使用指南

## 概述

本项目实现了基于 Kafka 的 Redis 操作重试机制。当 Redis 的增删改操作失败时，会自动将任务发送到 Kafka 队列进行异步重试，提高系统的可靠性。

## 架构设计

```
Repository 层
    ↓ (Redis 操作失败)
LogAndRetryRedisError()
    ↓
mq.SendRedisTask()
    ↓
Kafka Producer (发送任务到队列)
    ↓
redis-retry-queue (Kafka Topic)
    ↓
RedisRetryConsumer (后台消费)
    ↓
重新执行 Redis 操作
```

## 代码结构

```
pkg/kafka/                     # 通用 Kafka 组件
├── producer.go                # 通用 Kafka 生产者
├── consumer.go                # 通用 Kafka 消费者
└── logger_adapter.go          # 日志适配器

apps/user/mq/                  # Redis 重试业务逻辑
├── redis_task.go              # RedisTask 定义 + 构造器
├── redis_consumer.go          # Redis 重试消费者
└── manager.go                 # 全局 Producer 管理

apps/user/internal/repository/
└── errors.go                  # LogAndRetryRedisError 函数
```

## 配置

### 1. Kafka 配置 (config/kafka.go)

默认配置：
- Brokers: `kafka:9092`
- Topic: `redis-retry-queue`
- Consumer Group: `redis-retry-consumer-group`
- 最大重试次数: 3次

### 2. 启动服务

Kafka Producer 和 Consumer 在 `apps/user/cmd/main.go` 中自动初始化，无需手动配置。

## 使用方法

### 场景 1: 简单的 DEL 操作

```go
import "github.com/013677890/LCchat-Backend/apps/user/mq"

func (r *UserRepository) DeleteUserCache(ctx context.Context, userUUID string) error {
    key := fmt.Sprintf("user:info:%s", userUUID)
    
    // 执行 Redis 删除操作
    err := r.redis.Del(ctx, key).Err()
    if err != nil {
        // 构造重试任务
        task := mq.BuildDelTask(key).
            WithSource("UserRepository.DeleteUserCache")
        
        // 发送到重试队列
        LogAndRetryRedisError(ctx, task, err)
        
        return WrapRedisError(err)
    }
    
    return nil
}
```

### 场景 2: SET 操作（带 TTL）

```go
func (r *DeviceRepository) SaveToken(ctx context.Context, deviceID string, token string, ttl time.Duration) error {
    key := fmt.Sprintf("device:token:%s", deviceID)
    
    // 执行 Redis SET 操作
    err := r.redis.Set(ctx, key, token, ttl).Err()
    if err != nil {
        // 构造重试任务
        task := mq.BuildSetTask(key, token, ttl).
            WithSource("DeviceRepository.SaveToken")
        
        // 发送到重试队列
        LogAndRetryRedisError(ctx, task, err)
        
        return WrapRedisError(err)
    }
    
    return nil
}
```

### 场景 3: Pipeline 批量操作

```go
func (r *UserRepository) DeleteUserCacheAndRelation(ctx context.Context, userUUID string) error {
    // 准备 Pipeline 命令
    cmds := []mq.RedisCmd{
        {Command: "del", Args: []interface{}{fmt.Sprintf("user:info:%s", userUUID)}},
        {Command: "del", Args: []interface{}{fmt.Sprintf("user:relation:%s", userUUID)}},
        {Command: "del", Args: []interface{}{fmt.Sprintf("user:session:%s", userUUID)}},
    }
    
    // 执行 Pipeline
    pipe := r.redis.Pipeline()
    for _, cmd := range cmds {
        args := make([]interface{}, 0, len(cmd.Args)+1)
        args = append(args, cmd.Command)
        args = append(args, cmd.Args...)
        pipe.Do(ctx, args...)
    }
    
    _, err := pipe.Exec(ctx)
    if err != nil {
        // 构造重试任务
        task := mq.BuildPipelineTask(cmds).
            WithSource("UserRepository.DeleteUserCacheAndRelation")
        
        // 发送到重试队列
        LogAndRetryRedisError(ctx, task, err)
        
        return WrapRedisError(err)
    }
    
    return nil
}
```

## 构造器函数 (Builder Functions)

### 可用的构造器

| 函数 | 用途 | 示例 |
|------|------|------|
| `mq.BuildDelTask(key)` | 删除键 | `mq.BuildDelTask("user:1")` |
| `mq.BuildSetTask(key, val, ttl)` | 设置键值（带TTL） | `mq.BuildSetTask("token:123", "abc", 5*time.Minute)` |
| `mq.BuildHSetTask(key, field, value)` | Hash 字段设置 | `mq.BuildHSetTask("user:1", "name", "Alice")` |
| `mq.BuildHDelTask(key, fields...)` | Hash 字段删除 | `mq.BuildHDelTask("user:1", "cache", "temp")` |
| `mq.BuildSAddTask(key, members...)` | Set 添加成员 | `mq.BuildSAddTask("friends:1", "user2", "user3")` |
| `mq.BuildSRemTask(key, members...)` | Set 删除成员 | `mq.BuildSRemTask("friends:1", "user2")` |
| `mq.BuildPipelineTask(cmds)` | Pipeline 批量操作 | 见场景3 |
| `mq.BuildLuaTask(script, keys, args)` | Lua 脚本执行 | 见完整文档 |

### 链式方法

所有构造器返回的 `mq.RedisTask` 都支持以下链式方法：

```go
task := mq.BuildDelTask("user:1").
    WithContext(ctx).              // 添加上下文信息（trace_id, user_uuid, device_id）
    WithError(err).                // 添加原始错误信息
    WithSource("UserRepo.Delete"). // 添加来源标识
    WithMaxRetries(5)              // 设置最大重试次数（默认3次）
```

## 监控和告警

### 日志示例

**正常重试：**
```json
{
  "level": "error",
  "msg": "Redis 操作失败，发送到重试队列",
  "error": "connection refused",
  "task_type": "simple",
  "command": "del",
  "trace_id": "abc123",
  "user_uuid": "user123"
}
```

**Kafka 发送失败：**
```json
{
  "level": "error",
  "msg": "发送 Redis 重试任务到 Kafka 失败，放弃处理",
  "kafka_error": "kafka: connection timeout",
  "original_error": "redis: connection refused",
  "task_type": "simple"
}
```

**达到最大重试次数：**
```json
{
  "level": "error",
  "msg": "Redis 任务达到最大重试次数，放弃处理",
  "error": "redis: connection refused",
  "retry_count": 3,
  "max_retries": 3
}
```

## 注意事项

### 1. 只重试增删改操作

查询操作（GET, HGET, EXISTS 等）失败不需要重试，直接返回错误即可：

```go
// ❌ 不要对查询操作使用重试
val, err := r.redis.Get(ctx, key).Result()
if err != nil {
    return "", WrapRedisError(err)  // 直接返回错误
}
```

#### 1.1 读路径回填不重试（Fire and Forget）

缓存回填（read-through / cache-aside rebuild）是“读路径”的副作用：Redis 写失败不会影响主流程正确性，下次请求仍会回源并再次尝试回填。因此回填失败不应走 MQ 重试队列，只需要记录日志即可。

```go
// ✅ 回填失败只记录日志，不使用重试队列
pipe := r.redis.Pipeline()
pipe.Set(ctx, key, value, ttl)
_, err := pipe.Exec(ctx)
if err != nil {
    LogRedisError(ctx, err) // Fire and Forget
}
```

适用场景：
- Query/Read 后的缓存重建（Set/Hash/Set/ZSet 回填）
- 空值缓存写入
- 批量回填 Pipeline

### 2. 幂等性要求

所有发送到重试队列的操作必须是幂等的（多次执行结果相同）：

- ✅ DEL, SET, HSET, HDEL（幂等）
- ✅ SADD, SREM（幂等）
- ⚠️ INCR, DECR（非幂等，需要特殊处理）
- ⚠️ LPUSH, RPUSH（非幂等，需要特殊处理）

### 3. 性能考虑

- Kafka 发送是异步的，不会阻塞主流程
- 如果 Kafka Producer 未初始化，会静默失败（不影响业务）
- 消费者在后台单独的 goroutine 中运行

### 4. 数据一致性

Redis 重试机制不保证强一致性，只保证最终一致性：
- 如果重试失败，数据可能永久丢失
- 建议配合 MySQL 持久化层使用
- 关键操作应该先写 MySQL，再更新 Redis

## 完整示例：Repository 方法

```go
func (r *UserRepository) DeleteUser(ctx context.Context, userUUID string) error {
    // 1. 先删除 MySQL（持久化层）
    if err := r.db.Where("uuid = ?", userUUID).Delete(&model.UserInfo{}).Error; err != nil {
        return WrapDBError(err)
    }
    
    // 2. 删除 Redis 缓存（可失败）
    key := fmt.Sprintf("user:info:%s", userUUID)
    if err := r.redis.Del(ctx, key).Err(); err != nil {
        // 发送到重试队列，但不影响主流程
        task := mq.BuildDelTask(key).
            WithSource("UserRepository.DeleteUser")
        LogAndRetryRedisError(ctx, task, err)
        
        // 记录警告日志但继续执行
        logger.Warn(ctx, "删除用户缓存失败，已发送到重试队列",
            logger.String("user_uuid", userUUID),
            logger.ErrorField("error", err),
        )
    }
    
    return nil
}
```

## 故障排查

### Kafka Consumer 没有启动

检查日志中是否有：
```
"msg": "Redis 重试消费者启动中"
```

### 任务没有被消费

1. 检查 Kafka Topic 是否存在：`kafka-topics --list`
2. 检查消费者组状态：`kafka-consumer-groups --group redis-retry-consumer-group --describe`
3. 检查 Redis 连接状态

### 重试一直失败

1. 检查 Redis 集群是否正常
2. 检查网络连接
3. 查看错误日志，确认错误原因
4. 考虑增加重试次数或调整重试策略
