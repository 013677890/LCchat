package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/outbox"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ==================== 仓储实现 ====================

// repositoryImpl 消息领域仓储实现
// 依赖：MySQL (GORM) + Redis
type repositoryImpl struct {
	db    *gorm.DB
	redis *redis.Client
}

const (
	// 这些名字必须与 model/Message.go、config/mysql/001_schema.sql 保持一致，
	// 仓储层依赖它们把 DB 唯一冲突拆成“客户端幂等”和“seq 回退”等不同业务含义。
	duplicateKeySenderClient = "uidx_sender_client"
	duplicateKeyConvSeq      = "uidx_conv_seq"
	duplicateKeyMsgID        = "uk_message_msg_id"
)

// NewRepository 创建消息仓储实例
func NewRepository(db *gorm.DB, redisClient *redis.Client) Repository {
	return &repositoryImpl{db: db, redis: redisClient}
}

// ==================== seq 分配 ====================

// AllocSeq 通过 Redis 原子分配会话内递增序号，并在计数器缺失时从 DB 回填。
//
// Redis Key:  msg:seq:{conv_id}
// 数据类型:   String (integer)
// TTL:        无（永久存在，随会话生命周期）
//
// 每次调用返回值严格 +1，保证同一会话内 seq 全局唯一且递增。
// 客户端收到消息后按 seq 排序展示，并利用 seq gap 检测消息丢失。
func (r *repositoryImpl) AllocSeq(ctx context.Context, convId string) (int64, error) {
	if r.redis == nil {
		return 0, fmt.Errorf("AllocSeq: redis client is nil")
	}
	key := rediskey.MsgSeqKey(convId)
	// 热路径不查 DB：绝大多数情况下 Redis 计数器存在，直接在 Lua 内完成 INCR。
	// 如果返回 -1，说明 key 缺失，必须先用 DB 最大已落库 seq 作为恢复基准。
	seq, err := r.redis.Eval(ctx, allocExistingSeqScript, []string{key}).Int64()
	if err != nil {
		return 0, fmt.Errorf("AllocSeq: redis allocate %s failed: %w", key, err)
	}
	if seq >= 0 {
		return seq, nil
	}

	// Redis 不是最终事实源；key 丢失时只能以 message 表中的 MAX(seq) 作为权威上界。
	// 后续 recoverSeqScript 会在 Redis 内原子地“回填上界 + 分配下一号”。
	maxSeq, err := r.getDBMaxSeq(ctx, convId)
	if err != nil {
		return 0, fmt.Errorf("AllocSeq: load db max seq failed: %w", err)
	}
	seq, err = r.redis.Eval(ctx, recoverSeqScript, []string{key}, maxSeq).Int64()
	if err != nil {
		return 0, fmt.Errorf("AllocSeq: redis recover %s failed: %w", key, err)
	}
	return seq, nil
}

// RepairSeq 将 Redis seq 计数器修复到不小于 DB 最大已落库 seq。
func (r *repositoryImpl) RepairSeq(ctx context.Context, convId string) error {
	if r.redis == nil {
		return fmt.Errorf("RepairSeq: redis client is nil")
	}
	maxSeq, err := r.getDBMaxSeq(ctx, convId)
	if err != nil {
		return fmt.Errorf("RepairSeq: load db max seq failed: %w", err)
	}
	key := rediskey.MsgSeqKey(convId)
	// RepairSeq 只修复 Redis 上界，不直接 INCR，避免一次修复操作额外消耗 seq 造成更大的空洞。
	if err := r.redis.Eval(ctx, repairSeqScript, []string{key}, maxSeq).Err(); err != nil {
		return fmt.Errorf("RepairSeq: redis repair %s failed: %w", key, err)
	}
	return nil
}

// getDBMaxSeq 查询消息事实表中的最大已落库 seq，作为 Redis 计数器恢复的权威上界。
func (r *repositoryImpl) getDBMaxSeq(ctx context.Context, convId string) (int64, error) {
	var maxSeq int64
	if err := r.db.WithContext(ctx).
		Model(&model.Message{}).
		Where("conv_id = ?", convId).
		Select("COALESCE(MAX(seq), 0)").
		Scan(&maxSeq).Error; err != nil {
		return 0, err
	}
	return maxSeq, nil
}

// ==================== 幂等 (SETNX 锁 + 结果缓存) ====================

