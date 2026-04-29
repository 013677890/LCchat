 # ctx 传递问题排查与改造方案

 > 文档状态：排查结论与改造建议  
 > 适用范围：`gateway`、`user`、`msg`、`connect`、`message-push`、`pkg/async`、`pkg/deviceactive`  
 > 关联文档：`doc/design/并发调用服务问题现象与改造方案.md`  
 > 更新时间：2026-04-29

 ---

 ## 一、结论概览

 当前项目并不是完全没有传递 `ctx`，而是存在明显的语义分层不清：

1. **Gateway 主链路的 ctx/metadata 传递相对完整**：HTTP 请求经 `TraceLogger`、`ClientIPMiddleware`、认证中间件写入 `trace_id/user_uuid/device_id/client_ip`，再由 Gateway gRPC client interceptor 透传到下游。
2. **请求内并发错误使用了后台异步模型**：`gateway`、`msg` 中部分“必须等待结果”的 fan-out 使用了 `async.RunSafe + channel`，会切断父请求取消/超时语义，并存在提交失败后等待 channel 的风险。
3. **后台异步大量使用 `context.Background()`**：能避免请求返回后任务被取消，但同时会丢失 trace、用户、设备、客户端 IP 等链路信息，也不利于 shutdown 统一收敛。
4. **跨服务 metadata 透传不统一**：Gateway 出站 gRPC 有 metadata 注入，但 `msg -> user`、`connect -> user`、`message-push -> user/connect` 等链路大多只配置了 timeout interceptor，没有统一透传 metadata。
5. **connect 状态同步存在 channel 生命周期竞态**：关闭 `statusQueue` 时仍可能有 goroutine 发送任务，存在 `send on closed channel` panic 风险。
6. **批量 fan-out 对 `ctx.Done()` 响应不足**：广播、批量推送链路在循环中没有主动检查取消信号，超时后仍可能继续消耗资源。

推荐总体方向与并发改造文档一致：

- **请求内并发统一使用 `errgroup.WithContext`**；
- **后台异步明确使用 detached context，但保留必要 metadata**；
- **gRPC metadata client interceptor 下沉到 `pkg/grpcx` 并全服务统一使用**；
- **connect/statusQueue、deviceactive、批量 fan-out 补齐生命周期、取消传播和可观测性**。

---

## 二、当前 ctx 传递链路现状

### 2.1 Gateway HTTP 主链路

Gateway 侧已经具备比较完整的入口 ctx 元数据采集：

| 信息 | 写入位置 | 说明 |
|---|---|---|
| `trace_id` | `pkg/util/trace.go` | 从 `X-Request-ID` 读取或生成 UUID，并写入 Gin context |
| `client_ip` | `apps/gateway/internal/middleware/client_ip.go` | 解析客户端真实 IP，并写入 Gin context |
| `user_uuid` | `apps/gateway/internal/middleware/auth.go` | JWT 认证后写入 Gin context |
| `device_id` | `apps/gateway/internal/middleware/auth.go` / 登录接口 | 从 token 或 `X-Device-ID` 写入 Gin context |

`pkg/ctxmeta/gin.go` 的 `BuildContextFromGin` 会基于 `c.Request.Context()` 构造业务 ctx，并复制 Gin context 中的元数据。该设计是正确的，因为它保留了 HTTP 请求的取消和 deadline。

### 2.2 Gateway 出站 gRPC

Gateway 有专门的 metadata interceptor：

- `apps/gateway/internal/middleware/grpc_metadata.go`
- `apps/gateway/internal/pb/client.go`

它会把 ctx 中的：

- `trace_id`
- `user_uuid`
- `device_id`
- `client_ip` / `x-real-ip`

写入 gRPC outgoing metadata。

### 2.3 服务端 gRPC 入站 metadata

服务端统一 interceptor 位于：

- `pkg/grpcx/metadata.go`

它会从 incoming metadata 中读取 `trace_id/user_uuid/device_id/client_ip` 并注入到业务 ctx。

因此，**只要客户端把 metadata 发出来，服务端就能正常接收**。

