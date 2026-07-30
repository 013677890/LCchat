package redisbloom

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	goredis "github.com/redis/go-redis/v9"
)

// Filter 表示一个 RedisBloom 布隆过滤器配置。
//
// 这里的 Bloom 用作“存在性索引”：false 代表确定不存在，true 代表可能存在。
// 因此业务写入新实体时必须先 Add 再写 DB，避免出现 DB 已存在但 Bloom 判不存在的 false negative。
type Filter struct {
	Key       string
	ErrorRate float64
	Capacity  int64
}

var ensuredFilters sync.Map

// Ensure 在当前进程内按 Redis client + key 维度初始化一次 Bloom Filter。
//
// RedisBloom 的 BF.RESERVE 只能在 key 不存在时执行；如果 key 已存在，说明过滤器已经初始化，
// 这里把“已存在”视为成功。其它错误会返回给调用方，由业务决定降级读 DB 或拒绝写 DB。
func (f Filter) Ensure(ctx context.Context, client *goredis.Client) error {
	if client == nil || f.Key == "" {
		return nil
	}
	if f.ErrorRate <= 0 || f.ErrorRate >= 1 {
		return fmt.Errorf("invalid bloom error rate: %v", f.ErrorRate)
	}
	if f.Capacity <= 0 {
		return fmt.Errorf("invalid bloom capacity: %d", f.Capacity)
	}

	ensureKey := fmt.Sprintf("%p:%s", client, f.Key)
	if _, ok := ensuredFilters.Load(ensureKey); ok {
		return nil
	}

	err := client.Do(ctx,
		"BF.RESERVE",
		f.Key,
		strconv.FormatFloat(f.ErrorRate, 'f', -1, 64),
		strconv.FormatInt(f.Capacity, 10),
	).Err()

	if err != nil && !isBloomAlreadyExists(err) {
		return err
	}
	ensuredFilters.Store(ensureKey, struct{}{})
	return nil
}

// Exists 用 BF.EXISTS 判断单个元素是否可能存在。
//
// usable=false 表示 RedisBloom 没有给出可靠答案，读路径应回源 DB；写路径不应依赖这个结果。
func (f Filter) Exists(ctx context.Context, client *goredis.Client, item string) (exists bool, usable bool, err error) {
	if client == nil || f.Key == "" || item == "" {
		return false, false, nil
	}
	if err := f.Ensure(ctx, client); err != nil {
		return false, false, err
	}
	value, err := client.Do(ctx, "BF.EXISTS", f.Key, item).Int()
	if err != nil {
		return false, false, err
	}
	return value == 1, true, nil
}

// MExists 用 BF.MEXISTS 批量判断元素是否可能存在。
//
// 返回值顺序与入参 items 保持一致；如果 RedisBloom 返回数量异常，视为不可用，避免误过滤。
func (f Filter) MExists(ctx context.Context, client *goredis.Client, items []string) (exists []bool, usable bool, err error) {
	if client == nil || f.Key == "" || len(items) == 0 {
		return nil, false, nil
	}
	if err := f.Ensure(ctx, client); err != nil {
		return nil, false, err
	}
	args := make([]interface{}, 0, len(items)+2)
	args = append(args, "BF.MEXISTS", f.Key)
	for _, item := range items {
		args = append(args, item)
	}
	values, err := client.Do(ctx, args...).Slice()
	if err != nil {
		return nil, false, err
	}
	if len(values) != len(items) {
		return nil, false, fmt.Errorf("redis bloom mexists reply length mismatch: got %d want %d", len(values), len(items))
	}
	result := make([]bool, len(values))
	for i, value := range values {
		exists, err := parseBoolReply(value)
		if err != nil {
			return nil, false, err
		}
		result[i] = exists
	}

	return result, true, nil
}

// Add 用 BF.ADD 写入单个元素。
//
// 新增业务实体时应在 DB 事务前调用它：即使后续 DB 写失败，也只是留下 false positive，
// 不会破坏 Bloom “false 一定不存在”的关键约束。
func (f Filter) Add(ctx context.Context, client *goredis.Client, item string) error {
	if client == nil || f.Key == "" || item == "" {
		return nil
	}
	if err := f.Ensure(ctx, client); err != nil {
		return err
	}
	return client.Do(ctx, "BF.ADD", f.Key, item).Err()
}

// MAdd 用 BF.MADD 批量写入元素。
//
// 主要用于读回源命中后的异步自愈补写；失败由调用方记录日志即可，不影响已读到的数据。
func (f Filter) MAdd(ctx context.Context, client *goredis.Client, items []string) error {
	if client == nil || f.Key == "" || len(items) == 0 {
		return nil
	}
	if err := f.Ensure(ctx, client); err != nil {
		return err
	}
	args := make([]interface{}, 0, len(items)+2)
	args = append(args, "BF.MADD", f.Key)
	for _, item := range items {
		if item == "" {
			continue
		}
		args = append(args, item)
	}
	if len(args) == 2 {
		return nil
	}
	return client.Do(ctx, args...).Err()
}

func isBloomAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "item exists") ||
		strings.Contains(message, "already exists") ||
		strings.Contains(message, "busykey")
}

func parseBoolReply(value interface{}) (bool, error) {
	switch v := value.(type) {
	case int:
		return v == 1, nil
	case int64:
		return v == 1, nil
	case uint64:
		return v == 1, nil
	case bool:
		return v, nil
	case string:
		return v == "1", nil
	case []byte:
		return string(v) == "1", nil
	default:
		return false, fmt.Errorf("unsupported redis bloom bool reply type %T", value)
	}
}