// idempotentCacheEntry 幂等缓存条目
// 当消息首次创建成功后，将关键结果字段序列化为 JSON 存入 Redis，
// 后续相同请求直接反序列化返回，无需查 DB。
type idempotentCacheEntry struct {
	MsgId    string `json:"msg_id"`
	Seq      int64  `json:"seq"`
	ConvId   string `json:"conv_id"`
	SendTime int64  `json:"send_time"` // unix 毫秒
}

// TryAcquireIdempotent 尝试获取幂等锁
//
// 实现策略 (SETNX + 三态返回)：
//
//	┌─────────────────────────────────────────────────┐
//	│ SETNX msg:idempotent:{from}:{device}:{client}   │
//	│       value="PROCESSING"  TTL=10s                │
//	│                                                  │
//	│ ├─ SETNX 成功 (key 不存在)                        │
//	│ │   → return (nil, nil) 获取锁成功                 │
//	│ │                                                │
//	│ ├─ SETNX 失败 (key 已存在)                        │
//	│ │   ├─ GET 值 = "PROCESSING"                     │
//	│ │   │   → return (nil, ErrIdempotentProcessing)  │
//	│ │   │     另一个请求正在处理，客户端应稍后重试         │
//	│ │   │                                            │
//	│ │   └─ GET 值 = JSON 结果                         │
//	│ │       → return (cachedMsg, nil)                 │
//	│ │         幂等命中，直接返回首次创建结果              │
//	│ │                                                │
//	│ └─ Redis 异常                                    │
//	│     → return (nil, redisErr) 降级到 DB 兜底        │
//	└─────────────────────────────────────────────────┘
func (r *repositoryImpl) TryAcquireIdempotent(ctx context.Context, fromUuid, deviceId, clientMsgId string) (*model.Message, error) {
	key := rediskey.MsgIdempotentKey(fromUuid, deviceId, clientMsgId)

	// SETNX "PROCESSING" with 10s TTL
	// 10 秒 TTL 是防止处理中途崩溃导致锁永远不释放的保险措施
	ok, err := r.redis.SetNX(ctx, key, idempotentProcessing, time.Duration(idempotentLockTTLSec)*time.Second).Result()
	if err != nil {
		return nil, fmt.Errorf("TryAcquireIdempotent: redis SETNX failed: %w", err)
	}

	if ok {
		// SETNX 成功 → 锁获取成功，调用者可以继续执行消息创建
		return nil, nil
	}

	// SETNX 失败 → key 已存在，读取当前值判断状态
	val, err := r.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("TryAcquireIdempotent: redis GET failed: %w", err)
	}

	if val == idempotentProcessing {
		// 值为 "PROCESSING"：另一个请求正在处理中
		// 客户端收到此错误后应短暂等待后重试
		return nil, ErrIdempotentProcessing
	}

	// 值为 JSON：已有处理结果（首次创建成功后回写的缓存）
	var entry idempotentCacheEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		// 缓存数据损坏（理论上不应发生），删除后让请求走正常流程
		r.redis.Del(ctx, key)
		return nil, fmt.Errorf("TryAcquireIdempotent: unmarshal cached result failed: %w", err)
	}

	// 反序列化为 model.Message（仅填充关键字段，足够构建 SendMessageResponse）
	msg := &model.Message{
		MsgId:    entry.MsgId,
		Seq:      entry.Seq,
		ConvId:   entry.ConvId,
		SendTime: time.UnixMilli(entry.SendTime),
	}
	return msg, nil
}

