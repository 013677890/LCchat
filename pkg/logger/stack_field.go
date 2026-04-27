package logger

import (
	"path/filepath"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// stackFramesMarshaler 将堆栈帧数组序列化为结构化日志字段。
type stackFramesMarshaler struct {
	frames []apperr.Frame
}

// MarshalLogArray 将堆栈帧数组序列化为结构化日志字段。
func (m stackFramesMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, frame := range m.frames {
		enc.AppendObject(stackFrameMarshaler{frame: frame})
	}
	return nil
}

// stackFrameMarshaler 将单个堆栈帧序列化为结构化日志字段。
type stackFrameMarshaler struct {
	frame apperr.Frame
}

// MarshalLogObject 将单个堆栈帧序列化为结构化日志字段。
func (m stackFrameMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("func", m.frame.Func)
	enc.AddString("file", frameFile(m.frame))
	enc.AddInt("line", m.frame.Line)
	return nil
}

// StackFrames 将原始 PC 切片输出为结构化数组字段。
func StackFrames(key string, pcs []uintptr) zap.Field {
	frames := apperr.ResolveFrames(pcs)
	if len(frames) == 0 {
		return zap.Skip()
	}
	return zap.Array(key, stackFramesMarshaler{frames: frames})
}

// frameFile 返回堆栈帧文件名。
func frameFile(frame apperr.Frame) string {
	if frame.File == "" {
		return ""
	}
	return filepath.Base(frame.File)
}
