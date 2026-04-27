package id

import (
	"github.com/oklog/ulid/v2"
)

// GenerateULID 生成时间有序的唯一 ID
//
// ULID (Universally Unique Lexicographically Sortable Identifier):
//   - 前 48 bit: 毫秒级时间戳 → 保证字典序按时间递增
//   - 后 80 bit: 加密随机数 → 保证分布式唯一性
//   - 编码为 26 字符 Crockford Base32 字符串
//
// 相比 UUID v4 的优势：
//   - MySQL B+ 树顺序写入，无页分裂（UUID v4 随机分布导致大量页分裂）
//   - 字典序 = 时间序，排序极其友好
//   - 不需要雪花算法的 worker_id / data_center_id 配置
//
// 并发安全：
//
//	ulid.Make() 内部使用基于 sync.Pool 的高性能、并发安全的全局熵池，
//	无需手动创建随机源，极高并发下仍然安全且高性能。
func GenerateULID() string {
	return ulid.Make().String()
}