当前问题是：并不是所有 gRPC 客户端都配置了 metadata 注入。

---

## 三、重点问题与影响

## 3.1 `async.RunSafe` 切断父 ctx，不能用于请求内 fan-out

### 代码位置

- `pkg/async/pool.go`
- `apps/gateway/internal/service/user_service.go`
- `apps/msg/internal/domain/conversation/service.go`

### 当前实现

`RunSafe` 内部从 `context.Background()` 创建 `baseCtx`，再通过 `ContextPropagator` 只复制部分元数据，最后套自己的 timeout：

```go
baseCtx := context.Background()
if ContextPropagator != nil && ctx != nil {
    baseCtx = ContextPropagator(ctx)
}
runCtx, cancel := context.WithTimeout(baseCtx, timeout)
```

提交失败时只记录日志：

```go
if err := Submit(wrap); err != nil {
    cancel()
    logger.Error(baseCtx, "async submit failed", ...)
    return
}
```

### 问题

这套语义适合后台异步任务，但不适合请求内并发：

1. 父请求取消后，异步任务仍会继续执行；
2. 父请求 deadline 不会传递给异步任务；
3. 协程池提交失败时，调用方拿不到错误；
4. 如果调用方正在等待 channel 结果，就可能悬挂等待；
5. 只复制白名单元数据，其它 context value 或 outgoing metadata 会丢失。

### 影响

- 聚合接口偶发卡住；
- 下游服务请求数高于上游有效请求数；
- 请求超时后 DB/RPC 仍在执行；
- trace 链路可能断裂；
- 压测 P99/P999 长尾抬高。

### 解决方案

明确拆分两类 API：

#### 请求内并发

统一使用：

```go
errgroup.WithContext(ctx)
```

要求：

- 使用父 ctx 派生；
- 自动响应取消和 deadline；
- 所有 goroutine 由 `errgroup` 收敛；
- 禁止使用 `RunSafe + channel + 必须等待结果`。

#### 后台异步

保留协程池，但建议改名或新增 API：

```go
async.RunDetached(...)
async.SubmitDetached(...)
```

并返回提交结果：

```go
err := async.SubmitDetached(ctx, taskName, timeout, fn)
```

后台异步使用 detached context：

```go
baseCtx := ctxmeta.CopyKnownFromParent(parent)
runCtx, cancel := context.WithTimeout(baseCtx, timeout)
```

语义上表示：**不继承取消，只保留链路元数据**。

---

## 3.2 Gateway `GetOtherProfile` 存在 channel 悬挂风险

### 代码位置

- `apps/gateway/internal/service/user_service.go`

### 当前实现

`GetOtherProfile` 并发查询：

1. 用户资料；
2. 好友关系。

当前使用两个 channel 收敛结果，并通过 `async.RunSafe` 提交任务。

如果 `async.RunSafe` 提交失败，不会写入 channel，但主协程仍然执行：

```go
userRes := <-userChan
friendRes := <-friendChan
```

### 问题

- 协程池未初始化、已满、关闭中时可能永久等待；
- 父请求取消后，下游 RPC 仍可能继续执行；
- 好友关系查询是可降级分支，但当前没有结构化降级指标。

### 解决方案

改为 `errgroup.WithContext`：

```go
g, gctx := errgroup.WithContext(ctx)

var userResp *userpb.GetOtherProfileResponse
var friendResp *userpb.CheckIsFriendResponse
var friendErr error

g.Go(func() error {
    resp, err := s.userClient.GetOtherProfile(gctx, grpcReq)
    if err != nil {
        return err
    }
    userResp = resp
    return nil
})

g.Go(func() error {
    resp, err := s.userClient.CheckIsFriend(gctx, &userpb.CheckIsFriendRequest{
        UserUuid: currentUserUUID,
        PeerUuid: req.UserUUID,
    })
    if err != nil {
        friendErr = err
        return nil
    }
    friendResp = resp
    return nil
})

if err := g.Wait(); err != nil {
    return nil, err
}
```

