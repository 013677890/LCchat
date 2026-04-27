package svc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
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



// Envelope 定义 WebSocket 通用消息包格式。

// 约定：

// - Type: 消息类型（如 heartbeat/message）；

// - Data: 消息体（由上层按 Type 再解析）。

type Envelope struct {

	Type string          `json:"type"`

	Data json.RawMessage `json:"data,omitempty"`

}



// ErrorData 定义 type=error 时的 data 结构。

type ErrorData struct {

	Code    int    `json:"code"`

	Message string `json:"message"`

}



// ConnectService 承载 connect 的核心业务逻辑。

type ConnectService struct {

	redisClient      *redis.Client

	userDeviceClient userpb.DeviceServiceClient // 可为 nil，降级时跳过 RPC

	activeSyncer     *deviceactive.Syncer

	statusQueue      chan deviceStatusTask // 设备状态 RPC 任务队列

	statusWg         sync.WaitGroup        // 等待工作协程退出

}



// NewConnectService 创建业务服务实例。

// userDeviceClient 可为 nil：此时设备状态 RPC 会被跳过（降级运行）。

// activeSyncer 由 InitActiveSyncer 在进程启动阶段注入，避免在 Wire 装配时启动同步器 goroutine。

func NewConnectService(redisClient *redis.Client, userDeviceClient userpb.DeviceServiceClient) *ConnectService {

	s := &ConnectService{

		redisClient:      redisClient,

		userDeviceClient: userDeviceClient,

	}



	// 仅在 userDeviceClient 可用时启动工作协程。

	if userDeviceClient != nil {

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

	if s.userDeviceClient == nil || s.activeSyncer != nil {

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

				activeItems := make([]*userpb.UpdateDeviceActiveItem, 0, end-start)

				for i := start; i < end; i++ {

					activeItems = append(activeItems, &userpb.UpdateDeviceActiveItem{

						UserUuid: items[i].UserUUID,

						DeviceId: items[i].DeviceID,

					})

				}

				rpcCtx, cancel := context.WithTimeout(context.Background(), cfg.RPCTimeout)

				_, callErr := s.userDeviceClient.UpdateDeviceActive(rpcCtx, &userpb.UpdateDeviceActiveRequest{Items: activeItems})

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

	return nil

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



// ParseEnvelope 解析客户端上行帧。

// 若 type 缺失或 JSON 不合法，会返回错误交由 handler 返回 error 帧。

func (s *ConnectService) ParseEnvelope(raw []byte) (*Envelope, error) {

	var envelope Envelope

	if err := json.Unmarshal(raw, &envelope); err != nil {

		return nil, err

	}

	envelope.Type = strings.TrimSpace(envelope.Type)

	if envelope.Type == "" {

		return nil, errors.New("type is required")

	}

	return &envelope, nil

}



// MarshalEnvelope 组装并序列化下行帧。

// 约定：data=nil 时省略 data 字段，避免无意义空对象。

func (s *ConnectService) MarshalEnvelope(msgType string, data any) ([]byte, error) {

	envelope := map[string]any{

		"type": msgType,

	}

	if data != nil {
		envelope["data"] = data
	}
	return json.Marshal(envelope)
}

