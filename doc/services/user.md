# user 服务

user 服务拥有用户公开资料事实，负责资料维护、搜索、头像、二维码和资料卡片聚合。

## 职责

- 创建和维护 `user_profile`。
- 获取本人资料、他人资料、批量资料卡片。
- 用户搜索，返回基础展示字段和关系标识。
- 头像 URL 更新和二维码 token 管理。
- 消费账号注册和注销事件，保持资料域最终一致。

## 启动与核心目录

| 路径 | 说明 |
| --- | --- |
| `apps/user/cmd` | gRPC 服务、Kafka Producer 和消费者装配。 |
| `apps/user/internal/service` | 用户资料业务逻辑。 |
| `apps/user/internal/repository` | MySQL、Redis、MinIO 相关访问。 |
| `apps/user/internal/consumer` | `user_created`、`account.deleted` 等事件消费者。 |
| `proto/user` | UserService 和内部资料服务契约。 |

## 数据所有权

| 数据 | 存储 |
| --- | --- |
| 用户资料 | MySQL `user_profile`。 |
| 用户资料缓存 | Redis `user:profile:{uuid}`。 |
| 二维码 token | Redis `user:qrcode:*`。 |
| 头像对象 | MinIO bucket。 |

## 暴露的 gRPC 服务

| 服务 | 能力 |
| --- | --- |
| `UserService` | 资料读取、更新、搜索、头像、二维码、批量资料。 |
| `InternalProfileService` | 内部创建资料、批量用户卡片、公开资料聚合。 |

## 事件

| 事件 | 方向 | 说明 |
| --- | --- | --- |
| `user_created` | 消费 | 注册成功后初始化 `user_profile`。 |
| `profile_display_changed` | 生产 | 昵称或头像变化后通知 auth 回写登录展示冗余字段。 |
| `account.deleted` | 消费 | 账号注销后资料域按当前策略清理或标记。 |

## 协作关系

- Gateway 通过 UserService 暴露 HTTP 资料接口。
- auth 通过 InternalAuthService 接收资料展示字段回写。
- relation、group、message-push 等需要展示卡片时应使用 user 能力或已有投影，不直接查 `user_profile`。

## 不变量

- user 不存储密码、Token、账号安全状态。
- 昵称和头像的权威数据在 `user_profile`，auth 中只保留登录返回需要的冗余字段。
- 二维码 token 过期后必须重新生成或重新获取。