好友关系查询失败时仍可按非好友降级，但需要补充指标：

- `gateway_profile_friend_check_degrade_total`
- `gateway_profile_friend_check_duration_seconds`

---

## 3.3 Msg 会话列表存在重复查询和取消失效问题

### 代码位置

- `apps/msg/internal/domain/conversation/service.go`

### 当前实现

会话列表查询同时拉取：

1. P2P 会话；
2. 群聊会话。

当前也是 `RunSafe + channel`，并额外使用 `waitCtx` 兜底。超时后会重新同步查询一次。

### 问题

虽然 `waitCtx` 避免了永久阻塞，但仍有问题：

1. `RunSafe` 内部脱离父 ctx，请求取消后查询可能继续跑；
2. 超时 fallback 会造成重复 DB 查询；
3. 代码复杂度高，维护风险大；
4. 多个 timeout 叠加后实际行为不直观。

### 解决方案

改为 `errgroup.WithContext`，删除 `waitCtx + select + fallback`：

```go
g, gctx := errgroup.WithContext(ctx)

var p2pConvs []*model.Conversation
var groupConvs []*model.Conversation

g.Go(func() error {
    convs, err := s.repo.ListP2P(gctx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
    if err != nil {
        return fmt.Errorf("获取会话列表 P2P 查询失败: %w", err)
    }
    p2pConvs = convs
    return nil
})

g.Go(func() error {
    convs, err := s.repo.ListGroup(gctx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
    if err != nil {
        return fmt.Errorf("获取会话列表 Group 查询失败: %w", err)
    }
    groupConvs = convs
    return nil
})

if err := g.Wait(); err != nil {
    return nil, false, "", err
}
```

---

## 3.4 gRPC metadata client interceptor 没有全服务复用

### 当前情况

Gateway 出站 gRPC 有 metadata 注入，但其它服务的 gRPC client 多数只配置了 timeout interceptor。

典型位置：

- `apps/msg/cmd/providers.go`
- `apps/connect/cmd/providers.go`
- `apps/message-push/cmd/providers.go`
- `apps/message-push/internal/connectcli/manager.go`

### 问题

这些链路会丢失：

- `trace_id`
- `user_uuid`
- `device_id`
- `client_ip`

具体影响：

1. `msg -> user` 查询群成员或权限时 trace 断链；
2. `connect -> user` 更新设备状态时 trace/client_ip 不会透传；
3. `message-push -> connect` 推送时 Kafka event 的 `TraceId` 不会进入 gRPC metadata；
4. connect 服务端虽有 `MetadataUnaryInterceptor`，但上游很多时候没有发送 metadata。

### 解决方案

把 Gateway 当前的 metadata interceptor 下沉到 `pkg/grpcx`：

```go
func MetadataClientUnaryInterceptor() grpc.UnaryClientInterceptor
```

所有 gRPC client 统一配置：

```go
grpc.WithChainUnaryInterceptor(
    grpcx.MetadataClientUnaryInterceptor(),
    grpcx.ClientTimeoutUnaryInterceptor(...),
)
```

建议覆盖：

- `gateway -> user/msg`
- `msg -> user`
- `connect -> user`
- `message-push -> user`
- `message-push -> connect`

对于 `message-push`，还需要从 Kafka event 中恢复 ctx 元数据：

```go
if event.TraceId != "" {
    ctx = ctxmeta.WithTraceID(ctx, event.TraceId)
}
if event.FromUuid != "" {
    ctx = ctxmeta.WithUserUUID(ctx, event.FromUuid)
}
if event.DeviceId != "" {
    ctx = ctxmeta.WithDeviceID(ctx, event.DeviceId)
}
```

---

## 3.5 Connect 设备状态同步使用 `context.Background()` 导致 trace 丢失

### 代码位置

- `apps/connect/internal/svc/lifecycle.go`

### 当前实现

状态任务中保存了 `logCtx`：

```go
task := deviceStatusTask{
    logCtx: ctx,
    ...
}
```

但 worker 调 user-service 时使用：

