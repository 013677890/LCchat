package mq

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"context"
	"time"
)

// ==================== Redis 任务定义 ====================

type CommandType string

const (
	CmdSimple   CommandType = "simple"   // Set, Del, HSet...
	CmdPipeline CommandType = "pipeline" // 批量操作
	CmdLua      CommandType = "lua"      // Lua 脚本
)

// RedisTask 存放在 Kafka 里的消息体
type RedisTask struct {
	Type CommandType `json:"type"`

	// 场景 1: 普通命令 (如 DEL key)
	Command string        `json:"command,omitempty"` // e.g., "del", "set"
	Args    []interface{} `json:"args,omitempty"`    // e.g., ["user:1", "value"]

	// 场景 2: Pipeline (一组命令)
	PipelineCmds []RedisCmd `json:"pipeline_cmds,omitempty"`

	// 场景 3: Lua 脚本
	LuaScript string        `json:"lua_script,omitempty"` // 脚本内容或 SHA
	LuaKeys   []string      `json:"lua_keys,omitempty"`
	LuaArgs   []interface{} `json:"lua_args,omitempty"`

	// 元数据（用于追踪和重试控制）
	TraceID     string    `json:"trace_id,omitempty"`
	UserUUID    string    `json:"user_uuid,omitempty"`
	DeviceID    string    `json:"device_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	RetryCount  int       `json:"retry_count"`      // 已重试次数
	MaxRetries  int       `json:"max_retries"`      // 最大重试次数
	OriginalErr string    `json:"original_err"`     // 原始错误信息
	Source      string    `json:"source,omitempty"` // 操作来源（repo/service）
}

type RedisCmd struct {
	Command string        `json:"command"`
	Args    []interface{} `json:"args"`
}

// ==================== 构造器函数（Builder） ====================

// BuildDelTask 构造一个 DEL 任务
func BuildDelTask(key string) RedisTask {
	return RedisTask{
		Type:       CmdSimple,
		Command:    "del",
		Args:       []interface{}{key},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3, // 默认最多重试3次
	}
}

// BuildSetTask 构造一个 SET 任务
func BuildSetTask(key string, val interface{}, ttl time.Duration) RedisTask {
	args := []interface{}{key, val}
	if ttl > 0 {
		args = append(args, "EX", int(ttl.Seconds()))
	}
	return RedisTask{
		Type:       CmdSimple,
		Command:    "set",
		Args:       args,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildHSetTask 构造一个 HSET 任务
func BuildHSetTask(key, field string, value interface{}) RedisTask {
	return RedisTask{
		Type:       CmdSimple,
		Command:    "hset",
		Args:       []interface{}{key, field, value},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildHDelTask 构造一个 HDEL 任务
func BuildHDelTask(key string, fields ...string) RedisTask {
	args := []interface{}{key}
	for _, f := range fields {
		args = append(args, f)
	}
	return RedisTask{
		Type:       CmdSimple,
		Command:    "hdel",
		Args:       args,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildSAddTask 构造一个 SADD 任务
func BuildSAddTask(key string, members ...interface{}) RedisTask {
	args := []interface{}{key}
	args = append(args, members...)
	return RedisTask{
		Type:       CmdSimple,
		Command:    "sadd",
		Args:       args,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildSRemTask 构造一个 SREM 任务
func BuildSRemTask(key string, members ...interface{}) RedisTask {
	args := []interface{}{key}
	args = append(args, members...)
	return RedisTask{
		Type:       CmdSimple,
		Command:    "srem",
		Args:       args,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildZAddTask 构造一个 ZADD 任务
func BuildZAddTask(key string, score float64, member interface{}) RedisTask {
	return RedisTask{
		Type:       CmdSimple,
		Command:    "zadd",
		Args:       []interface{}{key, score, member},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildZRemTask 构造一个 ZREM 任务
func BuildZRemTask(key string, members ...interface{}) RedisTask {
	args := []interface{}{key}
	args = append(args, members...)
	return RedisTask{
		Type:       CmdSimple,
		Command:    "zrem",
		Args:       args,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildPipelineTask 构造一个 Pipeline 任务
func BuildPipelineTask(cmds []RedisCmd) RedisTask {
	return RedisTask{
		Type:         CmdPipeline,
		PipelineCmds: cmds,
		Timestamp:    time.Now(),
		RetryCount:   0,
		MaxRetries:   3,
	}
}

// BuildLuaTask 构造一个 Lua 脚本任务
func BuildLuaTask(script string, keys []string, args ...interface{}) RedisTask {
	return RedisTask{
		Type:       CmdLua,
		LuaScript:  script,
		LuaKeys:    keys,
		LuaArgs:    args,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// ==================== 链式方法 ====================

// WithContext 为任务添加上下文信息
func (t RedisTask) WithContext(ctx context.Context) RedisTask {
	if traceID := ctxmeta.TraceID(ctx); traceID != "" {
		t.TraceID = traceID
	}
	if userUUID := ctxmeta.UserUUID(ctx); userUUID != "" {
		t.UserUUID = userUUID
	}
	if deviceID := ctxmeta.DeviceID(ctx); deviceID != "" {
		t.DeviceID = deviceID
	}
	return t
}

// WithError 为任务添加错误信息
func (t RedisTask) WithError(err error) RedisTask {
	t.OriginalErr = err.Error()
	return t
}

// WithSource 为任务添加来源信息
func (t RedisTask) WithSource(source string) RedisTask {
	t.Source = source
	return t
}

// WithMaxRetries 设置最大重试次数
func (t RedisTask) WithMaxRetries(maxRetries int) RedisTask {
	t.MaxRetries = maxRetries
	return t
}
