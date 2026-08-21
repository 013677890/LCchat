// Package cachex 包含仓储层共用的轻量级缓存策略辅助工具。
package cachex

import (
	"math/rand"
	"strings"
	"time"
)

// JitterTTL 在基础过期时间上增加 ±10% 的均匀分布随机抖动，
// 用于打散同一批次写入的缓存过期时间，防止发生缓存雪崩。
func JitterTTL(base time.Duration) time.Duration {
	jitterRange := float64(base) * 0.1
	jitter := time.Duration(rand.Float64()*jitterRange*2 - jitterRange)
	return base + jitter
}

// Chance 根据给定的概率（0.0 ~ 1.0）返回本次随机采样是否命中，
// 常用于热点缓存读取时的低频采样续期，降低 Redis 命令压力。
func Chance(probability float64) bool {
	return rand.Float64() < probability
}

// IsRedisWrongType 判断错误是否为 Redis 数据类型不匹配（WRONGTYPE）错误，
// 避免调用方与具体的 Redis 客户端错误类型产生强耦合，方便识别脏缓存并自愈。
func IsRedisWrongType(err error) bool {
	return err != nil && strings.Contains(err.Error(), "WRONGTYPE")
}
