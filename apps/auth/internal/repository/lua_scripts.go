package repository

const (
	// luaIncrementWithExpire 递增计数器，仅在首次创建时设置过期时间。
	luaIncrementWithExpire = `
local key = KEYS[1]
local expire = tonumber(ARGV[1])
local current = redis.call('INCR', key)

if current == 1 then
	redis.call('EXPIRE', key, expire)
end

return current
`
)
