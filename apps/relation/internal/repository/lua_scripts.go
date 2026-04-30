package repository

const (
	// luaAddPendingApplyIfExists 仅在待处理申请 key 已存在时做增量更新。
	//
	// 这样可以避免缓存过期后被单条写请求“半重建”，从而保证待处理列表的完整性始终由读
	// 路径负责全量回填，写路径只做 opportunistic 增量维护。
	luaAddPendingApplyIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('ZREM', KEYS[1], '__EMPTY__')
	redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaRemovePendingApplyIfExists 仅在待处理申请 key 已存在时移除某个申请人。
	luaRemovePendingApplyIfExists = `
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

	// luaUpsertFriendMetaIfExists 仅在好友 Hash 已存在时更新/写入某个好友 field。
	luaUpsertFriendMetaIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('HDEL', KEYS[1], '__EMPTY__')
	redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaInsertFriendMetaIfExists 仅在好友 Hash 已存在且 field 不存在时插入好友元数据。
	luaInsertFriendMetaIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('HDEL', KEYS[1], '__EMPTY__')
	redis.call('HSETNX', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaRemoveFriendMetaIfExists 仅在好友 Hash 已存在时删除某个好友 field。
	luaRemoveFriendMetaIfExists = `
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

	// luaAddBlacklistIfExists 仅在黑名单 ZSet 已存在时增量加入目标用户。
	luaAddBlacklistIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	redis.call('ZREM', KEYS[1], '__EMPTY__')
	redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`

	// luaRemoveBlacklistIfExists 仅在黑名单 ZSet 已存在时移除目标用户。
	luaRemoveBlacklistIfExists = `
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
)