// SetIdempotentResult 将 "PROCESSING" 标记替换为实际结果
//
// 在消息落库成功后调用，将 Redis 中的值从 "PROCESSING" 覆盖为 JSON 格式的结果，
// 并延长 TTL 到 10 分钟。后续相同请求直接命中此缓存，不再走 DB。
//
// 即使此方法失败（Redis 异常），也不影响主流程：
// - "PROCESSING" 标记会在 10 秒后自动过期
// - 后续请求会走 DB 唯一索引兜底
func (r *repositoryImpl) SetIdempotentResult(ctx context.Context, fromUuid, deviceId, clientMsgId string, msg *model.Message) error {
	key := rediskey.MsgIdempotentKey(fromUuid, deviceId, clientMsgId)

	entry := idempotentCacheEntry{
		MsgId:    msg.MsgId,
		Seq:      msg.Seq,
		ConvId:   msg.ConvId,
		SendTime: msg.SendTime.UnixMilli(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("SetIdempotentResult: marshal failed: %w", err)
	}

	// SET (覆盖) + 新 TTL = 10 分钟
	if err := r.redis.Set(ctx, key, string(data), rediskey.MsgIdempotentTTL).Err(); err != nil {
		return fmt.Errorf("SetIdempotentResult: redis SET failed: %w", err)
	}
	return nil
}

// ==================== 消息 CRUD ====================

// Create 插入一条消息到 DB
//
// 如果触发 uidx_sender_client (from_uuid, device_id, client_msg_id) 唯一索引冲突，
// 返回 ErrDuplicateMessage，调用者应查出已有记录返回（幂等兜底逻辑）。
func (r *repositoryImpl) Create(ctx context.Context, msg *model.Message) error {
	if err := r.db.WithContext(ctx).Create(msg).Error; err != nil {
		if duplicateErr := mapDuplicateMessageError(err); duplicateErr != nil {
			return duplicateErr
		}
		return fmt.Errorf("Create: db insert failed: %w", err)
	}
	return nil
}

// CreateWithOutbox 在同一事务中写入消息事实和 CDC Outbox 事件。
func (r *repositoryImpl) CreateWithOutbox(ctx context.Context, msg *model.Message, event OutboxEvent) error {
	if event.EventType == "" || event.EntityID == "" || event.Payload == "" {
		return fmt.Errorf("CreateWithOutbox: invalid outbox event")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			if duplicateErr := mapDuplicateMessageError(err); duplicateErr != nil {
				return duplicateErr
			}
			return fmt.Errorf("CreateWithOutbox: db insert failed: %w", err)
		}
		if err := outbox.InsertEvent(tx, event.EventType, event.EntityID, event.Payload); err != nil {
			return fmt.Errorf("CreateWithOutbox: outbox insert failed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// GetByDuplicateKey 通过幂等三元组从 DB 查询已存在的消息
//
// 使用场景：Redis SETNX 降级 + DB 唯一索引冲突后，需要查出首次创建的消息返回给客户端
// 查询走 uidx_sender_client 唯一索引，性能有保障
func (r *repositoryImpl) GetByDuplicateKey(ctx context.Context, fromUuid, deviceId, clientMsgId string) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).
		Where("from_uuid = ? AND device_id = ? AND client_msg_id = ?", fromUuid, deviceId, clientMsgId).
		First(&msg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrMessageNotFound
		}
		return nil, fmt.Errorf("GetByDuplicateKey: db query failed: %w", err)
	}
	return &msg, nil
}

// GetBySeqRange 按 seq 范围拉取消息
//
// 参数说明：
//   - convId:    会话 ID
//   - anchorSeq: 锚点 seq（不包含自身）
//   - direction: 1=FORWARD(seq > anchor, 拉新), 2=BACKWARD(seq < anchor, 拉旧)
//   - limit:     最大返回条数
//   - clearSeq:  会话清空位点，只返回 seq > clearSeq 的消息（0 表示不过滤）
//
// 排除规则：
//   - status=2 (已删除) 的消息不返回
//   - seq <= clearSeq 的消息不返回（用户删除会话后的过滤）
func (r *repositoryImpl) GetBySeqRange(ctx context.Context, convId string, anchorSeq int64, direction int, limit int, clearSeq int64) ([]*model.Message, error) {
	var msgs []*model.Message
	query := r.db.WithContext(ctx).Where("conv_id = ? AND status != 2", convId)

	// 过滤 clear_seq
	if clearSeq > 0 {
		query = query.Where("seq > ?", clearSeq)
	}

	// 根据方向设置 WHERE 和 ORDER BY
	switch direction {
	case DirectionForward:
		// 拉新消息：seq > anchor，按 seq 升序
		query = query.Where("seq > ?", anchorSeq).Order("seq ASC")
	case DirectionBackward:
		// 拉历史：seq < anchor，按 seq 降序（最新的在前，客户端可反转）。
		// anchorSeq=0 表示从最新消息开始拉取。
		if anchorSeq > 0 {
			query = query.Where("seq < ?", anchorSeq)
		}
		query = query.Order("seq DESC")
	default:
		// 默认拉最新
		query = query.Order("seq DESC")
	}

	if err := query.Limit(limit).Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("GetBySeqRange: db query failed: %w", err)
	}
	return msgs, nil
}

