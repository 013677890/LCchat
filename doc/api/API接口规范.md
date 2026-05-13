# ChatServer API 接口规范文档

> **版本**: v1.0.0

> **更新时间**: 2026-01-08

> **维护人**: 开发团队

> **数据格式**: JSON

---

## 📋 目录

- [1. 概述](#1-概述)

- [2. 通用规范](#2-通用规范)

- [3. 统一响应格式](#3-统一响应格式)

- [4. 认证机制](#4-认证机制)

- [5. 错误码规范](#5-错误码规范)

- [6. 接口列表](#6-接口列表)

  - [6.1 公开接口](#61-公开接口)

  - [6.2 用户认证接口](#62-用户认证接口)

  - [6.3 用户管理接口](#63-用户管理接口-待开发)

  - [6.4 好友关系接口](#64-好友关系接口-待开发)

  - [6.5 消息接口](#65-消息接口-待开发)

  - [6.6 群组接口](#66-群组接口-待开发)

- [7. 数据模型](#7-数据模型)

- [8. 附录](#8-附录)

---

## 1. 概述

### 1.1 系统架构

ChatServer 采用微服务架构,主要包含以下服务:

- **Gateway Service**: API 网关服务,提供 HTTP RESTful 接口

- **Connect Service**: 长连接服务,维持客户端 WebSocket 连接

- **User Service**: 用户服务,处理用户相关业务逻辑

- **Message Service**: 消息服务,处理消息存储和分发

### 1.2 环境信息

| 环境 | 基础URL | 说明 |

|------|---------|------|

| 开发环境 | `http://localhost:8080` | 本地开发测试 |

| 测试环境 | `https://test-api.chatserver.com` | 测试环境 |

| 生产环境 | `https://api.chatserver.com` | 正式环境 |

### 1.3 技术栈

- **后端框架**: Gin (Go)

- **认证方式**: JWT (JSON Web Token)

- **数据库**: MySQL 8.0+

- **缓存**: Redis 7.0+

- **消息队列**: Kafka

- **通信协议**: HTTP/1.1, WebSocket

---

## 2. 通用规范

### 2.1 请求规范

#### 2.1.1 URL 设计

- 使用小写字母和数字

- 多个单词使用连字符 `-` 连接

- 使用复数形式表示资源

示例:

```

✅ /api/v1/users

✅ /api/v1/user-groups

❌ /api/v1/getUsers

❌ /api/v1/UserInfo

```

#### 2.1.2 HTTP 方法

| 方法 | 说明 | 幂等性 |

|------|------|--------|

| GET | 获取资源 | ✅ 是 |

| POST | 创建资源 | ❌ 否 |

| PUT | 完整更新资源 | ✅ 是 |

| PATCH | 部分更新资源 | ❌ 否 |

| DELETE | 删除资源 | ✅ 是 |

#### 2.1.3 请求头

| 请求头 | 必填 | 说明 | 示例 |

|--------|------|------|------|

| Content-Type | ✅ | 请求体格式 | `application/json` |

| Authorization | ⚠️ | 认证信息 | `Bearer <token>` |

| X-Request-ID | ❌ | 请求追踪ID | `uuid` |

| X-Device-ID | ❌ | 设备唯一标识 | `device-uuid` |

| X-Client-Version | ❌ | 客户端版本 | `1.0.0` |

| User-Agent | ❌ | 客户端标识 | `ChatServer-iOS/1.0.0` |

#### 2.1.4 查询参数

- 使用驼峰命名

- 分页参数: `page`, `pageSize`

- 排序参数: `sortField`, `sortOrder` (asc/desc)

示例:

```

GET /api/v1/messages?page=1&pageSize=20&sortField=createdAt&sortOrder=desc

```

#### 2.1.5 时间格式

- 统一使用 Unix 毫秒时间戳

- 时区: UTC

示例:

```json

{

  "createdAt": "2026-01-08T10:30:00.000Z",

  "birthday": "1995-06-15"

}

```

#### 2.1.6 状态码

- 只要是业务 都返回200OK

### 2.2 版本控制

采用 URL 路径版本控制:

```

/api/v1/...

/api/v2/...

```

### 2.3 分页规范

#### 2.3.1 请求参数

| 参数 | 类型 | 必填 | 默认值 | 说明 |

|------|------|------|--------|------|

| page | int | ❌ | 1 | 页码(从1开始) |

| pageSize | int | ❌ | 20 | 每页数量(1-100) |

#### 2.3.2 响应结构

```json

{

  "code": 0,

  "message": "success",

  "data": {

    "items": [...],

    "pagination": {

      "page": 1,

      "pageSize": 20,

      "total": 100,

      "totalPages": 5

    }

  }

}

```

---

## 3. 统一响应格式

### 3.1 成功响应

```json

{

  "code": 0,

  "message": "success",

  "data": {

    // 业务数据

  },

  "timestamp": 1736344200000

}

```

### 3.2 错误响应

```json

{

  "code": 40001,

  "message": "参数验证失败",

  "errors": [

    {

      "field": "telephone",

      "message": "手机号格式不正确"

    }

  ],

  "timestamp": 1736344200000,

  "requestId": "550e8400-e29b-41d4-a716-446655440000"

}

```

### 3.3 响应字段说明

| 字段 | 类型 | 必填 | 说明 |

|------|------|------|------|

| code | int | ✅ | 业务状态码(0表示成功) |

| message | string | ✅ | 响应消息 |

| data | object/array | ❌ | 响应数据 |

| errors | array | ❌ | 详细错误信息(仅错误时) |

| timestamp | long | ✅ | Unix时间戳(毫秒) |

| requestId | string | ❌ | 请求追踪ID(仅错误时) |

---

## 4. 认证机制

### 4.1 JWT 认证流程

#### 4.1.1 登录获取 Token

1. 客户端调用登录接口

2. 服务端验证成功后返回 Access Token 和 Refresh Token

3. 客户端保存 Token

#### 4.1.2 请求携带 Token

```http

Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

```

#### 4.1.3 Token 刷新

- Access Token 过期时间: 2 小时

- Refresh Token 过期时间: 7 天

- 当 Access Token 过期时,使用 Refresh Token 获取新的 Access Token

### 4.2 Token 生成

#### 4.2.1 Access Token 结构

```json

{

  "user_uuid": "user-uuid-string",

  "device_id": "device-id-string",

  "exp": 1736347800,

  "iat": 1736344200,

  "iss": "ChatServer-Gateway"

}

```

#### 4.2.2 Token 有效期

| Token 类型 | 有效期 | 用途 |

|-----------|--------|------|

| Access Token | 2 小时 | 访问受保护的 API 接口 |

| Refresh Token | 7 天 | 刷新 Access Token |

### 4.3 认证中间件

需要认证的接口会自动验证 Token,验证失败返回:

```json

{

  "code": 401,

  "message": "Token 无效或已过期",

  "timestamp": 1736344200000

}

```

---

## 5. 错误码规范

### 5.1 错误码设计

错误码采用 5 位数字: `类型码(1位) + 业务码(2位) + 错误码(2位)`

| 类型码 | 说明 |

|--------|------|

| 1 | 客户端错误(4xx) |

| 2 | 服务端错误(5xx) |

| 9 | 系统级错误 |

### 5.2 通用错误码

| 错误码 | HTTP状态码 | 说明 |

|--------|-----------|------|

| 0 | 200 | 成功 |

| 10001 | 400 | 参数验证失败 |

| 10002 | 400 | 请求体格式错误 |

| 10003 | 404 | 资源不存在 |

| 10004 | 405 | 请求方法不允许 |

| 10005 | 429 | 请求过于频繁 |

| 10006 | 413 | 请求体过大 |

| 20001 | 401 | 未认证 |

| 20002 | 401 | Token 无效 |

| 20003 | 401 | Token 已过期 |

| 20004 | 403 | 权限不足 |

| 30001 | 500 | 服务器内部错误 |

| 30002 | 503 | 服务暂不可用 |

### 5.3 业务错误码

#### 用户模块 (1xxxx)

| 错误码 | 说明 |

|--------|------|

| 11001 | 用户不存在 |

| 11002 | 用户已存在 |

| 11003 | 密码错误 |

| 11004 | 用户已被禁用 |

| 11005 | 手机号格式错误 |

| 11006 | 验证码错误 |

| 11007 | 验证码已过期 |

#### 好友模块 (2xxxx)

| 错误码 | 说明 |

|--------|------|

| 12001 | 已经是好友 |

| 12002 | 好友申请已发送 |

| 12003 | 不存在该好友关系 |

| 12004 | 已经是黑名单 |

#### 消息模块 (3xxxx)

| 错误码 | 说明 |

|--------|------|

| 13001 | 消息不存在 |

| 13002 | 消息发送失败 |

| 13003 | 消息类型不支持 |

| 13004 | 会话不存在 |

#### 群组模块 (4xxxx)

| 错误码 | 说明 |

|--------|------|

| 14001 | 群组不存在 |

| 14002 | 不是群成员 |

| 14003 | 没有权限 |

| 14004 | 群成员已满 |

---

## 6. 接口列表

### 6.1 公开接口

#### 6.1.1 健康检查

**接口描述**: 检查服务健康状态

**请求信息**:

```

GET /health

```

**响应示例**:

```json

{

  "status": "ok"

}

```

---

### 6.2 用户认证接口

#### 6.2.1 用户登录

**接口描述**: 用户通过手机号和密码登录,返回 Access Token 和 Refresh Token

**请求信息**:

```

POST /api/v1/public/login

```

**请求头**:

```http

Content-Type: application/json

X-Device-ID: device-uuid

X-Client-Version: 1.0.0

```

**请求体**:

| 字段 | 类型 | 必填 | 说明 | 示例 |

|------|------|------|------|------|

| telephone | string | ✅ | 手机号(11位) | `13800138000` |

| password | string | ✅ | 密码(加密后) | `encrypted_password` |

| deviceInfo | object | ❌ | 设备信息 | 见下方 |

**deviceInfo 结构**:

| 字段 | 类型 | 必填 | 说明 |

|------|------|------|------|

| platform | string | ✅ | 平台(iOS/Android/Web) |

| osVersion | string | ❌ | 系统版本 |

| appVersion | string | ❌ | 应用版本 |

| deviceModel | string | ❌ | 设备型号 |

**请求示例**:

```json

{

  "telephone": "13800138000",

  "password": "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",

  "deviceInfo": {

    "platform": "iOS",

    "osVersion": "17.0",

    "appVersion": "1.0.0",

    "deviceModel": "iPhone 15 Pro"

  }

}

```

**响应示例(成功)**:

```json

{

  "code": 0,

  "message": "登录成功",

  "data": {

    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",

    "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",

    "tokenType": "Bearer",

    "expiresIn": 7200,

    "userInfo": {

      "uuid": "user-uuid-001",

      "nickname": "张三",

      "telephone": "13800138000",

      "email": "zhangsan@example.com",

      "avatar": "https://cdn.chatserver.com/avatars/user-001.jpg",

      "gender": 0,

      "signature": "个性签名",

      "birthday": "1995-06-15"

    }

  },

  "timestamp": 1736344200000

}

```

**响应示例(失败)**:

```json

{

  "code": 11003,

  "message": "密码错误",

  "timestamp": 1736344200000

}

```

**错误码**:

| 错误码 | 说明 |

|--------|------|

| 10001 | 参数验证失败 |

| 11001 | 用户不存在 |

| 11003 | 密码错误 |

| 11004 | 用户已被禁用 |

#### 6.2.2 刷新 Token

**接口描述**: 使用 Refresh Token 刷新 Access Token

**请求信息**:

```

POST /api/v1/public/refresh-token

```

**请求体**:

| 字段 | 类型 | 必填 | 说明 |

|------|------|------|------|

| refreshToken | string | ✅ | Refresh Token |

**请求示例**:

```json

{

  "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

}

```

**响应示例**:

```json

{

  "code": 0,

  "message": "刷新成功",

  "data": {

    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",

    "tokenType": "Bearer",

    "expiresIn": 7200

  },

  "timestamp": 1736344200000

}

```

**错误码**:

| 错误码 | 说明 |

|--------|------|

| 10001 | 参数验证失败 |

| 20002 | Token 无效 |

| 20003 | Token 已过期 |

#### 6.2.3 用户注册

**接口描述**: 新用户注册账号

**请求信息**:

```

POST /api/v1/public/register

```

**请求头**:

```http

Content-Type: application/json

X-Device-ID: device-uuid

```

**请求体**:

| 字段 | 类型 | 必填 | 说明 | 示例 |

|------|------|------|------|------|

| telephone | string | ✅ | 手机号(11位) | `13800138000` |

| password | string | ✅ | 密码(加密后) | `encrypted_password` |

| verifyCode | string | ✅ | 验证码 | `123456` |

| nickname | string | ❌ | 昵称 | `张三` |

**请求示例**:

```json

{

  "telephone": "13800138000",

  "password": "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",

  "verifyCode": "123456",

  "nickname": "张三"

}

```

**响应示例**:

```json

{

  "code": 0,

  "message": "注册成功",

  "data": {

    "userUuid": "user-uuid-002",

    "telephone": "13800138000",

    "nickname": "张三"

  },

  "timestamp": 1736344200000

}

```

**错误码**:

| 错误码 | 说明 |

|--------|------|

| 10001 | 参数验证失败 |

| 11002 | 用户已存在 |

| 11005 | 手机号格式错误 |

| 11006 | 验证码错误 |

| 11007 | 验证码已过期 |

#### 6.2.4 发送验证码

**接口描述**: 发送短信验证码

**请求信息**:

```

POST /api/v1/public/send-sms

```

**请求体**:

| 字段 | 类型 | 必填 | 说明 |

|------|------|------|------|

| telephone | string | ✅ | 手机号 |

| type | int | ✅ | 类型(1:注册 2:登录 3:找回密码) |

**请求示例**:

```json

{

  "telephone": "13800138000",

  "type": 1

}

```

**响应示例**:

```json

{

  "code": 0,

  "message": "发送成功",

  "timestamp": 1736344200000

}

```

**错误码**:

| 错误码 | 说明 |

|--------|------|

| 10001 | 参数验证失败 |

| 11005 | 手机号格式错误 |

| 10005 | 发送过于频繁 |

#### 6.2.5 用户登出

**接口描述**: 用户退出登录

**请求信息**:

```

POST /api/v1/auth/logout

```

**请求头**:

```http

Authorization: Bearer <access_token>

```

**响应示例**:

```json

{

  "code": 0,

  "message": "登出成功",

  "timestamp": 1736344200000

}

```

---

### 6.3 用户管理接口 (待开发)

> ⚠️ 以下接口待开发,仅作为设计参考

#### 6.3.1 获取用户信息

```

GET /api/v1/auth/user/info

```

#### 6.3.2 更新用户信息

```

PUT /api/v1/auth/user/info

```

#### 6.3.3 上传头像

```

POST /api/v1/auth/user/avatar

```

#### 6.3.4 修改密码

```

POST /api/v1/auth/user/password

```

---

### 6.4 好友关系接口 (待开发)

> ⚠️ 以下接口待开发,仅作为设计参考

#### 6.4.1 搜索用户

```

GET /api/v1/auth/user/search

```

#### 6.4.2 发送好友申请

```

POST /api/v1/auth/friends/request

```

#### 6.4.3 处理好友申请

```

PUT /api/v1/auth/friends/request/:requestId

```

#### 6.4.4 获取好友列表

```

GET /api/v1/auth/friends

```

#### 6.4.5 删除好友

```

DELETE /api/v1/auth/friends/:userUuid

```

---

### 6.5 消息接口 (待开发)

> ⚠️ 以下接口待开发,仅作为设计参考

#### 6.5.1 发送消息

```

POST /api/v1/auth/messages/send

```

#### 6.5.2 获取历史消息

```

GET /api/v1/auth/messages

```

#### 6.5.3 获取会话列表

```

GET /api/v1/auth/conversations

```

---

### 6.6 群组接口 (待开发)

> ⚠️ 以下接口待开发,仅作为设计参考

#### 6.6.1 创建群组

```

POST /api/v1/auth/groups

```

#### 6.6.2 获取群组信息

```

GET /api/v1/auth/groups/:groupUuid

```

#### 6.6.3 邀请成员

```

POST /api/v1/auth/groups/:groupUuid/members

```

---

## 7. 数据模型

### 7.1 用户信息 (UserInfo)

| 字段 | 类型 | 说明 |

|------|------|------|

| uuid | string | 用户唯一标识 |

| nickname | string | 昵称 |

| telephone | string | 手机号 |

| email | string | 邮箱 |

| avatar | string | 头像URL |

| gender | int | 性别(0:男 1:女 2:未知) |

| signature | string | 个性签名 |

| birthday | string | 生日(YYYY-MM-DD) |

| status | int | 状态(0:正常 1:禁用) |

### 7.2 设备会话 (DeviceSession)

| 字段 | 类型 | 说明 |

|------|------|------|

| userUuid | string | 用户UUID |

| deviceId | string | 设备ID |

| platform | string | 平台 |

| lastLoginTime | string | 最后登录时间 |

| onlineStatus | int | 在线状态 |

### 7.3 会话 (Conversation)

| 字段 | 类型 | 说明 |

|------|------|------|

| uuid | string | 会话UUID |

| type | int | 类型(1:单聊 2:群聊) |

| name | string | 会话名称 |

| avatar | string | 会话头像 |

| lastMessage | string | 最后一条消息 |

| lastTime | string | 最后消息时间 |

### 7.4 消息 (Message)

| 字段 | 类型 | 说明 |

|------|------|------|

| uuid | string | 消息UUID |

| conversationUuid | string | 会话UUID |

| senderUuid | string | 发送者UUID |

| content | string | 消息内容 |

| msgType | int | 消息类型(1:文本 2:图片 3:语音...) |

| sendTime | string | 发送时间 |

| status | int | 状态(0:发送中 1:已发送 2:已送达 3:已读) |

---

## 8. 附录

### 8.1 开发环境配置

```yaml

# config.yaml

server:

  port: 8080

  mode: debug

jwt:

  secret: "your-secret-key-change-in-production"

  accessExpire: 7200    # 2小时

  refreshExpire: 604800 # 7天

redis:

  addr: "localhost:6379"

  password: ""

  db: 0

mysql:

  host: "localhost"

  port: 3306

  database: "kama_chat_server"

  username: "root"

  password: "password"

```

### 8.2 密码加密规范

使用 bcrypt 加密, cost factor = 10

```go

// 示例: 密码加密

hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

// 示例: 密码验证

err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))

```

### 8.3 WebSocket 连接规范

#### 连接地址

```

ws://localhost:8081/ws?token=<access_token>&device_id=<device_id>

```

#### 消息格式

```json

{

  "type": "message",

  "data": {

    "conversationUuid": "conv-uuid",

    "content": "hello",

    "msgType": 1

  }

}

// 客户端每 30s 发送一次

{ "type": "heartbeat" }

// 服务端回复

{ "type": "heartbeat_ack" }

```

### 8.4 接口测试工具

推荐使用以下工具进行接口测试:

- **Postman**: https://www.postman.com/

- **Apifox**: https://www.apifox.cn/

- **curl**: 命令行工具

### 8.5 变更历史

| 版本 | 日期 | 变更内容 | 维护人 |

|------|------|---------|--------|

| v1.0.0 | 2026-01-08 | 初始版本,包含登录接口 | 开发团队 |

---

---

> **最后更新**: 2026-01-08
