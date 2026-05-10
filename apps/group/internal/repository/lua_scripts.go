package repository

const (
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
