package repository

const (
	// luaCheckAndIncrementVerifyCodeRateLimit 原子完成验证码发送频控检查与计数占位。
	//
	// KEYS:
	//   1. 单邮箱分钟级计数
	//   2. 单邮箱 24 小时计数
	//   3. 单 IP 小时级计数
	// ARGV:
	//   1. 单邮箱分钟级上限
	//   2. 单邮箱 24 小时上限
	//   3. 单 IP 小时级上限
	//   4. 单邮箱分钟级 TTL 秒数
	//   5. 单邮箱 24 小时 TTL 秒数
	//   6. 单 IP 小时级 TTL 秒数
	//
	// 返回 0 表示已通过并完成计数占位；返回 1/2/3 分别表示命中对应限流。
	luaCheckAndIncrementVerifyCodeRateLimit = `
local minuteKey = KEYS[1]
local dayKey = KEYS[2]
local ipKey = KEYS[3]

local minuteLimit = tonumber(ARGV[1])
local dayLimit = tonumber(ARGV[2])
local ipLimit = tonumber(ARGV[3])
local minuteTTL = tonumber(ARGV[4])
local dayTTL = tonumber(ARGV[5])
local ipTTL = tonumber(ARGV[6])

local minuteCount = tonumber(redis.call('GET', minuteKey) or '0')
if minuteCount >= minuteLimit then
	return 1
end

local dayCount = tonumber(redis.call('GET', dayKey) or '0')
if dayCount >= dayLimit then
	return 2
end

local ipCount = tonumber(redis.call('GET', ipKey) or '0')
if ipCount >= ipLimit then
	return 3
end

minuteCount = redis.call('INCR', minuteKey)
if minuteCount == 1 then
	redis.call('EXPIRE', minuteKey, minuteTTL)
end

dayCount = redis.call('INCR', dayKey)
if dayCount == 1 then
	redis.call('EXPIRE', dayKey, dayTTL)
end

ipCount = redis.call('INCR', ipKey)
if ipCount == 1 then
	redis.call('EXPIRE', ipKey, ipTTL)
end

return 0
`
)
