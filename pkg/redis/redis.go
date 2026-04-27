package redis

import (
	"context"
	"errors"
	"strings"

	"github.com/013677890/LCchat-Backend/config"

	goredis "github.com/redis/go-redis/v9"
)

var global *goredis.Client

// Client 返回全局 Redis 客户端（未初始化时为 nil）。
func Client() *goredis.Client { return global }

// ReplaceGlobal 设置全局 Redis 客户端。
func ReplaceGlobal(c *goredis.Client) { global = c }

// Build 基于配置创建 Redis 客户端并做一次 Ping 验证。
func Build(cfg config.RedisConfig) (*goredis.Client, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, errors.New("redis addr is empty")
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:            cfg.Addr,         // host:port
		Password:        cfg.Password,     // 可空
		DB:              cfg.DB,           // DB 索引，默认 0
		PoolSize:        cfg.PoolSize,     // 连接池大小
		MinIdleConns:    cfg.MinIdleConns, // 最小空闲连接
		DialTimeout:     cfg.DialTimeout,  // 建连超时
		ReadTimeout:     cfg.ReadTimeout,  // 读超时
		WriteTimeout:    cfg.WriteTimeout, // 写超时
		PoolTimeout:     cfg.PoolTimeout,  // 从池获取连接超时
		ConnMaxIdleTime: cfg.ConnMaxIdle,  // 连接最大空闲时间（对应 go-redis ConnMaxIdleTime）
		MaxRetries: cfg.MaxRetries, // 最大重试次数
		MinRetryBackoff: cfg.MinRetryBackoff, // 最小重试间隔
		MaxRetryBackoff: cfg.MaxRetryBackoff, // 最大重试间隔
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return client, nil
}
