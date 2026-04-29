package config

import "time"

// AsyncConfig 协程池配置。
// 说明：只用于异步任务执行，不负责定时/调度。
type AsyncConfig struct {
	PoolSize         int           `json:"poolSize" yaml:"poolSize"`                 // 协程池容量
	MaxBlockingTasks int           `json:"maxBlockingTasks" yaml:"maxBlockingTasks"` // 兼容字段；协程池强制非阻塞提交，该字段不再生效
	ExpiryDuration   time.Duration `json:"expiryDuration" yaml:"expiryDuration"`     // 空闲 worker 过期时间
	Nonblocking      bool          `json:"nonblocking" yaml:"nonblocking"`           // 兼容字段；实际构建时始终强制为 true
	ReleaseTimeout   time.Duration `json:"releaseTimeout" yaml:"releaseTimeout"`     // 优雅释放等待时间
}

// DefaultAsyncConfig 返回本地开发的默认配置。
func DefaultAsyncConfig() AsyncConfig {
	return AsyncConfig{
		PoolSize:         256,
		MaxBlockingTasks: 0,
		ExpiryDuration:   10 * time.Second,
		Nonblocking:      true,
		ReleaseTimeout:   5 * time.Second,
	}
}
