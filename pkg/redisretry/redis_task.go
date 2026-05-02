package redisretry

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
)

// CommandType 表示 Redis 重试任务的命令类型。
type CommandType string

const (
	CmdSimple   CommandType = "simple"
	CmdPipeline CommandType = "pipeline"
	CmdLua      CommandType = "lua"
)

// RedisTask 表示投递到 Kafka 的 Redis 重试任务。
type RedisTask struct {
	Type CommandType `json:"type"`

	// 场景 1：普通命令，如 DEL / SET / HSET。
	Command string        `json:"command,omitempty"`
	Args    []interface{} `json:"args,omitempty"`

	// 场景 2：Pipeline 命令集合。
	PipelineCmds []RedisCmd `json:"pipeline_cmds,omitempty"`

	// 场景 3：Lua 脚本任务。
	LuaScript string        `json:"lua_script,omitempty"`
	LuaKeys   []string      `json:"lua_keys,omitempty"`
	LuaArgs   []interface{} `json:"lua_args,omitempty"`

	// 元数据字段用于追踪来源和控制重试。
	TraceID     string    `json:"trace_id,omitempty"`
	UserUUID    string    `json:"user_uuid,omitempty"`
	DeviceID    string    `json:"device_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	RetryCount  int       `json:"retry_count"`
	MaxRetries  int       `json:"max_retries"`
	OriginalErr string    `json:"original_err"`
	Source      string    `json:"source,omitempty"`
}

// RedisCmd 表示 Pipeline 中的一条 Redis 命令。
type RedisCmd struct {
	Command string        `json:"command"`
	Args    []interface{} `json:"args"`
}

// BuildDelTask 构造 DEL 重试任务。
func BuildDelTask(key string) RedisTask {
	return RedisTask{
		Type:       CmdSimple,
		Command:    "del",
		Args:       []interface{}{key},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}
}

// BuildSetTask 构造 SET 重试任务。
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

// BuildHSetTask 构造 HSET 重试任务。
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

// BuildHDelTask 构造 HDEL 重试任务。
func BuildHDelTask(key string, fields ...string) RedisTask {
	args := []interface{}{key}
	for _, field := range fields {
		args = append(args, field)
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

// BuildSAddTask 构造 SADD 重试任务。
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

// BuildSRemTask 构造 SREM 重试任务。
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

// BuildZAddTask 构造 ZADD 重试任务。
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

// BuildZRemTask 构造 ZREM 重试任务。
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

// BuildPipelineTask 构造 Pipeline 重试任务。
func BuildPipelineTask(cmds []RedisCmd) RedisTask {
	return RedisTask{
		Type:         CmdPipeline,
		PipelineCmds: cmds,
		Timestamp:    time.Now(),
		RetryCount:   0,
		MaxRetries:   3,
	}
}

// BuildLuaTask 构造 Lua 脚本重试任务。
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

// WithContext 为任务补充 trace/user/device 等上下文信息。
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

// WithError 为任务记录原始错误。
func (t RedisTask) WithError(err error) RedisTask {
	t.OriginalErr = err.Error()
	return t
}

// WithSource 为任务记录调用来源。
func (t RedisTask) WithSource(source string) RedisTask {
	t.Source = source
	return t
}

// WithMaxRetries 为任务覆盖默认最大重试次数。
func (t RedisTask) WithMaxRetries(maxRetries int) RedisTask {
	t.MaxRetries = maxRetries
	return t
}
