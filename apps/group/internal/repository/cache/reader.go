package cache

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/projection"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/store"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// RepairScheduler 是 cache 读 miss 后提交权威修复意图的最小依赖接口。
//
// 架构设计约束：
//  1. 读线程在发现 Redis miss 或数据结构损坏时，禁止直接把当前请求读到的对象晚写回 Redis，
//     防止经典的 cache-aside “读旧快照覆盖并发写新事实” 竞态；
//  2. 读层仅把 group_uuid / user_uuid 标识提交给 projection 层的修复调度器；
//  3. 由 projection 后台异步重新从 MySQL 加载带锁一致性快照，再通过带版本号（cache_version）的 Lua 脚本安全重建缓存。
type RepairScheduler interface {
	// ScheduleGroupCacheReconcile 调度群维度缓存重建（资料、成员列表、待审批申请）。
	ScheduleGroupCacheReconcile(ctx context.Context, groupUUID string)
	// ScheduleUserGroupsCacheReconcile 调度指定用户群反向索引的完整重建并立旗 __READY__=1。
	ScheduleUserGroupsCacheReconcile(ctx context.Context, userUUID string)
	// ScheduleUserGroupsCacheAuditAfterHit 在用户群反向索引读命中后，通过分布式租约异步触发低频权威对账。
	ScheduleUserGroupsCacheAuditAfterHit(ctx context.Context, userUUID string)
}

// Reader 是 group-service 的同步读缓存核心实现。
//
// 一致性模型与职责边界：
//   - 一致性模型：最终一致性（Eventual Consistency）。群资料展示、成员身份判定、发送权限等
//     允许短暂落后于 MySQL 权威数据，通过 Kafka 事件投影（Projection）与后台异步对账（Reconciliation）收敛；
//   - store (*store.Store)：MySQL 权威回源，用于在 Redis miss、布隆命中或复杂管理查询时回源数据库；
//   - repair (RepairScheduler)：投影侧修复调度入口，避免在业务读请求线程中做无版本并发脏写；
//   - redisClient (*goredis.Client)：负责读取 String / Hash / ZSet 等只读数据结构；
//   - flightGroup (singleflight.Group)：并发控制组件，合并同一时刻针对同一群或用户的多并发回源请求，防止缓存击穿压垮 MySQL。
type Reader struct {
	redisClient *goredis.Client
	store       *store.Store
	repair      RepairScheduler
	flightGroup singleflight.Group
}

// New 创建同步读缓存实例。
//
// 构造阶段会自动尝试初始化 group_uuid 的 RedisBloom 布隆过滤器，
// 用于在读请求到达时前置短路“确定不存在的群”，防止恶意扫描导致缓存穿透。
func New(
	redisClient *goredis.Client,
	mysqlStore *store.Store,
	repair RepairScheduler,
) *Reader {
	repository.InitGroupUUIDBloom(context.Background(), redisClient)
	return &Reader{
		redisClient: redisClient,
		store:       mysqlStore,
		repair:      repair,
	}
}

// NewWithProjector 用投影仓储同时充当修复调度器，供 Wire 依赖注入装配使用。
func NewWithProjector(
	redisClient *goredis.Client,
	mysqlStore *store.Store,
	projector *projection.Repository,
) *Reader {
	return New(redisClient, mysqlStore, projector)
}

// scheduleGroupRepair 提交群维度缓存修复意图。
// 仅提交聚合标识 groupUUID，由后台异步协程重新读取 MySQL 一致性快照后通过版本化 Lua 写入。
func (r *Reader) scheduleGroupRepair(ctx context.Context, groupUUID string) {
	if r == nil || r.repair == nil || groupUUID == "" {
		return
	}
	r.repair.ScheduleGroupCacheReconcile(ctx, groupUUID)
}

// scheduleUserRepair 提交用户群反向索引修复意图。
// 用于在用户群列表未立旗（__READY__ != 1）或缓存失效时异步触发全量对账。
func (r *Reader) scheduleUserRepair(ctx context.Context, userUUID string) {
	if r == nil || r.repair == nil || userUUID == "" {
		return
	}
	r.repair.ScheduleUserGroupsCacheReconcile(ctx, userUUID)
}

// scheduleUserAudit 在用户群列表读命中（READY）后，尝试抢占分布式租约以触发低频后台对账。
// 这一机制用于自愈消除“用户被移除但缓存残留”等单群扫描难以察觉的幽灵反向索引数据。
func (r *Reader) scheduleUserAudit(ctx context.Context, userUUID string) {
	if r == nil || r.repair == nil || userUUID == "" {
		return
	}
	r.repair.ScheduleUserGroupsCacheAuditAfterHit(ctx, userUUID)
}
