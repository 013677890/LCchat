# 设备在线与 WebSocket 生命周期

本文描述设备会话、Connect WebSocket 建连、在线路由刷新、断线清理和在线状态查询之间的关系。HTTP 设备接口见 [认证与账号接口](../api/01-认证与账号接口.md)，协议细节见 [WebSocket协议](../api/06-WebSocket协议.md)。

## 1. 参与组件

| 组件 | 职责 |
| --- | --- |
| auth | 设备会话事实、设备列表、踢设备、在线状态查询。 |
| connect | WebSocket 建连、在线路由、ACK、心跳。 |
| gateway | 设备列表和在线状态 HTTP 入站。 |
| Redis | `user:routing:*`、设备活跃、Token 哈希。 |
| message-push | 查询在线路由并向 connect 精确投递。 |

## 2. 设备会话与在线连接的区别

| 概念 | 存储 | 含义 |
| --- | --- | --- |
| 设备会话 | MySQL `device_sessions` | 某用户曾经在哪个设备登录过，以及当前设备状态。 |
| 在线连接 | connect 内存 + Redis `user:routing:{user_uuid}` | 当前某台设备是否正在某个 connect 节点上保持 WebSocket 连接。 |

因此：

- 设备存在，不代表当前在线；
- 在线连接断开，不会删除设备会话事实；
- 在线状态查询依赖设备会话和 connect 路由的组合判断。

## 3. 建连前置条件

前提：用户已通过登录接口拿到 `accessToken`，并且该 Token 哈希已经写入 Redis。

前端握手 URL：

```text
GET /ws?token=<access_token>&device_id=<device_id>
```

必要条件：

1. `device_id` 与登录时保持一致；
2. `accessToken` 未过期；
3. Redis 中 `auth:at:{user_uuid}:{device_id}` 仍存在且匹配当前 token；
4. connect `Origin` 白名单允许当前前端域名。

## 4. 建连成功链路

1. connect 从 query 读取 `token` 和 `device_id`。
2. connect 解析 JWT，校验 claims 中的 `user_uuid`、`device_id`。
3. connect 读取 Redis `auth:at:{user_uuid}:{device_id}` 校验 token MD5。
4. connect 完成 WebSocket 升级。
5. connect 将连接注册到本节点连接管理器。
6. 若同一 `user_uuid + device_id` 已有旧连接，则关闭旧连接，仅保留最新连接。
7. connect 调用 `OnConnect`：
   - 无条件写入在线路由 `user:routing:{user_uuid}`；
   - 异步上报 auth 设备状态为在线。

## 5. 在线路由格式

Redis 结构：

```text
key   = user:routing:{user_uuid}
field = {device_id}
value = {connectGrpcAddr}|{lastActiveMs}
TTL   = CONNECT_ROUTE_TTL_SECONDS（默认 360s）
```

解释：

| 字段 | 说明 |
| --- | --- |
| `connectGrpcAddr` | 该设备所在 connect 节点 gRPC 地址。 |
| `lastActiveMs` | 最近活跃时间，Unix 毫秒；新鲜度约等于心跳周期。 |

该路由是 presence 契约的事实源：message-push 用它做推送寻址（窗口
`MESSAGE_PUSH_ROUTE_TTL_SECONDS`，默认 360s），auth 用它做在线判定（窗口
`PRESENCE_ONLINE_WINDOW_SECONDS`，默认 120s），两者统一经 `pkg/presence` 读取。

## 6. 心跳生命周期

当前服务端支持上行 `heartbeat` 二进制帧。

流程：

1. 客户端发送 `heartbeat`。
2. connect 处理 `OnHeartbeat`。
3. connect **无条件**刷新路由 `lastActiveMs` 和 TTL（不做任何本地节流，
   保证路由 Key 被外部删除后能被下一个心跳立即恢复）。
4. connect 返回 `heartbeat_ack`。

建议：

- 心跳间隔约 30 秒（20-60 秒均可）；
- 不要超过在线判定窗口（120 秒），否则会被判离线；
- 若连续两个心跳周期未收到响应，触发重连。

## 7. 断开连接链路

发生场景：

- 用户主动关闭页面；
- 网络断开；
- connect 主动关闭不可写连接；
- 同设备新连接替换旧连接；
- 后台踢线。

断开时处理：

1. connect 从连接管理器中注销该 client。
2. connect 调用 `OnDisconnect`。
3. 删除 `user:routing:{user_uuid}` 下该 `device_id` 字段。
4. 异步上报 auth 设备状态为离线。

注意：如果进程异常退出，Redis 路由也会依赖 TTL 自然过期，因此短时间内可能存在“脏路由窗口”。

## 8. 在线状态查询链路

HTTP 接口：

| 接口 | 说明 |
| --- | --- |
| `GET /api/v1/auth/user/online-status/:userUuid` | 查询单人在线状态。 |
| `POST /api/v1/auth/user/batch-online-status` | 批量查询在线状态。 |

返回字段：

| 字段 | 说明 |
| --- | --- |
| `isOnline` | 是否有在线设备。 |
| `lastSeenAt` | 最近活跃时间，RFC3339。 |
| `onlinePlatforms` | 当前在线平台列表。 |

查询语义：

- **在线 = 该用户在 presence 窗口（默认 120s）内存在存活的 WS 连接**；纯 HTTP 活跃不算在线；
- `lastSeenAt`：在线设备取路由心跳时间；离线设备取会话最后状态迁移时刻（登录/断连/登出）；
- 已登出 / 被踢终态的设备即使路由残留也不计入在线；
- 前端展示应接受短暂抖动，不要把“离线”理解为强事务事实。

## 9. 踢设备链路

入口：`DELETE /api/v1/auth/user/devices/:deviceId`

步骤：

1. gateway 识别当前登录用户。
2. auth 校验目标设备存在且不是当前设备。
3. auth 清理该设备 Token 哈希。
4. 若能定位 connect 节点，则调用 connect `KickConnection` 精确断开。
5. 设备会话状态更新为被踢或离线。

结果：

- 目标设备当前连接会被立即踢掉；
- 即使没踢掉，后续重连也会因为 Token 哈希已删除而失败。

## 10. 常见异常与自愈

| 场景 | 结果 | 自愈方式 |
| --- | --- | --- |
| Redis 短暂异常 | 新握手失败（Fail-Close） | 前端稍后重连。 |
| connect 进程异常退出 | 路由短暂残留 | 等待 TTL 过期或节点启动清理。 |
| 路由 Key 被外部删除/淘汰 | 该用户短暂显示离线、推送寻址落空 | 下一个心跳无条件重建路由。 |
| auth 设备状态上报失败 | WS 仍可工作 | 下次重连或状态同步修正。 |
| 同设备多连接并发 | 旧连接被替换 | 前端只保留单连接实例。 |

## 11. 前端实现建议

1. Web 端把 `device_id` 持久化到 localStorage，不要每次刷新重新生成。
2. 连接状态至少维护：`connecting`、`connected`、`reconnecting`、`auth_failed`。
3. 被踢线或 401 后立即清理本地 Token，避免死循环重连。
4. 设备列表页面不要把在线状态当作绝对真值，应接受秒级最终一致延迟。
