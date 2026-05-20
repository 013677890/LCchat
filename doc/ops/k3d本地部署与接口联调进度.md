 # k3d 本地部署与接口联调进度

 ## 1．本次目标

 本轮工作的目标是：

 1．按 `doc/guides/k3s迁移方案.md` 在本地 Docker 环境落地 k3d／k3s；
 2．复用本机已有的 MySQL、Redis、Kafka、MinIO 基础设施；
 3．让 LCChat 应用层服务在 k8s 中跑起来；
 4．用黑盒脚本验证 gateway 关键接口链路。

 ## 2．已完成进度

 ### 2．1 集群与部署层

 已完成以下事项：

 - 已建立本地 k3d 集群，并使用 `lcchat-dev` 命名空间承载应用；
 - 已通过 Kubernetes `Service + Endpoints` 映射本机外部基础设施，供应用直接访问；
 - 已把应用运行模式从 `go run` 切换为预编译二进制镜像，避免 Pod 启动时现场编译导致探针失败；
 - Ingress 已可访问，当前通过 `http://127.0.0.1:8088` 并携带 `Host: lcchat.local` 访问 gateway；
 - `connect` 的 StatefulSet 已按新模板切换，基础健康检查路径可用。

 ### 2．2 代码与脚本调整

 本轮与联调直接相关的代码调整如下：

 - `scripts/gateway_blackbox_test.py`
   - 支持 `LCCHAT_BASE_URL`；
   - 支持 `LCCHAT_HOST_HEADER`；
   - 指标、上传等请求统一补 Host 头；
   - 注册场景自动生成合法手机号，避免脚本本身因参数不合法而失败。
- `pkg/grpcx/client_timeout.go`
   - 放宽 auth 相关 gRPC client 超时预算。
- `pkg/grpcx/client_timeout_test.go`
   - 同步更新超时配置断言。
- `apps/gateway/internal/router/router.go`
   - 放宽认证相关 HTTP 请求超时预算。
- `apps/auth/cmd/providers.go`
   - 放宽 auth 服务端默认 gRPC 超时。
- `apps/auth/internal/service/auth_service.go`
   - 为密码登录主链路增加分步骤耗时日志，便于定位真实慢点，而不是继续泛化为“超时问题”。
- `deploy/k8s/overlays/dev/kustomization.yaml`
   - 当前镜像标签已切到 `dev-bin-4`，用于承载带诊断日志的新一轮排查。

 ## 3．当前验证结果

 ### 3．1 编译与局部校验

 - `go test ./apps/auth/internal/service` 已通过；
 - 当前改动文件未发现新的 lint 问题。

 说明：`apps/auth/internal/service` 目录当前没有测试文件，该命令主要用于确认当前修改未破坏编译。

 ### 3．2 黑盒接口脚本进度

 目前黑盒脚本推进到以下状态：

 - `health`：通过；
 - `metrics`：通过；
 - `send-verify-code`：告警；
 - `verify-code`：通过；
 - `register-a`：通过；
 - `register-b`：通过；
 - `login-a1`：失败。

 其中：

 - `send-verify-code` 的告警主要是本地邮箱配置缺失，属于环境侧可接受问题；
 - 注册链路已经打通，说明 gateway、auth、MySQL、Redis 的基本联通性已成立；
 - 当前真正的阻塞点已经收敛到“首次密码登录链路失败”。

 ## 4．当前结论

 现阶段可以确认：

 1．问题主轴已经不再是“集群起不来”或“Ingress 不通”；
 2．问题也不太像单纯的本机网络 RTT 不足；
 3．更可能是密码登录链路内部某一步执行偏慢，或发生了只在登录路径触发的异常；
 4．为避免继续盲目放大超时，已经对 `Login` 主流程补充了分段耗时日志。

 换句话说，下一轮排查应该基于真实步骤耗时继续收敛，而不是继续把问题归因为“超时值太小”。

 ## 5．下一步建议

 下次继续时，建议按下面顺序推进：

 1．构建并导入 `lcchat:dev-bin-4`；
 2．重新应用 `deploy/k8s/overlays/dev`；
 3．重跑 `python .\scripts\gateway_blackbox_test.py`；
 4．结合 auth 日志重点观察以下阶段耗时：
   - `get_user`；
   - `compare_password`；
   - `generate_token`；
   - `store_access_token`；
   - `store_refresh_token`；
   - `upsert_session`；
   - `set_active_timestamp`。

 如果慢点最终不在 Redis／token，而落在 bcrypt、数据库查询、设备会话写入等阶段，再按真实瓶颈继续修正。

 ## 6．本次建议提交范围

 本次建议提交以下文件：

 - `apps/auth/cmd/providers.go`
 - `apps/auth/internal/service/auth_service.go`
 - `apps/gateway/internal/router/router.go`
 - `deploy/k8s/overlays/dev/kustomization.yaml`
 - `pkg/grpcx/client_timeout.go`
 - `pkg/grpcx/client_timeout_test.go`
 - `scripts/gateway_blackbox_test.py`
 - `doc/ops/k3d本地部署与接口联调进度.md`

 不建议提交以下临时产物：

 - `scripts/__pycache__/`
