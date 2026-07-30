package apperr

import (
	"errors"
	"sync/atomic"

	"github.com/013677890/LCchat-Backend/consts"
)

// Error 是统一业务错误对象，兼容标准库 error 链。
type Error struct {
	code    int
	message string
	cause   error
	stack   []uintptr
	logged  atomic.Int32
}

// Error 实现 error 接口，返回错误消息。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := e.message
	if msg == "" {
		msg = consts.GetMessage(e.Code())
	}
	if e.cause != nil {
		return msg + ": " + e.cause.Error()
	}
	return msg
}

// Unwrap 实现 error 接口，返回错误链中的原始错误。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Code 返回业务错误码，空对象时视为成功。
func (e *Error) Code() int {
	if e == nil {
		return consts.CodeSuccess
	}
	return normalizeCode(e.code)
}

// Message 返回业务文案，未显式指定时按业务码回退。
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	if e.message != "" {
		return e.message
	}
	return consts.GetMessage(e.Code())
}

// Stack 返回错误堆栈副本，避免外部修改内部数据。
func (e *Error) Stack() []uintptr {
	if e == nil || len(e.stack) == 0 {
		return nil
	}
	copied := make([]uintptr, len(e.stack))
	copy(copied, e.stack)
	return copied
}

// New 创建带默认业务文案的应用错误。
func New(code int) error {
	return &Error{code: normalizeCode(code), message: consts.GetMessage(normalizeCode(code))}
}

// NewWithMessage 创建带自定义文案的应用错误。
func NewWithMessage(code int, msg string) error {
	return &Error{code: normalizeCode(code), message: normalizeMessage(code, msg)}
}

// Wrap 包装原始错误并附加业务码与文案，必要时采集堆栈。
func Wrap(err error, code int, msg string) error {
	if err == nil {
		return nil
	}
	wrapped := &Error{
		code:    normalizeCode(code),
		message: normalizeMessage(code, msg),
		cause:   err,
	}
	if app := first(err); app != nil {
		wrapped.stack = app.Stack()
		if app.logged.Load() == 1 {
			wrapped.logged.Store(1)
		}
		return wrapped
	}
	wrapped.stack = callers(defaultCallersSkip)
	return wrapped
}

// WithStack 为错误补充堆栈；已有堆栈时直接返回。
func WithStack(err error) error {
	if err == nil {
		return nil
	}
	if HasStack(err) {
		return err
	}
	if app := first(err); app != nil {
		wrapped := &Error{
			code:    app.Code(),
			message: app.Message(),
			cause:   app.cause,
			stack:   callers(defaultCallersSkip),
		}

		if app.logged.Load() == 1 {
			wrapped.logged.Store(1)
		}
		return wrapped
	}

	return &Error{
		code:    consts.CodeInternalError,
		message: consts.GetMessage(consts.CodeInternalError),
		cause:   err,
		stack:   callers(defaultCallersSkip),
	}
}

// MarkLogged 标记错误已记录日志，避免重复打印。
func MarkLogged(err error) error {
	if err == nil {
		return nil
	}
	if app := first(err); app != nil {
		app.logged.Store(1)
	}
	return err
}

// IsLogged 判断错误是否已被标记为已记录日志。
func IsLogged(err error) bool {
	if app := first(err); app != nil {
		return app.logged.Load() == 1
	}
	return false
}

// Code 提取错误链中的业务错误码。
func Code(err error) int {
	if err == nil {
		return consts.CodeSuccess
	}
	if app := first(err); app != nil {
		return app.Code()
	}
	return consts.CodeInternalError
}

// Message 提取错误链中的业务文案。
func Message(err error) string {
	if err == nil {
		return ""
	}
	if app := first(err); app != nil {
		return app.Message()
	}
	return consts.GetMessage(Code(err))
}

// HasStack 判断错误是否携带堆栈。
func HasStack(err error) bool {
	return len(Frames(err)) > 0
}

// Frames 提取错误中的原始堆栈 PC 切片。
func Frames(err error) []uintptr {
	if app := first(err); app != nil {
		return app.Stack()
	}
	return nil
}

// normalizeCode 归一化错误码，0 统一转为内部错误码。
func normalizeCode(code int) int {
	if code == 0 {
		return consts.CodeInternalError
	}
	return code
}

// normalizeMessage 归一化错误文案，空文案时按业务码回退。
func normalizeMessage(code int, msg string) string {
	if msg != "" {
		return msg
	}
	return consts.GetMessage(normalizeCode(code))
}

// first 从错误链中提取第一个应用错误对象。
func first(err error) *Error {
	var app *Error
	if errors.As(err, &app) {
		return app
	}
	return nil
}
