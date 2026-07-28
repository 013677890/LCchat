package utils

import (
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
)

// ExtractErrorCode 提取业务错误码。
//
// 解析顺序（无旧协议旁路）：
//  1. 错误链中的本地 apperr；
//  2. gRPC status 的 errdetails.ErrorInfo.biz_code，或无明细时的传输层码粗映射
//     （由 apperr.FromStatus 完成）。
//
// 已删除：message 纯数字业务码、把 gRPC codes.Code 数值当作业务码的历史写法。
func ExtractErrorCode(err error) int {
	if err == nil {
		return 0
	}

	if code := apperr.Code(err); code != consts.CodeInternalError {
		return code
	}

	if converted := apperr.FromStatus(err); converted != nil {
		return apperr.Code(converted)
	}

	return consts.CodeInternalError
}
