package utils

import (
	"strconv"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"google.golang.org/grpc/status"
)

// ExtractErrorCode 提取业务错误码。
//
// 兼容顺序：
// 1. 优先读取项目内统一的 apperr。
// 2. 其次解析新的 gRPC errdetails 协议。
// 3. 最后兼容旧的 message 纯数字协议和历史测试里把 grpc code 当业务码的写法。
func ExtractErrorCode(err error) int {
	if err == nil {
		return 0
	}

	if code := apperr.Code(err); code != consts.CodeInternalError {
		return code
	}

	if converted := apperr.FromStatus(err); converted != nil {
		if code := apperr.Code(converted); code != consts.CodeInternalError {
			return code
		}
	}

	if st, ok := status.FromError(err); ok {
		if parsed, parseErr := strconv.Atoi(st.Message()); parseErr == nil {
			return parsed
		}
		if int(st.Code()) >= 10000 {
			return int(st.Code())
		}
	}

	return consts.CodeInternalError
}
