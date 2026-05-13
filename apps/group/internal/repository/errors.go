package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Group repository 层统一错误定义。
//
// 这里沿用项目内其它服务的风格，把底层 gorm 错误先归一化，
// 方便 service 层只关心“记录不存在 / 数据库错误”这类业务语义，
// 避免上层直接感知 ORM 细节。
var (
	// ErrRecordNotFound 表示查询目标不存在。
	ErrRecordNotFound = errors.New("record not found")
	// ErrDuplicateKey 预留给未来写操作使用；当前只读阶段虽然不会命中，
	// 但先统一好错误模型，后续补写接口时不需要再改 service 侧判断风格。
	ErrDuplicateKey = errors.New("duplicate key")
	// ErrGroupDismissed 表示群已解散。
	ErrGroupDismissed = errors.New("group dismissed")
	// ErrNoPermission 表示当前操作者没有执行群管理操作的权限。
	ErrNoPermission = errors.New("no permission")
	// ErrCannotKickOwner 表示不能移除群主。
	ErrCannotKickOwner = errors.New("cannot kick owner")
	// ErrCannotQuitAsOwner 表示群主不能主动退群。
	ErrCannotQuitAsOwner = errors.New("cannot quit as owner")
	// ErrGroupMemberNotFound 表示目标成员不存在或已不在群内。
	ErrGroupMemberNotFound = errors.New("group member not found")
	// ErrDatabase 表示通用数据库错误。
	ErrDatabase = errors.New("database error")
	// ErrRedisNil 表示 Redis key 不存在。
	ErrRedisNil = errors.New("redis: key not found")
	// ErrRedis 表示 Redis 通用错误。
	ErrRedis = errors.New("redis error")
	// ErrInvalidProjectorPayload 表示 group.cache 事件内容不满足投影所需最小约束。
	//
	// 这类错误通常不是基础设施抖动，而是事件内容本身不完整；
	// 上层 consumer 可以据此决定记录告警后跳过，而不是无限重试同一坏消息。
	ErrInvalidProjectorPayload = errors.New("invalid projector payload")
)
var dbErrorRules = map[error]error{
	gorm.ErrRecordNotFound: ErrRecordNotFound,
	gorm.ErrDuplicatedKey:  ErrDuplicateKey,
}

// WrapDBError 把底层 gorm 错误映射为 group repository 统一错误。
//
// 规则：
//  1. 已知错误映射成稳定语义；
//  2. 未知错误统一包成 ErrDatabase，并保留原始错误文本供日志排查。
func WrapDBError(err error) error {
	if err == nil {
		return nil
	}
	for source, target := range dbErrorRules {
		if errors.Is(err, source) {
			return target
		}
	}
	return fmt.Errorf("%w: %v", ErrDatabase, err)
}

// WrapRedisError 把底层 Redis 错误映射为 group repository 统一错误。
func WrapRedisError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return ErrRedisNil
	}
	return fmt.Errorf("%w: %v", ErrRedis, err)
}

// LogRedisError 统一记录 Redis 降级日志。
func LogRedisError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.Warn(ctx, "Redis 操作错误，已降级处理", logger.ErrorField("error", err))
}
