package apperr

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/013677890/LCchat-Backend/consts"
)

// NewFromPanic 专用于 recovery 中间件，避免 recover 之后普通采栈导致位移。
func NewFromPanic(recoverVal any) error {
	return &Error{
		code:    consts.CodeInternalError,
		message: consts.GetMessage(consts.CodeInternalError),
		cause:   fmt.Errorf("panic: %v", recoverVal),
		stack:   capturePanicStack(),
	}
}

// capturePanicStack 采集并过滤 panic 场景下的堆栈帧。
func capturePanicStack() []uintptr {
	const skip = 3
	pcs := callers(skip)
	if len(pcs) == 0 {
		return nil
	}
	filtered := make([]uintptr, 0, len(pcs))
	for _, pc := range pcs {
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			filtered = append(filtered, pc)
			continue
		}
		name := fn.Name()
		if shouldSkipPanicFrame(name) {
			continue
		}
		filtered = append(filtered, pc)
	}

	if len(filtered) == 0 {
		return pcs
	}
	return filtered
}

// shouldSkipPanicFrame 判断当前帧是否属于应剔除的 runtime/中间件包装层。
func shouldSkipPanicFrame(name string) bool {
	if name == "runtime.gopanic" || name == "runtime.sigpanic" {
		return true
	}
	if strings.Contains(name, "RecoveryUnaryInterceptor") || strings.Contains(name, "RecoverMiddleware") || strings.Contains(name, "GinRecovery") {
		return true
	}
	return false
}
