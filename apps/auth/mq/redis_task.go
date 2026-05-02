package mq

import "github.com/013677890/LCchat-Backend/pkg/redisretry"

type CommandType = redisretry.CommandType

const (
	CmdSimple   = redisretry.CmdSimple
	CmdPipeline = redisretry.CmdPipeline
	CmdLua      = redisretry.CmdLua
)

type RedisTask = redisretry.RedisTask
type RedisCmd = redisretry.RedisCmd

var (
	BuildDelTask      = redisretry.BuildDelTask
	BuildSetTask      = redisretry.BuildSetTask
	BuildHSetTask     = redisretry.BuildHSetTask
	BuildHDelTask     = redisretry.BuildHDelTask
	BuildSAddTask     = redisretry.BuildSAddTask
	BuildSRemTask     = redisretry.BuildSRemTask
	BuildZAddTask     = redisretry.BuildZAddTask
	BuildZRemTask     = redisretry.BuildZRemTask
	BuildPipelineTask = redisretry.BuildPipelineTask
	BuildLuaTask      = redisretry.BuildLuaTask
)
