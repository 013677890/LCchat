package svc

import (
	"sync"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/pkg/presence"
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
	statusQueue      chan deviceStatusTask      // 设备状态 RPC 任务队列
	statusWg         sync.WaitGroup             // 等待工作协程退出
	// routeTTL 路由 Key 的写入 TTL。presence 契约下 connect 是唯一写方，
	// 心跳无条件刷新路由，TTL 只需覆盖数个心跳周期以容忍抖动。
	// 为 0 表示未注入（测试路径），此时回退到 presence.DefaultRouteTTL。
	routeTTL time.Duration
}

// NewConnectService 创建业务服务实例。
// authDeviceClient 可为 nil：此时设备状态 RPC 会被跳过（降级运行）。
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

// SetRouteTTL 在进程启动阶段注入路由 Key 写入 TTL（CONNECT_ROUTE_TTL_SECONDS）。
// 必须在对外提供服务前调用；非正值会被忽略并沿用默认值。
func (s *ConnectService) SetRouteTTL(ttl time.Duration) {
	if ttl > 0 {
		s.routeTTL = ttl
	}
}

// effectiveRouteTTL 返回实际使用的路由 Key TTL。
// 未注入（降级/测试路径）时回退到 presence.DefaultRouteTTL。
func (s *ConnectService) effectiveRouteTTL() time.Duration {
	if s.routeTTL > 0 {
		return s.routeTTL
	}
	return presence.DefaultRouteTTL
}

// ShutdownStatusWorkers 优雅关闭后台协程。
func (s *ConnectService) ShutdownStatusWorkers() {
	if s.statusQueue != nil {
		close(s.statusQueue)
		s.statusWg.Wait()
	}
}
