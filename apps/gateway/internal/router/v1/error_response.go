package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/utils"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/result"

	"github.com/gin-gonic/gin"
)

// handleServiceError 统一把下游服务错误转换成 gateway HTTP 响应。
//
// 错误处理仍遵循原有两条路径：
//  1. 可识别的非服务端业务错误按原业务码返回，并保持 HTTP 200；
//  2. 未知错误或服务端错误统一返回内部错误，同时挂入 Gin 错误链供日志中间件记录。
//
// 参数绑定等 gateway 本地校验仍由各 handler 就地处理；上传失败等具有专用错误码的流程也不走这里，
// 从而只合并语义完全一致的标准分支。
func handleServiceError(c *gin.Context, err error) {
	code := utils.ExtractErrorCode(err)
	if consts.IsNonServerError(code) {
		result.Fail(c, nil, code)
		return
	}
	result.FailServer(c, err, consts.CodeInternalError)
}
