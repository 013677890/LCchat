package apperr

import (
	"strconv"

	"github.com/013677890/LCchat-Backend/consts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

const (
	errorInfoReason = "BIZ_ERROR"
	errorInfoDomain = "lcchat"
	metadataBizCode = "biz_code"
)

// ToStatus 将应用错误转换为对外暴露的 gRPC status。
func ToStatus(err error) error {
	if err == nil {
		return nil
	}
	code := Code(err)
	st := status.New(grpcCodeForBizCode(code), Message(err))
	detail := &errdetails.ErrorInfo{
		Reason: errorInfoReason,
		Domain: errorInfoDomain,
		Metadata: map[string]string{
			metadataBizCode: strconv.Itoa(code),
		},
	}
	withDetails, detailErr := st.WithDetails(detail)
	if detailErr != nil {
		return st.Err()
	}
	return withDetails.Err()
}

// FromStatus 将 gRPC status 还原为统一应用错误。
func FromStatus(err error) error {
	if err == nil {
		return nil
	}
	// 尝试从 status 中提取业务码与文案
	st, ok := status.FromError(err)
	if !ok {
		// 如果 status 提取失败，则尝试从错误链中提取应用错误对象
		if app := first(err); app != nil {
			return err
		}
		// 如果错误链中没有应用错误对象，则创建一个内部错误
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
