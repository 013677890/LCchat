package apperr

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	maxStackDepth      = 32
	defaultCallersSkip = 3
)

// Frame 表示一帧可结构化输出的堆栈信息。
type Frame struct {
	Func string `json:"func"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// callers 采集原始程序计数器切片。
func callers(skip int) []uintptr {
	pcs := make([]uintptr, maxStackDepth)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return nil
	}
	copied := make([]uintptr, n)
	copy(copied, pcs[:n])
	return copied
}

// StackFrames 将错误中的原始堆栈转换为结构化帧。
func StackFrames(err error) []Frame {
	return ResolveFrames(Frames(err))
}

// ResolveFrames 将原始 PC 切片解析为结构化帧。
func ResolveFrames(pcs []uintptr) []Frame {
	if len(pcs) == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs)
	resolved := make([]Frame, 0, len(pcs))
	for {
		frame, more := frames.Next()
		resolved = append(resolved, Frame{Func: frame.Function, File: frame.File, Line: frame.Line})
		if !more {
			break
		}
	}
	return resolved
}

// TopFrame 返回首个可读堆栈帧摘要，便于日志摘要展示。
func TopFrame(err error) string {
	frames := StackFrames(err)
	if len(frames) == 0 {
		return ""
	}
	top := frames[0]
	if top.File == "" {
		return top.Func
	}
	if top.Func == "" {
		return fmt.Sprintf("%s:%d", filepath.Base(top.File), top.Line)
	}
	return fmt.Sprintf("%s %s:%d", trimFunc(top.Func), filepath.Base(top.File), top.Line)
}

// trimFunc 裁剪函数全路径，仅保留可读的尾部函数名。
func trimFunc(name string) string {
	if name == "" {
		return ""
	}
	idx := strings.LastIndex(name, "/")
	if idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return name
}
