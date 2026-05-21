# Docker Compose 三副本本机验证

本文用于在本机 Docker Compose 环境验证 LCChat 应用层 3 副本运行。基础设施仍复用原有 MySQL、Redis、Kafka、Kafka Connect 和 MinIO。

## 拓扑

- `auth`、`user`、`relation`、`group`、`msg`、`gateway`、`message-push` 使用 3 副本。
- `connect` 不使用普通 scale，而是保留 `connect` 并新增 `connect-2`、`connect-3`。
- `gateway-lb` 暴露本机 `8080`，转发到 3 个 gateway 副本。
- `connect-lb` 暴露本机 `8081`，转发到 3 个 connect 节点。

覆盖文件固定 Compose project name 为 `lcchat`，这样 `gateway-lb` 可以稳定指向 `lcchat-gateway-1`、`lcchat-gateway-2`、`lcchat-gateway-3` 三个入口副本。

`connect` 需要特殊处理，因为在线路由会把 `CONNECT_SELF_GRPC_ADDR` 写入 Redis。每个 connect 节点必须写入自己的可直连 gRPC 地址：

```text
connect:9091
connect-2:9091
connect-3:9091
```

## 启动

PowerShell：

```powershell
.\scripts\docker-compose-scale-up.ps1
```

等价命令：

```powershell
docker compose -f docker-compose.yml -f docker-compose.scale.yml up -d --build --scale auth=3 --scale user=3 --scale relation=3 --scale group=3 --scale msg=3 --scale gateway=3 --scale message-push=3
```

## 查看

```powershell
docker compose -f docker-compose.yml -f docker-compose.scale.yml ps
docker compose -f docker-compose.yml -f docker-compose.scale.yml logs -f gateway-lb connect-lb message-push
```

## 验证

```powershell
curl.exe -s -o NUL -w "%{http_code}" http://127.0.0.1:8080/health
curl.exe -s -o NUL -w "%{http_code}" http://127.0.0.1:8081/health
```

然后跑接口黑盒或手动验证注册、登录、WebSocket 连接和消息下行推送。

## 停止

```powershell
docker compose -f docker-compose.yml -f docker-compose.scale.yml down
```

如需清空数据卷再重新验证：

```powershell
docker compose -f docker-compose.yml -f docker-compose.scale.yml down -v
```
