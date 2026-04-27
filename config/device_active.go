package config

import (
	"github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"time"
)

// DeviceActiveConfig 设备活跃时间同步配置（Gateway / Connect 通用）。
type DeviceActiveConfig struct {
	// ShardCount 节流分片数量（分片锁 map）。
	ShardCount int `json:"shardCount" yaml:"shardCount"`
	// UpdateInterval 超过该间隔才会再次触发活跃时间同步。
	UpdateInterval time.Duration `json:"updateInterval" yaml:"updateInterval"`
	// FlushInterval 缓冲 map 批量消费周期。
	FlushInterval time.Duration `json:"flushInterval" yaml:"flushInterval"`
	// WorkerCount 异步同步工作协程数。
	WorkerCount int `json:"workerCount" yaml:"workerCount"`
	// QueueSize 异步同步任务队列容量。
	QueueSize int `json:"queueSize" yaml:"queueSize"`
	// RPCTimeout 单次 gRPC 更新超时。
	RPCTimeout time.Duration `json:"rpcTimeout" yaml:"rpcTimeout"`
	// OnlineWindow 设备在线判定窗口。
	OnlineWindow time.Duration `json:"onlineWindow" yaml:"onlineWindow"`
}

// DefaultDeviceActiveConfig 返回默认配置（可通过环境变量覆盖）。
// - DEVICE_ACTIVE_SHARD_COUNT: 分片数量（默认 64）
// - DEVICE_ACTIVE_UPDATE_INTERVAL_SECONDS: 同步间隔秒数（默认 180，即 3 分钟）
// - DEVICE_ACTIVE_FLUSH_INTERVAL_SECONDS: 批量消费周期秒数（默认 60，即 1 分钟）
// - DEVICE_ACTIVE_WORKER_COUNT: worker 数（默认 8）
// - DEVICE_ACTIVE_QUEUE_SIZE: 队列容量（默认 8192）
// - DEVICE_ACTIVE_RPC_TIMEOUT_MS: RPC 超时毫秒（默认 3000）
// - DEVICE_ACTIVE_ONLINE_WINDOW_SECONDS: 在线判定窗口秒数（默认 300，即 5 分钟）
func DefaultDeviceActiveConfig() DeviceActiveConfig {
	cfg := DeviceActiveConfig{
		ShardCount:     getenvInt("DEVICE_ACTIVE_SHARD_COUNT", 64),
		UpdateInterval: time.Duration(getenvInt("DEVICE_ACTIVE_UPDATE_INTERVAL_SECONDS", int(deviceactive.DefaultUpdateInterval/time.Second))) * time.Second,
		FlushInterval:  time.Duration(getenvInt("DEVICE_ACTIVE_FLUSH_INTERVAL_SECONDS", int(deviceactive.DefaultFlushInterval/time.Second))) * time.Second,
		WorkerCount:    getenvInt("DEVICE_ACTIVE_WORKER_COUNT", 8),
		QueueSize:      getenvInt("DEVICE_ACTIVE_QUEUE_SIZE", 8192),
		RPCTimeout:     time.Duration(getenvInt("DEVICE_ACTIVE_RPC_TIMEOUT_MS", 3000)) * time.Millisecond,
		OnlineWindow:   time.Duration(getenvInt("DEVICE_ACTIVE_ONLINE_WINDOW_SECONDS", int(deviceactive.DefaultOnlineWindow/time.Second))) * time.Second,
	}
	return normalizeDeviceActiveConfig(cfg)
}

func normalizeDeviceActiveConfig(cfg DeviceActiveConfig) DeviceActiveConfig {
	if cfg.ShardCount <= 0 {
		cfg.ShardCount = deviceactive.DefaultShardCount
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = deviceactive.DefaultWorkerCount
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = deviceactive.DefaultQueueSize
	}
	if cfg.RPCTimeout <= 0 {
		cfg.RPCTimeout = 3 * time.Second
	}
	if cfg.OnlineWindow <= 0 {
		cfg.OnlineWindow = deviceactive.DefaultOnlineWindow
	}
	if cfg.UpdateInterval <= 0 {
		cfg.UpdateInterval = deviceactive.DefaultUpdateInterval
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = deviceactive.DefaultFlushInterval
	}

	// 保证 update 间隔小于在线窗口，避免“持续活跃误判离线”。
	if cfg.UpdateInterval >= cfg.OnlineWindow {
		candidate := cfg.OnlineWindow - time.Second
		if candidate < time.Second {
			candidate = time.Second
		}
		cfg.UpdateInterval = candidate
	}

	// 保证 flush 周期不大于 update 间隔，避免缓冲滞留过久。
	if cfg.FlushInterval > cfg.UpdateInterval {
		candidate := cfg.UpdateInterval / 2
		if candidate < time.Second {
			candidate = time.Second
		}
		cfg.FlushInterval = candidate
	}

	return cfg
}