```go
rpcCtx, cancel := context.WithTimeout(context.Background(), deviceStatusRPCTimeout)
```

### 问题

- `trace_id` 丢失；
- `client_ip` 丢失；
- gRPC metadata 不会带上连接上下文；
- user-service 日志无法和 connect 连接生命周期串起来。

### 解决方案

这是后台状态上报，不应直接继承连接 ctx 的取消，但应保留元数据：

```go
baseCtx := ctxmeta.CopyKnownFromParent(task.logCtx)
rpcCtx, cancel := context.WithTimeout(baseCtx, deviceStatusRPCTimeout)
defer cancel()
```

再配合 `grpcx.MetadataClientUnaryInterceptor()` 将 metadata 透传到 user-service。

---

## 3.6 `connect.statusQueue` 关闭存在 panic 风险

### 代码位置

- `apps/connect/internal/svc/lifecycle.go`
- `apps/connect/internal/svc/connect_service.go`

### 当前实现

发送任务：

```go
case s.statusQueue <- task:
```

关闭队列：

```go
close(s.statusQueue)
s.statusWg.Wait()
```

### 问题

如果 shutdown 期间还有 goroutine 发送状态任务，会触发：

```text
panic: send on closed channel
```

### 解决方案

引入关闭标记，采用“停止接收新任务 -> drain -> worker 退出”的模型。

建议字段：

```go
closing atomic.Bool
```

发送前检查：

```go
if s.closing.Load() {
    return
}
```

关闭时：

```go
s.closing.Store(true)
close(s.statusQueue)
s.statusWg.Wait()
```

如果仍担心检查与发送之间的竞态，可以进一步用 `stopCh` 或专门的 submit 方法封装发送，并在内部 recover `send on closed channel`，但更推荐通过生命周期约束避免发送方在关闭后继续提交。

---

## 3.7 `deviceactive` 后台批处理 ctx 生命周期不完整

### 代码位置

- `pkg/deviceactive/cache.go`
- `apps/connect/internal/svc/connect_service.go`
- `apps/gateway/cmd/app.go`

### 当前实现

