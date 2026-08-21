package store

import (
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Store 是 group 的 MySQL 权威仓储。
//
// 职责边界：
//   - 命令写：所有群资料/成员/申请的状态变更，必须在同一 MySQL 事务内完成
//     业务表写入、cache_version 递增和 Outbox 事件落库；
//   - 回源读：当 cache 层 Redis miss 或缓存结构非法时，作为权威数据源返回
//     MySQL 最新快照，但不负责写回 Redis（写回由 projection 层异步完成）；
//   - Bloom Filter：redisClient 仅用于群 UUID 存在性 Bloom Filter 的读写，
//     不是展示缓存的写入口，禁止在 Store 内直接写 Redis 业务缓存。
//
// 写事务的并发控制全在事务内通过 SELECT ... FOR UPDATE 完成，不在应用层
// 使用分布式锁，保证锁粒度精确到单行且与业务数据同生命周期。
type Store struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// New 创建 MySQL 权威仓储。
//
// 构造阶段只保存依赖，不启动后台任务。Bloom Filter 由 cache 包在读侧初始化；
// 写侧 CreateGroup 会再次 Add，保证新群不会被假阴性拦截。
func New(db *gorm.DB, redisClient *goredis.Client) *Store {
	return &Store{db: db, redisClient: redisClient}
}

// DB 返回底层数据库句柄，供同包测试构造事务夹具。
func (s *Store) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// isActiveGroupMember 判断成员关系是否仍为有效成员。
//
// 有效成员需同时满足：记录非空、状态为正常、未被软删除。
// 该函数在写事务的锁内使用，判断结果直接决定后续操作分支。
func isActiveGroupMember(member *model.GroupMember) bool {
	return member != nil && member.Status == repository.MemberStatusNormal && !member.DeletedAt.Valid
}
