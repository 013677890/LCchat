 # Gateway HTTP 接口测试记录

 ## 1. 记录说明

 - 记录时间：2026-05-05
- 测试入口：`scripts/tmp_api_interface_log_test_v3.ps1`
 - 校验方式：通过 PowerShell 调用 Gateway HTTP 接口，并依据 `trace_id` 检索 `docker compose logs` 判断是否命中对应容器日志。
 - 说明：原计划由脚本在每个接口执行后自动写入本文件，但脚本在登录响应解析阶段中断，导致自动文档未落盘；本文件依据当日测试日志人工补录。

 ## 2. 今日测试结论

 今日测试已经完成到公开认证接口的前半段，最新一轮有效结果如下：

 | 接口标识 | 方法 | 路径 | 结果 | HTTP | 业务码 | 命中容器 | trace_id |
 | --- | --- | --- | --- | --- | --- | --- | --- |
 | health | GET | `/health` | PASS_NO_HEALTH_LOG | 200 | - | 无成功日志，按 health 特性放行 | `20226f89-2e54-4893-a248-c927026b27a1` |
 | metrics | GET | `/metrics` | PASS | 200 | - | `gateway-1` | `6f1de6b5-449d-480a-96a1-f5fc4fe42272` |
 | public.register.A | POST | `/api/v1/public/user/register` | PASS | 200 | 0 | `auth-1`, `gateway-1` | `1503d4f9-8e81-48c5-92b5-88f864651c78` |
 | public.register.B | POST | `/api/v1/public/user/register` | PASS | 200 | 0 | `auth-1`, `gateway-1` | `6c6660d3-81ac-42f3-98ae-0bb4bbcf0022` |
 | public.register.C | POST | `/api/v1/public/user/register` | PASS | 200 | 0 | `auth-1`, `gateway-1` | `e385dee3-de34-4a58-84ec-d2cea43542d4` |
 | public.verify-code | POST | `/api/v1/public/user/verify-code` | PASS | 200 | 0 | `auth-1`, `gateway-1` | `2c10c40f-7580-412f-8afa-ab8dbac31d73` |
 | public.login.A.dev1 | POST | `/api/v1/public/user/login` | PASS | 200 | 0 | `auth-1`, `gateway-1` | `24c137b3-8e00-4f98-8807-c136d442fe58` |

 上表表示：

 1. 请求已经成功到达 Gateway。
 2. 需要联动的认证接口已经命中 `auth` 容器日志。
 3. 至少当前这批公开接口在服务侧是可用的。

 ## 3. 本轮中断点

 在 `public.login.A.dev1` 返回成功之后，脚本进入登录响应解析阶段时中断，报错如下：

 ```text
 login A1 响应缺少 access/refresh/uuid
 ```

该问题发生在 `scripts/tmp_api_interface_log_test_v3.ps1` 的登录后处理逻辑中，症状是：

 - 登录接口本身已经返回成功；
 - `gateway-1` 与 `auth-1` 日志也已命中；
 - 但脚本没有稳定从响应体中取出：
   - `accessToken`
   - `refreshToken`
   - `userInfo.uuid`

 因此，本轮阻塞点更偏向于 **测试脚本响应解析问题**，而不是登录接口服务本身失败。

 ## 4. 当日排查过程摘要

 今天的测试过程中，前面还出现过几类历史失败，后续已经通过重跑或环境修正推进到当前阶段：

 1. `public.verify-code` 曾出现过 500。
 2. `public.register.A` 曾出现过 500。
 3. 之后通过补齐表结构、等待 breaker 冷却、重新执行后，已经推进到：
    - 注册 A/B/C 成功
    - verify-code 成功
    - login 成功

 额外处理过的环境项：

 - 确保 MySQL 中存在以下表：
   - `user_account`
   - `outbox_events`
   - `idempotent_events`
 - 通过 Redis 预置验证码，支撑注册、校验码登录、重置密码等测试前置条件。
 - 脚本中对手机号做了动态化，避免重复注册触发唯一键冲突。

 ## 5. 尚未完成的接口范围

 由于脚本在登录响应解析处中断，以下接口今天尚未完成正式验证：

 - `public.login-by-code`
 - `public.reset-password`
 - `public.refresh-token`
 - 全部 `/api/v1/auth/user/*`
 - 全部 `/api/v1/auth/friend/*`
 - 全部 `/api/v1/auth/blacklist/*`
 - 全部 `/api/v1/auth/messages/*`
 - 全部 `/api/v1/auth/conversations/*`
 - `logout / delete-account / change-password / change-email` 等后续接口

 ## 6. 下次继续时的建议

 下次继续建议按下面顺序推进：

1. 先修 `scripts/tmp_api_interface_log_test_v3.ps1` 的登录响应解析逻辑。
 2. 让脚本把 `LOGIN_A1_PARSE_FAIL|raw=...` 等诊断信息稳定写入进度日志。
 3. 修复自动文档落盘后，再从登录链路继续跑剩余接口。
 4. 每跑完一个接口，继续按 `trace_id + docker compose logs` 方式核对容器日志。

 ## 7. 本次收尾说明

 - 今日测试工作先停在“公开接口已部分验证、脚本解析待修”的状态。
 - 当前文档为阶段性记录，可作为下次继续测试的起点。

 ## 8. 测试环境收尾

 - 按本轮收尾要求，文档已先补齐到当前测试进度。
 - 测试环境随后执行关闭，不再继续跑后续接口。
 - 下次恢复时，建议先重新启动 Docker 环境，再从登录响应解析问题继续往后推进。
