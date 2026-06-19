package apperr

import (
	"strconv"

	"github.com/013677890/LCchat-Backend/consts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	errorInfoReason = "BIZ_ERROR"
	errorInfoDomain = "lcchat"
	metadataBizCode = "biz_code"
)

// newBizStatus 按业务码构造携带 biz_code 明细的 gRPC status。
// ToStatus 与 GRPCStatus 共用，确保两条路径产出的 status 结构一致。
func newBizStatus(grpcCode codes.Code, message string, bizCode int) *status.Status {
	st := status.New(grpcCode, message)
	detail := &errdetails.ErrorInfo{
		Reason: errorInfoReason,
		Domain: errorInfoDomain,
		Metadata: map[string]string{
			metadataBizCode: strconv.Itoa(bizCode),
		},
	}
	if withDetails, err := st.WithDetails(detail); err == nil {
		return withDetails
	}
	return st
}

// GRPCStatus 实现 grpc-go 识别的 interface{ GRPCStatus() *status.Status }。
// 作用：当 *apperr.Error 未经 ErrorNormalize 边界就直接返回给 gRPC 框架时
// （例如 Validate / Timeout 拦截器位于归一化层之外、或 panic 恢复路径），
// 框架会调用本方法取得正确的传输层 code，而不再兜底成 codes.Unknown
// —— 后者会触发客户端默认重试、被熔断器计为基础设施失败、并使网关拿不到业务码。
// 为与 ErrorNormalize 的脱敏边界保持一致，对外只暴露业务码标准文案，
// 不携带内部 cause 链细节（如 PGV 校验原文中的字段名）。
func (e *Error) GRPCStatus() *status.Status {
	if e == nil {
		return nil
	}
	code := e.Code()
	return newBizStatus(grpcCodeForBizCode(code), consts.GetMessage(code), code)
}

// ToStatus 将应用错误转换为对外暴露的 gRPC status。
func ToStatus(err error) error {
	if err == nil {
		return nil
	}
	code := Code(err)
	return newBizStatus(grpcCodeForBizCode(code), Message(err), code).Err()
}

// FromStatus 将 gRPC status 还原为统一应用错误。
func FromStatus(err error) error {
	if err == nil {
		return nil
	}
	// 错误链中本就携带应用错误对象时直接复用，保留 code/message/stack/logged，
	// 避免经 status 往返丢失堆栈与"已记录日志"标记。
	// 注意：自 *apperr.Error 实现 GRPCStatus() 起，status.FromError 会对裸 apperr
	// 返回 ok=true，因此这层判断必须前置，否则本地错误会被错误地走远端 status 还原路径。
	if app := first(err); app != nil {
		return err
	}

	// 尝试从 status 中提取业务码与文案
	st, ok := status.FromError(err)
	if !ok {
		// 错误链中没有应用错误对象，创建一个内部错误
		return Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}

	// 尝试从 status 的 details 中提取业务码
	code := 0
	for _, detail := range st.Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok {
			continue
		}
		if rawCode, exists := info.Metadata[metadataBizCode]; exists {
			if parsed, parseErr := strconv.Atoi(rawCode); parseErr == nil {
				code = parsed
				break
			}
		}
	}
	if code == 0 {
		// 如果业务码提取失败，则尝试从 status 的 message 中提取业务码
		if parsed, parseErr := strconv.Atoi(st.Message()); parseErr == nil {
			code = parsed
		}
	}
	if code == 0 {
		// 如果业务码提取失败，则尝试从 status 的 code 中提取业务码
		code = bizCodeFromGRPCCode(st.Code())
	}
	// 创建一个应用错误对象
	return NewWithMessage(code, st.Message())
}

// Sanitize 仅保留业务码与业务文案，剥离内部细节。
func Sanitize(err error) error {
	if err == nil {
		return nil
	}
	code := Code(err)
	clean := NewWithMessage(code, consts.GetMessage(code))
	if IsLogged(err) {
		MarkLogged(clean)
	}
	return clean
}