`deviceactive` 消费批次时使用：

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
err := s.handler(ctx, batch)
```

而部分 handler 又忽略传入 ctx，重新创建 `context.Background()`。

### 问题

1. `Stop()` 时无法取消正在执行的批处理 RPC，只能等待 timeout；
2. handler 入参 ctx 的意义被削弱；
3. trace、任务名、来源等可观测信息不足；
4. 多层 `Background()` 导致生命周期不可控。

### 解决方案

`Syncer` 内部维护 root ctx：

```go
rootCtx, cancel := context.WithCancel(context.Background())
```

`Stop()` 时 cancel：

```go
cancel()
close(stopCh)
wg.Wait()
```

消费时基于 root ctx 派生：

```go
taskCtx, cancel := context.WithTimeout(s.rootCtx, 10*time.Second)
err := s.handler(taskCtx, batch)
cancel()
```

handler 内继续基于入参 ctx 派生 RPC timeout：

```go
rpcCtx, cancel := context.WithTimeout(ctx, cfg.RPCTimeout)
defer cancel()
```

---

## 3.8 批量 fan-out 未主动响应 `ctx.Done()`

### 代码位置

- `apps/connect/internal/grpc/server.go`
- `apps/message-push/internal/consumer/consumer.go`
- `apps/message-push/internal/connectcli/manager.go`

### 当前问题

`BroadcastToUsers` 对用户列表循环投递，但没有检查 `ctx.Done()`。如果上游已取消，循环仍可能继续执行。

`message-push` 推送多个设备时也类似，单次 PushToDevice 会带 ctx，但外层循环没有在每轮开始前主动判断取消状态。

### 解决方案

批量循环统一增加取消检查：

```go
for _, item := range items {
    select {
    case <-ctx.Done():
        return resp, ctx.Err()
    default:
    }

    // do work
}
```

大批量场景进一步增加：

- 分批处理；
- 并发度限制；
- 每批成功/失败/取消统计；
- 返回结构化降级结果。

---

## 四、推荐改造方案

## 4.1 明确 ctx 类型语义

建议将项目中的 ctx 分为三类：

| 类型 | 适用场景 | 是否继承取消 | 是否保留 metadata |
|---|---|---:|---:|
| request ctx | HTTP/gRPC 主请求、请求内 fan-out | 是 | 是 |
| detached ctx | 缓存更新、补偿、状态上报等后台任务 | 否 | 是 |
| process ctx | 服务生命周期、worker root context | 是，受进程 shutdown 控制 | 可选 |

### request ctx

来自：

- `c.Request.Context()`
- gRPC handler 入参 `ctx`

用于：

- 主链路 RPC；
- DB 查询；
- 请求内并发聚合。

### detached ctx

来自：

```go
ctxmeta.CopyKnownFromParent(parent)
```

用于：

- 请求返回后仍需要执行的异步任务；
- 缓存更新；
- 非阻断补偿；
- connect 状态上报。

### process ctx

由服务启动时创建，服务关闭时 cancel。

用于：

- 后台 worker；
- ticker loop；
- deviceactive syncer；
- Kafka consumer。

---

## 4.2 请求内并发统一封装

建议禁止在请求内使用：

```go
async.RunSafe(ctx, ...)
```

推荐使用：

```go
g, gctx := errgroup.WithContext(ctx)
```

如果担心并发度，可使用：

```go
g.SetLimit(n)
```

统一要求：

1. 父 ctx 取消后所有分支尽快退出；
2. 不允许通过 channel 永久等待；
3. 可降级分支错误不阻断主流程，但必须计数；
4. 不可降级分支错误通过 `g.Wait()` 返回。

---

## 4.3 后台异步统一封装

建议新增任务提交结果：

```go
type SubmitResult struct {
    Accepted bool
    Err      error
}
```

或直接返回 error：

```go
func SubmitDetached(parent context.Context, opt TaskOption, fn func(context.Context)) error
```

`TaskOption` 至少包含：

```go
type TaskOption struct {
    Module   string
    Name     string
    Source   string
    Timeout  time.Duration
}
```

日志字段统一包含：

- `task_name`
- `module`
- `source`
- `trace_id`
- `timeout`
- `degrade_reason`

---

## 4.4 gRPC metadata 透传统一封装

新增：

```go
package grpcx

