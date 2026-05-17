# MinIO 对象存储

本文说明当前项目中 MinIO 的用途、配置项、运行方式和前端/后端需要遵守的约束。事实来源为 `docker-compose.yml`、`deploy/env/chatserver.env.example` 和 gateway 头像上传逻辑。

## 1. 用途

当前 MinIO 主要用于：

1. 用户头像对象存储；
2. 对外返回可访问的头像 URL；
3. 为后续媒体消息或群头像扩展提供统一对象存储基础设施。

当前明确接入 MinIO 的公开 HTTP 能力是：

- `POST /api/v1/auth/user/avatar`

## 2. 运行配置

### 2.1 关键环境变量

| 变量 | 说明 |
| --- | --- |
| `MINIO_ENDPOINT` | MinIO API 地址，例如 `minio:9000`。 |
| `MINIO_ACCESS_KEY` | Access Key。 |
| `MINIO_SECRET_KEY` | Secret Key。 |
| `MINIO_BUCKET` | 对象桶名，默认 `chatserver`。 |
| `MINIO_BASE_URL` | 对外访问基础 URL。 |
| `MINIO_USE_SSL` | 是否使用 SSL。 |
| `MINIO_PUBLIC_READ` | 是否公开读。 |

### 2.2 Docker Compose 端口

| 端口 | 用途 |
| --- | --- |
| `9000` | MinIO API。 |
| `9001` | MinIO Console。 |

## 3. 头像上传链路

入口：`POST /api/v1/auth/user/avatar`

请求要求：

| 字段 | 要求 |
| --- | --- |
| `avatar` | multipart/form-data 文件字段。 |
| 文件大小 | 最大 2MB。 |
| 文件类型 | 仅 `image/jpeg`、`image/png`。 |

服务端处理步骤：

1. gateway 从表单读取 `avatar` 文件；
2. 校验文件大小不超过 2MB；
3. 校验 Content-Type 仅允许 jpg/png；
4. 获取 MinIO 客户端；
5. 使用当前登录用户 UUID 生成对象路径前缀；
6. 上传成功后得到对象 URL；
7. 调用 user 服务更新用户头像字段；
8. 返回 `avatarUrl`。

## 4. 对象路径规则

当前头像上传对象名格式：

```text
avatars/{user_uuid}/{timestamp}.{ext}
```

特点：

1. 以用户 UUID 分目录，便于排查；
2. 文件名基于时间戳生成；
3. 不覆盖历史对象，天然保留旧头像版本；
4. 数据库存储最终可访问 URL，而不是仅存对象 key。

## 5. 对前端的约束

1. 使用 `multipart/form-data` 上传，不要手动写死 boundary；
2. 上传前尽量在客户端压缩图片，减少超过 2MB 的失败率；
3. 只允许 JPG / PNG；
4. 上传成功后应使用返回的 `avatarUrl` 更新本地资料，而不是拼接猜测 URL。

## 6. 常见失败场景

| 场景 | 错误码 | 说明 |
| --- | --- | --- |
| 未传文件 | `10001` | 参数错误。 |
| 文件过大 | `10006` | 请求体过大。 |
| 类型不支持 | `11011` | 只允许 jpg/png。 |
| MinIO 客户端未初始化 | `30001` | 服务端配置异常。 |
| 上传失败 | `11012` | MinIO 写入失败或网络异常。 |

## 7. 运维排查点

当头像上传异常时，按顺序检查：

1. MinIO 容器是否健康；
2. `MINIO_ENDPOINT`、账号密码是否正确；
3. bucket 是否存在且可写；
4. `MINIO_BASE_URL` 是否能被前端访问；
5. gateway 日志中是否出现上传失败或类型校验失败；
6. user 资料更新是否成功。

## 8. 维护规则

1. 若扩展到群头像、媒体文件，必须更新本文的对象路径规则和访问控制说明。
2. 若修改允许的文件类型或大小限制，必须同步更新 `api/02-用户资料接口.md`。
3. 若对象改为私有读，需要同步更新 URL 生成与鉴权策略文档。
