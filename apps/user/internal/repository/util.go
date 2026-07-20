package repository

import (
	"math/rand"
	"time"
)

// getRandomExpireTime 生成基础过期时间上下 10% 范围内的随机 TTL。
//
// 缓存写入集中在相近时刻时，固定 TTL 容易让大量 key 同时失效；这里保留本地抖动逻辑，
// 只改变过期时刻的分布，不改变调用方配置的数量级和缓存读写语义。
func getRandomExpireTime(baseExpire time.Duration) time.Duration {
	// 计算随机抖动范围（±10%）
	jitterRange := float64(baseExpire) * 0.1
	jitter := time.Duration(rand.Float64()*float64(jitterRange)*2 - float64(jitterRange))

	return baseExpire + jitter
}
