package repository

import (
	"context"
	"fmt"
	"strconv"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	goredis "github.com/redis/go-redis/v9"
)

// UserGroupProjectionReference 是 READY 用户群列表中的一条反向索引。
//
// Version 是该用户在这个群上最后一次成功投影的 membership 版本，
// 读路径用它和 group:info 的 CacheVersion 比较，拒绝“反向索引领先群资料”的半完成投影。
type UserGroupProjectionReference struct {
	GroupUUID string
	Version   int64
}

// ReadVersionedHashField 在一个 Lua 快照里读取版本化 Hash 的元数据和目标 field。
//
// 返回值必须区分两种“没有这个 field”：
//   - fieldExists=false 且 cacheHit=true：缓存完整，确定没有该业务 field；
//   - cacheHit=false：缓存缺失或结构非法，调用方必须回源，不能当成非成员。
func ReadVersionedHashField(
	ctx context.Context,
	redisClient *goredis.Client,
	cacheKey, field string,
	ttlSeconds int,
	renew bool,
) (raw string, fieldExists, cacheHit bool, err error) {
	if redisClient == nil || cacheKey == "" || field == "" || ttlSeconds <= 0 {
		return "", false, false, nil
	}
	renewFlag := "0"
	if renew {
		renewFlag = "1"
	}
	result, err := goredis.NewScript(LuaReadVersionedHashField).Run(
		ctx,
		redisClient,
		[]string{cacheKey},
		GroupCacheSchemaVersion,
		field,
		ttlSeconds,
		renewFlag,
	).Slice()
	if err != nil {
		return "", false, false, repoerr.WrapRedisError(err)
	}
	if len(result) == 0 {
		return "", false, false, fmt.Errorf("%w: empty versioned hash read response", repoerr.ErrRedis)
	}

	status, err := parseLuaInt64(result[0])
	if err != nil {
		return "", false, false, fmt.Errorf("%w: invalid versioned hash read status: %v", repoerr.ErrRedis, err)
	}
	switch status {
	case 0, -1:
		return "", false, false, nil
	case 1:
	default:
		return "", false, false, fmt.Errorf("%w: unknown versioned hash read status %d", repoerr.ErrRedis, status)
	}

	if len(result) < 3 {
		return "", false, false, fmt.Errorf("%w: incomplete versioned hash read response", repoerr.ErrRedis)
	}
	existsFlag, err := parseLuaString(result[1])
	if err != nil {
		return "", false, false, fmt.Errorf("%w: invalid versioned hash exists flag: %v", repoerr.ErrRedis, err)
	}
	if _, err := parseLuaPositiveInt64(result[2]); err != nil {
		return "", false, false, fmt.Errorf("%w: invalid versioned hash version: %v", repoerr.ErrRedis, err)
	}

	switch existsFlag {
	case "0":
		if len(result) != 3 {
			return "", false, false, fmt.Errorf("%w: malformed missing-field response", repoerr.ErrRedis)
		}
		return "", false, true, nil
	case "1":
		if len(result) != 4 {
			return "", false, false, fmt.Errorf("%w: malformed present-field response", repoerr.ErrRedis)
		}
		raw, err = parseLuaString(result[3])
		if err != nil {
			return "", false, false, fmt.Errorf("%w: invalid versioned hash field value: %v", repoerr.ErrRedis, err)
		}
		return raw, true, true, nil
	default:
		return "", false, false, fmt.Errorf("%w: unknown versioned hash exists flag %q", repoerr.ErrRedis, existsFlag)
	}
}

// ReadVersionedUserGroups 原子读取 READY 用户群列表及每个活跃群的版本。
func ReadVersionedUserGroups(
	ctx context.Context,
	redisClient *goredis.Client,
	userUUID string,
	renew bool,
) ([]UserGroupProjectionReference, bool, error) {
	if redisClient == nil || userUUID == "" {
		return nil, false, nil
	}
	renewFlag := "0"
	if renew {
		renewFlag = "1"
	}
	result, err := goredis.NewScript(LuaReadVersionedUserGroups).Run(
		ctx,
		redisClient,
		[]string{rediskey.UserGroupListKey(userUUID), rediskey.UserGroupVersionKey(userUUID)},
		GroupCacheSchemaVersion,
		int(cachex.JitterTTL(rediskey.UserGroupListTTL).Seconds()),
		renewFlag,
	).Slice()
	if err != nil {
		return nil, false, repoerr.WrapRedisError(err)
	}
	if len(result) == 0 {
		return nil, false, fmt.Errorf("%w: empty user-group read response", repoerr.ErrRedis)
	}

	status, err := parseLuaInt64(result[0])
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid user-group read status: %v", repoerr.ErrRedis, err)
	}
	switch status {
	case 0, -1:
		return nil, false, nil
	case 1:
	default:
		return nil, false, fmt.Errorf("%w: unknown user-group read status %d", repoerr.ErrRedis, status)
	}

	if len(result) < 2 {
		return nil, false, fmt.Errorf("%w: incomplete user-group read response", repoerr.ErrRedis)
	}
	count, err := parseLuaInt64(result[1])
	if err != nil || count <= 0 || int64(len(result)) != 2+count*2 {
		return nil, false, fmt.Errorf("%w: malformed user-group read count", repoerr.ErrRedis)
	}

	references := make([]UserGroupProjectionReference, 0, count)
	for index := 0; index < int(count); index++ {
		groupUUID, stringErr := parseLuaString(result[2+index*2])
		if stringErr != nil || groupUUID == "" {
			return nil, false, fmt.Errorf("%w: invalid user-group uuid", repoerr.ErrRedis)
		}
		version, versionErr := parseLuaInt64(result[3+index*2])
		if versionErr != nil {
			return nil, false, fmt.Errorf("%w: invalid user-group version: %v", repoerr.ErrRedis, versionErr)
		}
		if groupUUID == UserGroupsEmptyValue {
			if count != 1 || version != 0 {
				return nil, false, fmt.Errorf("%w: malformed user-group empty sentinel", repoerr.ErrRedis)
			}
		} else if version <= 0 {
			return nil, false, fmt.Errorf("%w: non-positive user-group version", repoerr.ErrRedis)
		}
		references = append(references, UserGroupProjectionReference{GroupUUID: groupUUID, Version: version})
	}
	return references, true, nil
}

// parseLuaPositiveInt64 把 Lua 返回值解析为正整数，0 或负数视为协议错误。
func parseLuaPositiveInt64(value interface{}) (int64, error) {
	parsed, err := parseLuaInt64(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("expected positive integer, got %d", parsed)
	}
	return parsed, nil
}

// parseLuaInt64 把 Redis Lua 整数结果统一解析为 int64。
func parseLuaInt64(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported Lua integer type %T", value)
	}
}

// parseLuaString 把 Redis Lua 字符串结果统一解析为 string。
func parseLuaString(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf("unsupported Lua string type %T", value)
	}
}