func MetadataClientUnaryInterceptor() grpc.UnaryClientInterceptor
```

逻辑复用当前 Gateway 的 `GRPCMetadataInterceptor`。

所有服务的 gRPC client 创建都应包含：

```go
grpc.WithChainUnaryInterceptor(
    grpcx.MetadataClientUnaryInterceptor(),
    grpcx.ClientTimeoutUnaryInterceptor(...),
)
```

注意 interceptor 顺序建议：

1. metadata 注入；
2. timeout 收紧；
3. logging；
4. breaker / retry 相关。

如果 timeout interceptor 派生了新的 ctx，metadata 仍会随 parent ctx 保留；但为避免后续 interceptor 覆盖 outgoing context，建议在单测中覆盖顺序行为。

---

## 4.5 timeout 策略统一

当前项目存在多层 timeout：

1. HTTP Gateway 请求 timeout；
2. gRPC server timeout；
3. gRPC client method timeout；
4. 业务代码手动 `context.WithTimeout`；
5. `async.RunSafe` 自带 timeout。

建议原则：

- 请求内 RPC 主要依赖父 ctx + `ClientTimeoutUnaryInterceptor`；
- 不要在业务层随意使用 `context.WithTimeout(context.Background(), ...)`；
- 后台任务使用 detached ctx + task timeout；
- 进程 worker 使用 process ctx + 每次任务 timeout；
- 文档中标明最终生效 timeout 是多层 deadline 的最小值。

---

## 五、实施优先级

## 第一阶段：修稳定性 P0

目标：消除卡死和 panic 风险。

1. `gateway GetOtherProfile` 改为 `errgroup.WithContext`；
2. `msg GetConversations` 改为 `errgroup.WithContext`；
3. `connect.statusQueue` 增加 closing 标记，修复关闭竞态；
4. `async.RunSafe` 新增返回提交错误的替代 API，并禁止请求内 fan-out 使用旧模式。

交付标准：

- 不再存在必须等待 channel 但提交失败无返回的代码；
- 请求取消后请求内并发分支能退出；
- `statusQueue` 关闭期间不会发生 `send on closed channel`。

## 第二阶段：修 trace/metadata 断链 P1

目标：跨服务日志和链路追踪可串联。

1. 将 Gateway metadata client interceptor 下沉到 `pkg/grpcx`；
2. 所有 gRPC client 统一使用 metadata interceptor；
3. `message-push` 从 Kafka event 恢复 trace/user/device 到 ctx；
4. `connect` 状态上报基于 `ctxmeta.CopyKnownFromParent(task.logCtx)` 创建 RPC ctx。

交付标准：

- `gateway -> msg -> user` trace 连续；
- `message-push -> connect` trace 连续；
- `connect -> user` 状态同步日志可关联到连接上下文。

## 第三阶段：生命周期与可观测性 P2

目标：后台任务可控、可观测。

1. `deviceactive.Syncer` 引入 process root ctx；
2. batch handler 不再忽略入参 ctx；
3. 批量 fan-out 循环增加 `ctx.Done()` 检查；
4. 异步任务、队列、降级补齐指标。

交付标准：

- shutdown 能取消后台批处理；
- 批量广播和推送能响应取消；
- 有队列长度、提交失败、超时、丢弃、降级等指标。

---

## 六、代码改造检查清单

### 请求内并发

- [ ] 是否使用 `errgroup.WithContext(ctx)`？
- [ ] 是否继承父请求取消和 deadline？
- [ ] 是否避免 `RunSafe + channel + 必须等待结果`？
- [ ] 可降级分支是否有指标？

### 后台异步

- [ ] 是否明确 detached 语义？
- [ ] 是否只保留必要 metadata，而不是继承请求取消？
- [ ] 提交失败是否能返回给调用方？
- [ ] 是否有 task_name/module/source/trace_id 日志字段？

### gRPC metadata

- [ ] client 是否配置 `grpcx.MetadataClientUnaryInterceptor()`？
- [ ] server 是否配置 `grpcx.MetadataUnaryInterceptor()`？
- [ ] Kafka 消费场景是否从 event 恢复 trace 到 ctx？
- [ ] 日志中是否能看到连续 trace_id？

### timeout/cancel

- [ ] 是否避免 `context.WithTimeout(context.Background(), ...)` 滥用？
- [ ] 是否明确最终 timeout 来源？
- [ ] 批量循环是否检查 `ctx.Done()`？
- [ ] worker 是否能被 shutdown ctx 取消？

### channel 生命周期

- [ ] close channel 前是否阻止新发送？
- [ ] 是否存在发送到已关闭 channel 的风险？
- [ ] worker 是否有明确退出路径？

---

## 七、最终结论

当前项目 ctx 传递的主要问题不是入口没有 ctx，而是 **ctx 的语义在请求内并发、后台异步、跨服务调用之间混用了**。

具体表现为：

1. 请求内 fan-out 使用 `RunSafe`，导致取消和超时失效；
2. 后台任务直接使用 `Background`，导致 trace/metadata 丢失；
3. gRPC metadata client interceptor 只在 Gateway 完整使用，服务间调用没有统一；
4. connect 状态队列关闭存在 channel 竞态；
5. 批量 fan-out 缺少取消检查和结构化降级结果。

推荐按三个阶段落地：

1. **先修 P0 稳定性**：`errgroup` 替换请求内 fan-out，修 `statusQueue` 关闭竞态；
2. **再修 metadata 断链**：统一 gRPC metadata client interceptor，Kafka 消费恢复 trace；
3. **最后补齐生命周期和观测**：后台 worker root ctx、队列/任务指标、批量取消检查。

这样可以不推翻现有架构，又能明显提升请求稳定性、链路可观测性和服务关闭安全性。
