package repository

const (
	// LuaSetVersionedGroupInfo 以“结构版本 + 投影版本”为栅栏原子写入 group:info。
	//
	// 返回值：
	//   1  已写入；
	//   0  当前版本更高，或普通事件与当前版本相等，拒绝重复/乱序写；
	//  -1  key 不存在且调用方要求 patch-if-exists；
	//  -2  发现旧格式/错误类型，已删除，但 patch 调用不会拿旧值继续兼容。
	//
	// ARGV[6] 只允许权威 MySQL 对账传 1：此时 incoming==current 也会重写，
	// 用于修复“版本元数据仍在、业务值已损坏”的缓存。普通 Kafka 事件必须传 0，
	// 保持同版本重放幂等，且绝不让相同版本的不同 payload 互相覆盖。
	LuaSetVersionedGroupInfo = `
local key_type = redis.call('TYPE', KEYS[1])
if type(key_type) == 'table' then key_type = key_type['ok'] end
local incompatible = false

if key_type ~= 'none' and key_type ~= 'string' then
	redis.call('DEL', KEYS[1])
	key_type = 'none'
	incompatible = true
end

if key_type == 'string' then
	local raw = redis.call('GET', KEYS[1])
	local schema, current = string.match(raw, '^(%d+)|(%d+)|')
	current = tonumber(current)
	if schema ~= ARGV[5] or current == nil then
		redis.call('DEL', KEYS[1])
		key_type = 'none'
		incompatible = true
		elseif tonumber(ARGV[1]) < current or
		       (tonumber(ARGV[1]) == current and ARGV[6] ~= '1') then
			return 0
		end
	end

if key_type == 'none' and ARGV[4] ~= '1' then
	if incompatible then return -2 end
	return -1
end

redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
return 1
`

	// LuaReplaceVersionedHash 用一段 Lua 原子完成 Hash 的版本判断、整表替换和 TTL。
	//
	// ARGV 固定头部依次为 schema、version、ttl、emptyField、emptyValue，后续是
	// field/value 成对列表。ARGV[6] 是“允许同版本权威修复”标记，业务字段从
	// ARGV[7] 开始。循环 HSET 而不使用 unpack，避免大群成员数触发 Lua 参数栈上限。
	// 旧 Hash 缺少当前 schema/version 时会被直接删除后按新格式重建。
	LuaReplaceVersionedHash = `
local key_type = redis.call('TYPE', KEYS[1])
if type(key_type) == 'table' then key_type = key_type['ok'] end
if key_type ~= 'none' and key_type ~= 'hash' then
	redis.call('DEL', KEYS[1])
	key_type = 'none'
end

	if key_type == 'hash' then
		local schema = redis.call('HGET', KEYS[1], '__SCHEMA__')
		local current = tonumber(redis.call('HGET', KEYS[1], '__VERSION__'))
		local complete = redis.call('HGET', KEYS[1], '__COMPLETE__')
		if schema ~= ARGV[1] or current == nil or complete ~= '1' then
			redis.call('DEL', KEYS[1])
		elseif tonumber(ARGV[2]) < current or
		       (tonumber(ARGV[2]) == current and ARGV[6] ~= '1') then
			return 0
		end
	end

redis.call('DEL', KEYS[1])
	redis.call('HSET', KEYS[1],
		'__SCHEMA__', ARGV[1],
		'__VERSION__', ARGV[2],
		'__COMPLETE__', '1')
	local index = 7
local written = 0
while index <= #ARGV do
	redis.call('HSET', KEYS[1], ARGV[index], ARGV[index + 1])
	written = written + 1
	index = index + 2
end
if written == 0 then
	redis.call('HSET', KEYS[1], ARGV[4], ARGV[5])
end
redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
	`

	// LuaReadVersionedHashField 原子读取版本化 Hash 的元数据与一个业务 field。
	//
	// 返回值：
	//   {0}                 key 不存在；
	//   {-1}                类型/schema/version 非法，已在同一脚本中删除；
	//   {1, "0", version}  完整 Hash 命中，但业务 field 不存在；
	//   {1, "1", version, value} 完整 Hash 与业务 field 同时命中。
	//
	// Pipeline 不能保证 EXISTS/HGET/HGET 之间不插入另一客户端的 Lua 更新，因此权限
	// 点查必须用一个脚本获取自洽快照。ARGV[4]=1 时顺便原子续期。
	LuaReadVersionedHashField = `
	local key_type = redis.call('TYPE', KEYS[1])
	if type(key_type) == 'table' then key_type = key_type['ok'] end
	if key_type == 'none' then return {0} end
	if key_type ~= 'hash' then
		redis.call('DEL', KEYS[1])
		return {-1}
	end

		local schema = redis.call('HGET', KEYS[1], '__SCHEMA__')
		local version_raw = redis.call('HGET', KEYS[1], '__VERSION__')
		local version = tonumber(version_raw)
		local complete = redis.call('HGET', KEYS[1], '__COMPLETE__')
		if schema ~= ARGV[1] or version == nil or version <= 0 or complete ~= '1' then
			redis.call('DEL', KEYS[1])
			return {-1}
		end
	local empty_value = redis.call('HGET', KEYS[1], '__EMPTY__')
		local field_count = redis.call('HLEN', KEYS[1])
		if empty_value then
			if empty_value ~= '{}' or field_count ~= 4 then
				redis.call('DEL', KEYS[1])
				return {-1}
			end
		elseif field_count <= 3 then
			-- 三个元数据 field 不能单独表示空集合；当前格式必须显式
			-- 保留 __EMPTY__，否则无法区分完整空集合和部分写入。
		redis.call('DEL', KEYS[1])
		return {-1}
	end
	if ARGV[4] == '1' then redis.call('EXPIRE', KEYS[1], ARGV[3]) end
	if redis.call('HEXISTS', KEYS[1], ARGV[2]) == 0 then
		return {1, '0', version_raw}
	end
	return {1, '1', version_raw, redis.call('HGET', KEYS[1], ARGV[2])}
	`

	// LuaUpsertVersionedHash 只在完整 Hash 已存在时，原子覆盖一批 field。
	//
	// 同一事件涉及两个成员（例如群主转让）时必须一次传入，不能循环执行两次：
	// 第一次执行已经推进 key 版本，第二次会被当作重复事件拒绝。
	LuaUpsertVersionedHash = `
	local key_type = redis.call('TYPE', KEYS[1])
	if type(key_type) == 'table' then key_type = key_type['ok'] end
if key_type == 'none' then return -1 end
if key_type ~= 'hash' then
	redis.call('DEL', KEYS[1])
	return -2
		end
		local schema = redis.call('HGET', KEYS[1], '__SCHEMA__')
		local current = tonumber(redis.call('HGET', KEYS[1], '__VERSION__'))
		local complete = redis.call('HGET', KEYS[1], '__COMPLETE__')
		if schema ~= ARGV[1] or current == nil or current <= 0 or complete ~= '1' then
			redis.call('DEL', KEYS[1])
			return -2
		end
		-- __COMPLETE__ 由 full replace 与版本元数据原子写入。patch 只需再用 HLEN
		-- 校验空/非空形态，无需 HKEYS 扫描大群，保持单成员变更 O(1)。
		local empty_value = redis.call('HGET', KEYS[1], ARGV[4])
		local field_count = redis.call('HLEN', KEYS[1])
		if (empty_value and (empty_value ~= ARGV[5] or field_count ~= 4)) or
		   (not empty_value and field_count <= 3) then
			redis.call('DEL', KEYS[1])
			return -2
		end
	if tonumber(ARGV[2]) <= current then return 0 end

	redis.call('HDEL', KEYS[1], ARGV[4])
	local index = 6
	while index <= #ARGV do
		redis.call('HSET', KEYS[1], ARGV[index], ARGV[index + 1])
		index = index + 2
end
redis.call('HSET', KEYS[1], '__VERSION__', ARGV[2])
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`

	// LuaRemoveVersionedHash 只在完整 Hash 已存在时，原子删除一批 field 并留下版本。
	//
	// 即使业务 field 被删空，__VERSION__ 仍作为 tombstone 保留；这样旧的 add/upsert
	// 事件无法在删除之后复活成员或待审批申请。
	LuaRemoveVersionedHash = `
local key_type = redis.call('TYPE', KEYS[1])
if type(key_type) == 'table' then key_type = key_type['ok'] end
if key_type == 'none' then return -1 end
if key_type ~= 'hash' then
	redis.call('DEL', KEYS[1])
	return -2
		end
		local schema = redis.call('HGET', KEYS[1], '__SCHEMA__')
		local current = tonumber(redis.call('HGET', KEYS[1], '__VERSION__'))
		local complete = redis.call('HGET', KEYS[1], '__COMPLETE__')
		if schema ~= ARGV[1] or current == nil or current <= 0 or complete ~= '1' then
			redis.call('DEL', KEYS[1])
			return -2
		end
		-- 删除同样只能作用在完整缓存上。否则 metadata-only Hash 会被 HDEL 后补成
		-- 合法 __EMPTY__，把“未知全量状态”错误升级成“权威空集合”。
		local empty_value = redis.call('HGET', KEYS[1], ARGV[4])
		local field_count = redis.call('HLEN', KEYS[1])
		if (empty_value and (empty_value ~= ARGV[5] or field_count ~= 4)) or
		   (not empty_value and field_count <= 3) then
			redis.call('DEL', KEYS[1])
			return -2
		end
	if tonumber(ARGV[2]) <= current then return 0 end

redis.call('HDEL', KEYS[1], ARGV[4])
local index = 6
while index <= #ARGV do
	redis.call('HDEL', KEYS[1], ARGV[index])
	index = index + 1
end
redis.call('HSET', KEYS[1], '__VERSION__', ARGV[2])
	if redis.call('HLEN', KEYS[1]) == 3 then
		redis.call('HSET', KEYS[1], ARGV[4], ARGV[5])
	end
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`

	// LuaPatchVersionedUserGroup 原子更新用户群 ZSet 与逐群版本 Hash。
	//
	// 单条事件可以创建这两个 key 并留下版本 tombstone，但绝不写 __READY__；
	// 因为一个群事件不能证明该用户的整个群列表已经完整。只有全量用户对账脚本
	// 能设置 READY，读路径才会把这个 ZSet 当作命中。ARGV[7]=1 仅供权威群对账
	// 在 incoming==current 时修复损坏的 ZSet；普通事件必须传 0。
	LuaPatchVersionedUserGroup = `
local ztype = redis.call('TYPE', KEYS[1])
local vtype = redis.call('TYPE', KEYS[2])
if type(ztype) == 'table' then ztype = ztype['ok'] end
if type(vtype) == 'table' then vtype = vtype['ok'] end

if (ztype == 'none' and vtype ~= 'none') or (ztype ~= 'none' and vtype == 'none') or
   (ztype ~= 'none' and ztype ~= 'zset') or (vtype ~= 'none' and vtype ~= 'hash') then
	redis.call('DEL', KEYS[1], KEYS[2])
	ztype = 'none'
	vtype = 'none'
end
if vtype == 'hash' and redis.call('HGET', KEYS[2], '__SCHEMA__') ~= ARGV[1] then
	redis.call('DEL', KEYS[1], KEYS[2])
	ztype = 'none'
	vtype = 'none'
end

	local current = tonumber(redis.call('HGET', KEYS[2], ARGV[4])) or 0
	if tonumber(ARGV[2]) < current or
	   (tonumber(ARGV[2]) == current and ARGV[7] ~= '1') then
		return 0
	end

redis.call('HSET', KEYS[2], '__SCHEMA__', ARGV[1], ARGV[4], ARGV[2])
redis.call('ZREM', KEYS[1], '__EMPTY__')
if ARGV[6] == '1' then
	redis.call('ZADD', KEYS[1], ARGV[5], ARGV[4])
else
	redis.call('ZREM', KEYS[1], ARGV[4])
end
if redis.call('ZCARD', KEYS[1]) == 0 then
	redis.call('ZADD', KEYS[1], 0, '__EMPTY__')
end
redis.call('EXPIRE', KEYS[1], ARGV[3])
redis.call('EXPIRE', KEYS[2], ARGV[3])
	return 1
	`

	// LuaReadVersionedUserGroups 原子读取用户群 ZSet 与活跃群对应版本。
	//
	// 返回 {0} 表示 miss/局部集合尚未 READY，返回 {-1} 表示类型或当前严格结构非法且
	// 已删除，成功时返回 {1, count, group_uuid, version, ...}。空集合哨兵的版本固定
	// 返回 0。读取与版本校验在一个脚本中完成，避免 ZREVRANGE 和 HGETALL 之间插入
	// member_added/member_removed，导致一次请求观察到半新半旧的反向索引。
	LuaReadVersionedUserGroups = `
	local ztype = redis.call('TYPE', KEYS[1])
	local vtype = redis.call('TYPE', KEYS[2])
	if type(ztype) == 'table' then ztype = ztype['ok'] end
	if type(vtype) == 'table' then vtype = vtype['ok'] end

	if ztype == 'none' and vtype == 'none' then return {0} end
	if ztype ~= 'zset' or vtype ~= 'hash' then
		redis.call('DEL', KEYS[1], KEYS[2])
		return {-1}
	end
	if redis.call('HGET', KEYS[2], '__SCHEMA__') ~= ARGV[1] then
		redis.call('DEL', KEYS[1], KEYS[2])
		return {-1}
	end
	if redis.call('HGET', KEYS[2], '__READY__') ~= '1' then return {0} end

	local groups = redis.call('ZREVRANGE', KEYS[1], 0, -1)
	if #groups == 0 or (#groups > 1 and groups[#groups] == '__EMPTY__') then
		redis.call('DEL', KEYS[1], KEYS[2])
		return {-1}
	end

	local result = {1, #groups}
	for _, group_uuid in ipairs(groups) do
		if group_uuid == '__EMPTY__' then
			if #groups ~= 1 then
				redis.call('DEL', KEYS[1], KEYS[2])
				return {-1}
			end
			table.insert(result, group_uuid)
			table.insert(result, '0')
		else
			local version_raw = redis.call('HGET', KEYS[2], group_uuid)
			local version = tonumber(version_raw)
			if version == nil or version <= 0 then
				redis.call('DEL', KEYS[1], KEYS[2])
				return {-1}
			end
			table.insert(result, group_uuid)
			table.insert(result, version_raw)
		end
	end
	if ARGV[3] == '1' then
		redis.call('EXPIRE', KEYS[1], ARGV[2])
		redis.call('EXPIRE', KEYS[2], ARGV[2])
	end
	return result
	`

	// LuaReconcileVersionedUserGroups 用数据库完整快照合并用户群反向索引。
	//
	// ARGV 头部为 schema、ttl，之后每四项为 group_uuid、score、version、active。
	// 对每个群独立比较版本，所以在 DB 快照与 Lua 执行之间到达的更新事件不会被
	// 旧快照覆盖；最后才写 READY=1，使读路径不会观察到半完成的列表。
	LuaReconcileVersionedUserGroups = `
local ztype = redis.call('TYPE', KEYS[1])
local vtype = redis.call('TYPE', KEYS[2])
if type(ztype) == 'table' then ztype = ztype['ok'] end
if type(vtype) == 'table' then vtype = vtype['ok'] end

if (ztype == 'none' and vtype ~= 'none') or (ztype ~= 'none' and vtype == 'none') or
   (ztype ~= 'none' and ztype ~= 'zset') or (vtype ~= 'none' and vtype ~= 'hash') then
	redis.call('DEL', KEYS[1], KEYS[2])
	ztype = 'none'
	vtype = 'none'
end
	if vtype == 'hash' and redis.call('HGET', KEYS[2], '__SCHEMA__') ~= ARGV[1] then
		redis.call('DEL', KEYS[1], KEYS[2])
		ztype = 'none'
		vtype = 'none'
	end

	-- 对账是完整修复入口，不能在当前 schema 下继续保留非法元数据。这里校验
	-- ZSet 空哨兵、READY 和版本 Hash 的全部业务 field；发现污染就整对 key 清空，
	-- 再由本次 MySQL 快照重建，不尝试解释未知保留字段。
	if vtype == 'hash' then
		local invalid = false
		local ready = redis.call('HGET', KEYS[2], '__READY__')
		if ready and ready ~= '1' then invalid = true end

		local version_fields = redis.call('HGETALL', KEYS[2])
		local field_index = 1
		while field_index <= #version_fields do
			local field = version_fields[field_index]
			local value = version_fields[field_index + 1]
			if field ~= '__SCHEMA__' and field ~= '__READY__' then
				if string.sub(field, 1, 2) == '__' or
				   tonumber(value) == nil or tonumber(value) <= 0 then
					invalid = true
					break
				end
			end
			field_index = field_index + 2
		end

		local existing_groups = redis.call('ZRANGE', KEYS[1], 0, -1)
		for _, group_uuid in ipairs(existing_groups) do
			if group_uuid == '__EMPTY__' then
				if #existing_groups ~= 1 then invalid = true break end
			elseif tonumber(redis.call('HGET', KEYS[2], group_uuid)) == nil then
				invalid = true
				break
			end
		end
		if invalid then
			redis.call('DEL', KEYS[1], KEYS[2])
		end
	end
	redis.call('HSET', KEYS[2], '__SCHEMA__', ARGV[1])

local index = 3
while index <= #ARGV do
	local group_uuid = ARGV[index]
	local score = ARGV[index + 1]
	local incoming = tonumber(ARGV[index + 2])
	local active = ARGV[index + 3]
	local current = tonumber(redis.call('HGET', KEYS[2], group_uuid)) or 0
		-- 权威 DB 快照允许 incoming==current：相同版本也必须能够修复被人工篡改、
		-- 部分丢失的 ZSet 值；只有 Redis 已持有更高版本时才保留并发新事实。
		if incoming >= current then
		redis.call('ZREM', KEYS[1], '__EMPTY__')
		if active == '1' then
			redis.call('ZADD', KEYS[1], score, group_uuid)
		else
			redis.call('ZREM', KEYS[1], group_uuid)
		end
		redis.call('HSET', KEYS[2], group_uuid, incoming)
	end
	index = index + 4
end

redis.call('ZREM', KEYS[1], '__EMPTY__')
if redis.call('ZCARD', KEYS[1]) == 0 then
	redis.call('ZADD', KEYS[1], 0, '__EMPTY__')
end
redis.call('HSET', KEYS[2], '__READY__', '1')
redis.call('EXPIRE', KEYS[1], ARGV[2])
redis.call('EXPIRE', KEYS[2], ARGV[2])
return 1
`
)
