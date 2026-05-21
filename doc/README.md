# LCChat 文档中心

本目录是 LCChat 当前代码状态的唯一项目文档入口。文档以仓库内实际代码、配置、协议和部署脚本为准，重点描述当前已经存在的服务边界、接口契约、核心链路、数据结构和运维方式。

## 阅读顺序

1. 总览
   - [项目总览](overview/项目总览.md)：了解项目目标、服务清单和基础设施。
   - [服务边界](overview/服务边界.md)：了解各微服务职责、数据所有权和禁止跨界规则。
   - [技术栈与目录说明](overview/技术栈与目录说明.md)：了解代码目录和公共库定位。
2. 架构协作
   - [系统微服务架构拓扑](architecture/系统微服务架构拓扑.md)
   - [服务协作总览](architecture/服务协作总览.md)
   - [调用链路与治理](architecture/调用链路与治理.md)
   - [事件驱动与最终一致性](architecture/事件驱动与最终一致性.md)
   - [Group-Outbox与缓存投影](architecture/Group-Outbox与缓存投影.md)
   - [Message整体架构](architecture/Message整体架构.md)
   - [Connect与下行推送架构](architecture/Connect与下行推送架构.md)
   - [Gateway请求治理](architecture/Gateway请求治理.md)
   - [账号认证与设备会话架构](architecture/账号认证与设备会话架构.md)
3. 接口协议
   - [接口通用规范](api/00-接口通用规范.md)
   - [认证与账号接口](api/01-认证与账号接口.md)
   - [用户资料接口](api/02-用户资料接口.md)
   - [关系接口](api/03-关系接口.md)
   - [群组接口](api/04-群组接口.md)
   - [消息与会话接口](api/05-消息与会话接口.md)
   - [WebSocket协议](api/06-WebSocket协议.md)
4. 服务细节
   - [gateway](services/gateway.md)
   - [auth](services/auth.md)
   - [user](services/user.md)
   - [relation](services/relation.md)
   - [group](services/group.md)
   - [msg](services/msg.md)
   - [message-push](services/message-push.md)
   - [connect](services/connect.md)
5. 数据与契约
   - [数据库设计](data/数据库设计.md)
   - [Redis-Key设计](data/Redis-Key设计.md)
   - [Kafka事件](data/Kafka事件.md)
   - [Protobuf契约](data/Protobuf契约.md)
6. 核心链路
   - [登录注册与Token](flows/登录注册与Token.md)
   - [设备在线与WebSocket生命周期](flows/设备在线与WebSocket生命周期.md)
   - [账号注销与异步清理](flows/账号注销与异步清理.md)
   - [好友与黑名单](flows/好友与黑名单.md)
   - [群组创建申请与成员变更](flows/群组创建申请与成员变更.md)
   - [消息发送落库与会话更新](flows/消息发送落库与会话更新.md)
   - [消息下行推送与多端同步](flows/消息下行推送与多端同步.md)
   - [消息拉取已读撤回与离线自愈](flows/消息拉取已读撤回与离线自愈.md)
7. 运维
   - [本地运行与手工测试](ops/本地运行与手工测试.md)
   - [k3s 本地部署与接口联调](ops/k3s本地部署与接口联调.md)
   - [配置说明](ops/配置说明.md)
   - [监控指标](ops/监控指标.md)
   - [MinIO对象存储](ops/MinIO对象存储.md)
   - [验证码邮件](ops/验证码邮件.md)

## 文档维护规则

任何代码变更只要触及下列内容，都必须同步更新本目录中的对应文档：

| 变更类型 | 必须更新 |
| --- | --- |
| Gateway HTTP 路由、DTO、响应结构 | `api/`、相关 `flows/`、必要时 `services/gateway.md` |
| Protobuf RPC、消息字段、枚举 | `data/Protobuf契约.md`、相关 `api/`、相关服务文档 |
| MySQL 表、索引、状态字段 | `data/数据库设计.md`、相关架构或链路文档 |
| Redis Key、TTL、缓存策略 | `data/Redis-Key设计.md`、相关服务和链路文档 |
| Kafka Topic、事件体、消费语义 | `data/Kafka事件.md`、`architecture/事件驱动与最终一致性.md` |
| 服务边界、调用关系、跨服务依赖 | `overview/服务边界.md`、`architecture/服务协作总览.md` |
| 部署、端口、环境变量、依赖组件 | `ops/配置说明.md`、`ops/本地运行与手工测试.md` |
| 消息、群组、连接、账号注销等核心流程 | `architecture/` 与 `flows/` 中对应文档 |

更新文档时必须遵循以下原则：

- 只描述当前代码事实，不记录临时方案和过程性备忘。
- 同一事实只放在一个主文档中，其他文档用链接引用，避免重复漂移。
- API、Proto、数据库、Redis、Kafka 的字段名必须与代码一致。
- 如果代码和文档不一致，以代码为准，并立即修正文档。
- 删除功能或接口时，同步删除对应文档内容，不保留过期说明。

## 事实来源

- HTTP 路由：[apps/gateway/internal/router/router.go](../apps/gateway/internal/router/router.go)
- Protobuf 契约：[proto/](../proto/)
- 错误码：[consts/const.go](../consts/const.go)
- Redis Key：[consts/redisKey/](../consts/redisKey/)
- 数据模型：[model/](../model/)
- 服务入口：[apps/](../apps/)
- 本地运行：[docker-compose.yml](../docker-compose.yml)
- 环境变量示例：[deploy/env/chatserver.env.example](../deploy/env/chatserver.env.example)
