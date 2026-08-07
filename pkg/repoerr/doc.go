// Package repoerr 统一仓储层的基础设施错误。
//
// 本包把 GORM、Redis 和数据库驱动返回的错误归一为稳定的仓储哨兵错误，
// 供 repository、domain repository 以及 service 通过 errors.Is 判断。
// 本包不承载业务错误码，不转换 gRPC status，也不生成 HTTP 响应。
//
// repoerr 与 apperr 的职责按调用方向分开：repository 或 domain repository
// 用 repoerr 收口基础设施错误；service 识别仓储语义并映射为 apperr；
// 跨服务传输仍由 apperr 与 grpcx 负责。例如：
//
//	if errors.Is(err, repoerr.ErrRecordNotFound) {
//		return apperr.New(consts.CodeUserNotFound)
//	}
//
// 公共仓储错误只在本包定义。好友申请不存在、群已解散等领域错误仍留在
// 各服务的 internal/repository 或 domain 包中。
package repoerr
