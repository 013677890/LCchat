# Gateway HTTP 接口测试记录

- 开始时间: 2026-05-07 18:04:55
- 基地址: http://127.0.0.1:8080
- 说明: 每完成一个接口测试后立即追加一条记录；密码、验证码、Token 已脱敏。

## health
- 时间: 2026-05-07 18:05:54
- 请求: GET /health
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: (none); body: (empty)
- 期望类型: health_success
- 结果: PASS_NO_HEALTH_LOG
- HTTP 状态: 200
- 业务码: 
- 响应摘要: {"status":"ok"}
- 命中服务: 
- trace_id: bc3d5ba8-91f4-4329-8961-a67fff90e139
- 日志摘要: (empty)

## metrics
- 时间: 2026-05-07 18:05:55
- 请求: GET /metrics
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: (none); body: (empty)
- 期望类型: health_success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 
- 响应摘要: # HELP gateway_grpc_request_duration_seconds gRPC request latency distributions in seconds
# TYPE gateway_grpc_request_duration_seconds histogram
gateway_grpc_request_duration_seconds_bucket{method="BatchCheckIsFriend",service="relation.FriendService",le="0.005"} 5
gateway_grpc_request_duration_seconds_bucket{method="BatchCheckIsFriend",service="relation.FriendService",le="0.01"} 5
gateway_grpc_request_duration_seconds_bucket{method="BatchCheckIsFriend",service="relation.FriendService",le="0.025"} 5
gateway_grpc_request_duration_seconds_bucket{method="BatchCheckIsFriend",service="relation.FriendService",le="0.05"} 5
gateway_grpc_request_duration_seconds_bucket{method="BatchCheckIsFriend",ser...
- 命中服务: gateway-1
- trace_id: c6130df2-f637-4cf5-8882-3e520dad6702
- 日志摘要: gateway-1  | {"level":"info","ts":"2026-05-07T10:05:54.069451657Z","caller":"middleware/gin_logger.go:78","msg":"Gateway HTTP 请求成功","method":"GET","path":"/metrics","query":"","ip":"172.23.0.1","status":200,"cost":8,"trace_id":"c6130df2-f637-4cf5-8882-3e520dad6702"}

## public.register.A
- 时间: 2026-05-07 18:06:00
- 请求: POST /api/v1/public/user/register
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"nickname":"TestA","telephone":"13778148346","verifyCode":"***","password":"***","email":"ta1778148345@example.com"}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"userUuid":"2052329336836460544","email":"ta1778148345@example.com","telephone":"13778148346","nickname":"TestA"},"trace_id":"32f7a356-f949-488a-a9de-3a6dbb10ee1b","timestamp":1778148358}
- 命中服务: auth-1, gateway-1
- trace_id: 32f7a356-f949-488a-a9de-3a6dbb10ee1b
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:05:58.767145456Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/Register","cost":76,"grpc_code":"OK","trace_id":"32f7a356-f949-488a-a9de-3a6dbb10ee1b"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:05:58.76832268Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/Register","service":"auth...

## public.register.B
- 时间: 2026-05-07 18:06:05
- 请求: POST /api/v1/public/user/register
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"nickname":"TestB","telephone":"13778148347","verifyCode":"***","password":"***","email":"tb1778148345@example.com"}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"userUuid":"2052329357124308992","email":"tb1778148345@example.com","telephone":"13778148347","nickname":"TestB"},"trace_id":"b7b71a04-d67f-4267-b92e-a934c93b4844","timestamp":1778148363}
- 命中服务: auth-1, gateway-1
- trace_id: b7b71a04-d67f-4267-b92e-a934c93b4844
- 日志摘要: gateway-1  | {"level":"info","ts":"2026-05-07T10:06:03.598817975Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/Register","service":"auth:9090","cost":70,"grpc_code":"OK","trace_id":"b7b71a04-d67f-4267-b92e-a934c93b4844"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:03.599169265Z","caller":"middleware/gin_logger.go:78","msg":"Gateway HTTP 请求成功","method":"POST...

## public.register.C
- 时间: 2026-05-07 18:06:09
- 请求: POST /api/v1/public/user/register
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"nickname":"TestC","telephone":"13778148348","verifyCode":"***","password":"***","email":"tc1778148345@example.com"}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"userUuid":"2052329377558958080","email":"tc1778148345@example.com","telephone":"13778148348","nickname":"TestC"},"trace_id":"4edd32cf-074d-4438-88c2-9986fd2d8110","timestamp":1778148368}
- 命中服务: auth-1, gateway-1
- trace_id: 4edd32cf-074d-4438-88c2-9986fd2d8110
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:06:08.474289584Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/Register","cost":80,"grpc_code":"OK","trace_id":"4edd32cf-074d-4438-88c2-9986fd2d8110"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:08.475116069Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/Register","service":"aut...