// GetByIds 批量按消息 ID 查询
//
// 使用走 msg_id 的 uniqueIndex，WHERE IN 查询
// 找不到的 msg_id 静默跳过（不报错，与 proto 注释约定一致）
func (r *repositoryImpl) GetByIds(ctx context.Context, convId string, msgIds []string) ([]*model.Message, error) {
	var msgs []*model.Message
	err := r.db.WithContext(ctx).
		Where("conv_id = ? AND msg_id IN ? AND status != 2", convId, msgIds).
		Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("GetByIds: db query failed: %w", err)
	}
	return msgs, nil
}

// GetById 查单条消息
func (r *repositoryImpl) GetById(ctx context.Context, convId string, msgId string) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).
		Where("conv_id = ? AND msg_id = ?", convId, msgId).
		First(&msg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrMessageNotFound
		}
		return nil, fmt.Errorf("GetById: db query failed: %w", err)
	}
	return &msg, nil
}

// GetMaxSeq 查询会话当前 seq 上界。
// 优先读 Redis（AllocSeq 的 INCR 计数器），语义上是"已分配的最大 seq"。
// Redis 异常时降级查 DB MAX(seq)。
func (r *repositoryImpl) GetMaxSeq(ctx context.Context, convId string) (int64, error) {
	key := rediskey.MsgSeqKey(convId)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, key).Int64()
		if err == nil {
			return val, nil
		}
	}

	maxSeq, dbErr := r.getDBMaxSeq(ctx, convId)
	if dbErr != nil {
		return 0, fmt.Errorf("GetMaxSeq: db fallback failed: %w", dbErr)
	}
	return maxSeq, nil
}

// ==================== 撤回 ====================

// UpdateStatus 更新消息状态和内容
//
// 撤回场景：status=1, content 改写为系统提示 JSON
// 使用 map 而非 struct 更新，避免 GORM 跳过零值字段
// RowsAffected=0 说明消息不存在（conv_id + msg_id 不匹配）
func (r *repositoryImpl) UpdateStatus(ctx context.Context, convId string, msgId string, status int8, content string) error {
	return updateMessageStatus(r.db.WithContext(ctx), convId, msgId, status, content)
}

// UpdateStatusWithOutbox 在同一事务中更新消息状态并写入 CDC Outbox 事件。
func (r *repositoryImpl) UpdateStatusWithOutbox(ctx context.Context, convId string, msgId string, status int8, content string, event OutboxEvent) error {
	if event.EventType == "" || event.EntityID == "" || event.Payload == "" {
		return fmt.Errorf("UpdateStatusWithOutbox: invalid outbox event")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := updateMessageStatus(tx, convId, msgId, status, content); err != nil {
			return err
		}
		if err := outbox.InsertEvent(tx, event.EventType, event.EntityID, event.Payload); err != nil {
			return fmt.Errorf("UpdateStatusWithOutbox: outbox insert failed: %w", err)
		}
		return nil
	})
}

func updateMessageStatus(db *gorm.DB, convId string, msgId string, status int8, content string) error {
	result := db.
		Model(&model.Message{}).
		Where("conv_id = ? AND msg_id = ?", convId, msgId).
		Updates(map[string]interface{}{
			"status":  status,
			"content": content,
		})
	if result.Error != nil {
		return fmt.Errorf("UpdateStatus: db update failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// ==================== 辅助方法 ====================

// mapDuplicateMessageError 将唯一索引冲突映射为更细粒度的领域错误。
func mapDuplicateMessageError(err error) error {
	if !isDuplicateKeyError(err) {
		return nil
	}
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, duplicateKeyConvSeq) || hasAll(errMsg, "message.conv_id", "message.seq"):
		return ErrDuplicateMessageSeq
	case strings.Contains(errMsg, duplicateKeyMsgID) || strings.Contains(errMsg, "message.msg_id"):
		return ErrDuplicateMessageID
	case strings.Contains(errMsg, duplicateKeySenderClient) || hasAll(errMsg, "message.client_msg_id", "message.from_uuid", "message.device_id"):
		return ErrDuplicateMessage
	default:
		return ErrDuplicateMessage
	}
}

// isDuplicateKeyError 判断 MySQL/SQLite 错误是否为唯一索引冲突。
//
// MySQL Error 1062: Duplicate entry 'xxx' for key 'uidx_sender_client'
// SQLite 测试会返回 UNIQUE constraint failed，需要兼容单元测试环境。
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "1062") ||
		strings.Contains(errMsg, "Duplicate entry") ||
		strings.Contains(errMsg, "UNIQUE constraint failed")
}

func hasAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
