package message

// 本文件集中放置消息 seq 分配相关 Lua 脚本。
// 这些脚本都只操作单个 Redis key，目的是把“读当前值、必要时修复、递增”放到 Redis 内部原子完成，
// 避免 Go 侧 GET/SET/INCR 多命令交错导致并发请求拿到重复 seq。

// allocExistingSeqScript 只处理 Redis key 已存在的热路径。
// key 不存在时返回 -1，让 Go 侧先查询 DB MAX(seq) 作为恢复基准，避免 Redis 丢 key 后从 1 重新计数。
const allocExistingSeqScript = `
-- 热路径：计数器存在时直接递增，避免每次发送消息都查询 MySQL。
if redis.call("EXISTS", KEYS[1]) == 1 then
  return redis.call("INCR", KEYS[1])
end
-- 冷路径：计数器不存在，交给 Go 侧加载 DB 最大 seq 后再恢复。
return -1
`

// recoverSeqScript 用于 Redis key 缺失后的首次恢复并分配下一个 seq。
// ARGV[1] 是 DB 中当前最大已落库 seq，脚本会先把 Redis 修到该上界，再 INCR 返回新 seq。
const recoverSeqScript = `
local floor = tonumber(ARGV[1])
local current = redis.call("GET", KEYS[1])
if current == false then
  -- key 仍不存在：从 DB 最大已落库 seq 回填，下一步 INCR 会分配 floor + 1。
  redis.call("SET", KEYS[1], floor)
else
  local currentSeq = tonumber(current)
  if currentSeq == nil or currentSeq < floor then
    -- 并发恢复或异常写入时，如果 Redis 值低于 DB 上界，先单调修正。
    redis.call("SET", KEYS[1], floor)
  end
end
return redis.call("INCR", KEYS[1])
`

// repairSeqScript 用于 DB 唯一索引发现 seq 重复后的自愈。
// 它只把 Redis 计数器提升到 DB 最大 seq，不直接分配新 seq；后续 AllocSeq 再 INCR 分配。
const repairSeqScript = `
local floor = tonumber(ARGV[1])
local current = redis.call("GET", KEYS[1])
if current == false then
  -- Redis key 已丢失：恢复到 DB 上界即可，避免 RepairSeq 本身消耗一个 seq。
  redis.call("SET", KEYS[1], floor)
  return floor
end
local currentSeq = tonumber(current)
if currentSeq == nil or currentSeq < floor then
  -- Redis 值回退或损坏：只允许向前修复，绝不把计数器写小。
  redis.call("SET", KEYS[1], floor)
  return floor
end
return currentSeq
`
