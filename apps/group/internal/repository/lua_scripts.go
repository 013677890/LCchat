package repository

const (
	// luaSetStringIfExists 仅在 String key 已存在时覆盖值并刷新 TTL。
	//
	// 这个脚本用于 group:info 这类“整块 JSON”缓存：
	//  1. 读路径已经把完整缓存建好时，写事实才能做增量覆盖；
	//  2. key 不存在时不主动补建，避免写路径反向承担全量重建职责；
	//  3. 覆盖成功后统一刷新 TTL，和 relation 域 patch-if-exists 策略保持一致。
	luaSetStringIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
	return 1
end
return 0
`

	// luaZAddIfExists 仅在 ZSet key 已存在时追加/覆盖成员并刷新 TTL。
	//
	// 这里额外删除 __EMPTY__ 占位，保证“空集合缓存”在第一次 patch 成功后
	// 能自动恢复成真实集合，而不需要读路径先做一次重建。
	luaZAddIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('ZREM', KEYS[1], '__EMPTY__')
	redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaZRemIfExists 仅在 ZSet key 已存在时删除成员并刷新 TTL。
	//
	// 如果删除后集合为空，则补一个 __EMPTY__ 占位，避免后续每次读都穿透到 MySQL。
	luaZRemIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('ZREM', KEYS[1], ARGV[1])
	redis.call('ZREM', KEYS[1], '__EMPTY__')
	if redis.call('ZCARD', KEYS[1]) == 0 then
		redis.call('ZADD', KEYS[1], 0, '__EMPTY__')
	end
	redis.call('EXPIRE', KEYS[1], ARGV[2])
	return 1
end
return 0
`

	// luaUpsertGroupMemberIfExists 仅在群成员 Hash 已存在时更新/写入单个成员 field。
	luaUpsertGroupMemberIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('HDEL', KEYS[1], '__EMPTY__')
	redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaInsertGroupMemberIfExists 仅在群成员 Hash 已存在且 field 不存在时插入单个成员 field。
	luaInsertGroupMemberIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('HDEL', KEYS[1], '__EMPTY__')
	redis.call('HSETNX', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaRemoveGroupMemberIfExists 仅在群成员 Hash 已存在时删除单个成员 field。
	luaRemoveGroupMemberIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('HDEL', KEYS[1], ARGV[1])
	redis.call('HDEL', KEYS[1], '__EMPTY__')
	if redis.call('HLEN', KEYS[1]) == 0 then
		redis.call('HSET', KEYS[1], '__EMPTY__', ARGV[2])
	end
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`
)
