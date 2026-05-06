# Gateway HTTP 接口测试记录

- 开始时间: 2026-05-06 18:47:04
- 基地址: http://127.0.0.1:8080
- 说明: 每完成一个接口测试后立即追加一条记录；密码、验证码、Token 已脱敏。

## health
- 时间: 2026-05-06 18:48:07
- 请求: GET /health
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: (none); body: (empty)
- 期望类型: health_success
- 结果: FAIL
- HTTP 状态: 0
- 业务码: 
- 响应摘要: 由于目标计算机积极拒绝，无法连接。 (127.0.0.1:8080)
- 命中服务: 
- trace_id: 01d91b73-a073-40e1-b066-cbd519fd57d3
- 日志摘要: (empty)