## public.verify-code
- 时间: 2026-05-07 18:06:14
- 请求: POST /api/v1/public/user/verify-code
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: (none); body: {"verifyCode":"***","type":1,"email":"tv1778148345@example.com"}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"valid":true},"trace_id":"5ea9b0d1-8da2-42f8-8789-b0de99540f53","timestamp":1778148373}
- 命中服务: auth-1, gateway-1
- trace_id: 5ea9b0d1-8da2-42f8-8789-b0de99540f53
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:06:13.190419984Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/VerifyCode","cost":0,"grpc_code":"OK","trace_id":"5ea9b0d1-8da2-42f8-8789-b0de99540f53"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:13.190756845Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/VerifyCode","service":"...

## public.login.A.dev1
- 时间: 2026-05-07 18:06:20
- 请求: POST /api/v1/public/user/login
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"account":"ta1778148345@example.com","password":"***","deviceInfo":{"appVersion":"1.0.0","osVersion":"10","platform":"Windows","deviceName":"A1"}}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"accessToken":"***","refreshToken":"***","tokenType":"Bearer","expiresIn":7200,"userInfo":{"uuid":"2052329336836460544","nickname":"TestA","avatar":""}},"trace_id":"d99cf2e1-3467-4ba7-9130-b0f99e23de98","timestamp":1778148379}
- 命中服务: auth-1, gateway-1
- trace_id: d99cf2e1-3467-4ba7-9130-b0f99e23de98
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:06:19.562431882Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/Login","cost":69,"grpc_code":"OK","trace_id":"d99cf2e1-3467-4ba7-9130-b0f99e23de98","device_id":"dev-a1"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:19.562846857Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/Login"...

## public.login.A.dev2.setup
- 时间: 2026-05-07 18:06:22
- 请求: POST /api/v1/public/user/login
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"account":"ta1778148345@example.com","password":"***","deviceInfo":{"appVersion":"1.0.0","osVersion":"browser","platform":"Web","deviceName":"A2"}}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"accessToken":"***","refreshToken":"***","tokenType":"Bearer","expiresIn":7200,"userInfo":{"uuid":"2052329336836460544","nickname":"TestA","avatar":""}},"trace_id":"0a8097b2-1231-4d41-9931-ef7b19074862","timestamp":1778148380}
- 命中服务: auth-1, gateway-1
- trace_id: 0a8097b2-1231-4d41-9931-ef7b19074862
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:06:20.975982586Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/Login","cost":61,"grpc_code":"OK","trace_id":"0a8097b2-1231-4d41-9931-ef7b19074862","device_id":"dev-a2"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:20.976274367Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/Login"...

## public.login.B.dev1
- 时间: 2026-05-07 18:06:23
- 请求: POST /api/v1/public/user/login
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"account":"tb1778148345@example.com","password":"***","deviceInfo":{"appVersion":"1.0.0","osVersion":"14","platform":"Mac","deviceName":"B1"}}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"accessToken":"***","refreshToken":"***","tokenType":"Bearer","expiresIn":7200,"userInfo":{"uuid":"2052329357124308992","nickname":"TestB","avatar":""}},"trace_id":"77fb66bc-647e-42c6-b922-169fe26a1d59","timestamp":1778148382}
- 命中服务: auth-1, gateway-1
- trace_id: 77fb66bc-647e-42c6-b922-169fe26a1d59
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:06:22.382147769Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/Login","cost":62,"grpc_code":"OK","trace_id":"77fb66bc-647e-42c6-b922-169fe26a1d59","device_id":"dev-b1"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:22.382667901Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/Login"...

## public.login-by-code.C
- 时间: 2026-05-07 18:06:28
- 请求: POST /api/v1/public/user/login-by-code
- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。
- 请求摘要: headers: X-Device-ID=***; body: {"verifyCode":"***","deviceInfo":{"appVersion":"1.0.0","osVersion":"14","platform":"Android","deviceName":"C1"},"email":"tc1778148345@example.com"}
- 期望类型: success
- 结果: PASS
- HTTP 状态: 200
- 业务码: 0
- 响应摘要: {"code":0,"message":"success","data":{"accessToken":"***","refreshToken":"***","tokenType":"Bearer","expiresIn":7200,"userInfo":{"uuid":"2052329377558958080","nickname":"TestC","avatar":""}},"trace_id":"efc204c0-a34a-425c-aed3-6e4f15c8ed37","timestamp":1778148386}
- 命中服务: auth-1, gateway-1
- trace_id: efc204c0-a34a-425c-aed3-6e4f15c8ed37
- 日志摘要: auth-1     | {"level":"info","ts":"2026-05-07T10:06:26.88186773Z","caller":"grpcx/logging.go:83","msg":"gRPC 请求完成","method":"/auth.AuthService/LoginByCode","cost":12,"grpc_code":"OK","trace_id":"efc204c0-a34a-425c-aed3-6e4f15c8ed37","device_id":"dev-c1"} || gateway-1  | {"level":"info","ts":"2026-05-07T10:06:26.882179737Z","caller":"middleware/grpc_logger.go:40","msg":"Gateway gRPC 请求成功","method":"/auth.AuthService/L...

