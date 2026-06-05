package svc

import (
	"context"
	"sync"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// Session 保存连接鉴权后的身份信息。
// 该结构会在整个连接生命周期中复用，避免重复解析 token。
type Session struct {
	UserUUID string
	DeviceID string
	ClientIP string
}

// ConnectService 承载 connect 的核心业务逻辑。
type ConnectService struct {
	redisClient      *redis.Client
	authDeviceClient authpb.DeviceServiceClient // 可为 nil，降级时跳过 RPC
	activeSyncer     *deviceactive.Syncer
	statusQueue      chan deviceStatusTask // 设备状态 RPC 任务队列
	statusWg         sync.WaitGroup        // 等待工作协程退出
	// routeTTL 路由 Key 的写入 TTL。由 InitActiveSyncer 按 2×UpdateInterval 派生，
	// 保证 TTL >= 2×心跳节流间隔，避免“节流到期才刷新”时 Key 在下一次刷新前过期造成路由黑洞。
	// 为 0 表示未初始化（如降级/测试路径），此时回退到 defaultRouteTTL。
	routeTTL time.Duration
}

// NewConnectService 创建业务服务实例。
// authDeviceClient 可为 nil：此时设备状态 RPC 会被跳过（降级运行）。
// activeSyncer 由 InitActiveSyncer 在进程启动阶段注入，避免在 Wire 装配时启动同步器 goroutine。
func NewConnectService(redisClient *redis.Client, authDeviceClient authpb.DeviceServiceClient) *ConnectService {
	s := &ConnectService{
		redisClient:      redisClient,
		authDeviceClient: authDeviceClient,
	}

	// 仅在 authDeviceClient 可用时启动工作协程。
	if authDeviceClient != nil {
		s.statusQueue = make(chan deviceStatusTask, statusQueueSize)
		for i := 0; i < statusWorkerCount; i++ {
			s.statusWg.Add(1)
			go s.statusWorker()
		}
	}

	return s
}

// RedisClient 返回构造期注入的 Redis 客户端（可为 nil）。
func (s *ConnectService) RedisClient() *redis.Client {
	return s.redisClient
}

// InitActiveSyncer 在进程启动阶段初始化设备活跃同步器。
// 必须在对外提供服务前调用，避免在 Wire 装配阶段启动后台 goroutine。
func (s *ConnectService) InitActiveSyncer(cfg config.DeviceActiveConfig) error {
	if s.authDeviceClient == nil || s.activeSyncer != nil {
		return nil
	}

	syncer, err := deviceactive.NewSyncer(deviceactive.Config{
		ShardCount:     cfg.ShardCount,
		UpdateInterval: cfg.UpdateInterval,
		FlushInterval:  cfg.FlushInterval,
		WorkerCount:    cfg.WorkerCount,
		QueueSize:      cfg.QueueSize,
		BatchHandler: func(_ context.Context, items []deviceactive.BatchItem) error {
			const batchSize = 1000

			var firstErr error
			for start := 0; start < len(items); start += batchSize {
				end := start + batchSize
				if end > len(items) {
					end = len(items)
				}

				activeItems := make([]*authpb.UpdateDeviceActiveItem, 0, end-start)
				for i := start; i < end; i++ {
					activeItems = append(activeItems, &authpb.UpdateDeviceActiveItem{
						UserUuid: items[i].UserUUID,
						DeviceId: items[i].DeviceID,
					})
				}

				rpcCtx, cancel := context.WithTimeout(context.Background(), cfg.RPCTimeout)
				_, callErr := s.authDeviceClient.UpdateDeviceActive(rpcCtx, &authpb.UpdateDeviceActiveRequest{Items: activeItems})
				cancel()
				if callErr != nil && firstErr == nil {
					logger.Error(context.Background(), "Connect 批量更新设备活跃时间失败",
						logger.ErrorField("error", callErr),
						logger.Int("item_count", len(activeItems)),
					)
					firstErr = callErr
				}
			}

			return firstErr
		},
	})
	if err != nil {
		return err
	}

	s.activeSyncer = syncer
	// 路由 Key TTL 必须 >= 2×UpdateInterval：心跳刷新被节流到每 UpdateInterval 一次，
	// TTL 需留出余量，否则 Key 可能在下一次刷新前过期，造成瞬时路由黑洞（Defect 10）。
	s.routeTTL = 2 * cfg.UpdateInterval
	if s.routeTTL < defaultRouteTTL {
		s.routeTTL = defaultRouteTTL
	}
	return nil
}

// effectiveRouteTTL 返回实际使用的路由 Key TTL。
// 未通过 InitActiveSyncer 初始化（降级/测试路径，无节流）时回退到 defaultRouteTTL。
func (s *ConnectService) effectiveRouteTTL() time.Duration {
	if s.routeTTL > 0 {
		return s.routeTTL
	}
	return defaultRouteTTL
}

// ShutdownStatusWorkers 优雅关闭后台协程。
func (s *ConnectService) ShutdownStatusWorkers() {
	if s.statusQueue != nil {
		close(s.statusQueue)
		s.statusWg.Wait()
	}
	if s.activeSyncer != nil {
		s.activeSyncer.Stop()
	}
}
