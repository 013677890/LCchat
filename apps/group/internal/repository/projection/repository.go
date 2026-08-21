package projection

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// Repository 是 group.cache 异步投影与权威对账的实现。
//
// 职责边界：
//  1. 消费已校验的 group.cache 事件，按 projection_version 写入 Redis；
//  2. 从 MySQL 读取一致性快照做权威修复；
//  3. 接收 cache 读 miss 提交的修复意图（Schedule*），不把请求线程里的旧对象晚写 Redis。
//
// 本类型不实现业务写命令，也不对 service 暴露；consumer 与 cache 才是调用方。
type Repository struct {
	db          *gorm.DB
	redisClient *goredis.Client
	flightGroup singleflight.Group
}

// New 创建投影仓储。
//
// Bloom Filter 在构造时 best-effort 初始化：失败只影响存在性短路，不阻断投影写入。
func New(db *gorm.DB, redisClient *goredis.Client) *Repository {
	projector := &Repository{db: db, redisClient: redisClient}
	repository.InitGroupUUIDBloom(context.Background(), redisClient)
	return projector
}

var _ repository.IGroupCacheProjectorRepository = (*Repository)(nil)
