package middleware

import (
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"sync"
	"time"
)

var (
	deviceActiveSyncerMu sync.RWMutex
	deviceActiveSyncer   *deviceactive.Syncer
)

// InitDeviceActiveSyncer 初始化活跃时间同步器（Gateway）。
func InitDeviceActiveSyncer(cfg config.DeviceActiveConfig, handler deviceactive.BatchHandler) error {
	syncer, err := deviceactive.NewSyncer(deviceactive.Config{
		ShardCount:     cfg.ShardCount,
		UpdateInterval: cfg.UpdateInterval,
		FlushInterval:  cfg.FlushInterval,
		WorkerCount:    cfg.WorkerCount,
		QueueSize:      cfg.QueueSize,
		BatchHandler:   handler,
	})
	if err != nil {
		return err
	}

	deviceActiveSyncerMu.Lock()
	old := deviceActiveSyncer
	deviceActiveSyncer = syncer
	deviceActiveSyncerMu.Unlock()

	if old != nil {
		old.Stop()
	}
	return nil
}

// ShutdownDeviceActiveSyncer 停止活跃时间同步器。
func ShutdownDeviceActiveSyncer() {
	deviceActiveSyncerMu.Lock()
	syncer := deviceActiveSyncer
	deviceActiveSyncer = nil
	deviceActiveSyncerMu.Unlock()

	if syncer != nil {
		syncer.Stop()
	}
}

func updateDeviceActive(userUUID, deviceID string) {
	if userUUID == "" || deviceID == "" {
		return
	}

	deviceActiveSyncerMu.RLock()
	syncer := deviceActiveSyncer
	deviceActiveSyncerMu.RUnlock()
	if syncer == nil {
		return
	}

	_ = syncer.Touch(userUUID, deviceID, time.Now())
}
