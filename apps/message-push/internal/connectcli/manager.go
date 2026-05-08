package connectcli

import (
	"context"
	"fmt"
	"sync"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"google.golang.org/grpc"
)

// ClientManager 管理按地址复用的 connect gRPC client。
type ClientManager struct {
	mu      sync.Mutex
	clients map[string]*grpc.ClientConn
}

// NewClientManager 创建 connect client 管理器。
func NewClientManager() *ClientManager {
	return &ClientManager{clients: make(map[string]*grpc.ClientConn)}
}

// Get 获取指定地址的 client，内部复用连接。
func (m *ClientManager) Get(addr string) (connectpb.ConnectServiceClient, error) {
	if addr == "" {
		return nil, fmt.Errorf("connect 地址不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.clients[addr]; ok {
		return connectpb.NewConnectServiceClient(conn), nil
	}

	// 首次访问某个 connect 地址时才建立连接。
	// 建好后放入缓存，后续相同地址的推送请求直接复用。
	// 这条连接只调用 connect.ConnectService，
	// 因此 service config、timeout 与日志策略都可以直接复用统一 builder。
	conn, err := grpcx.NewClient(grpcx.ClientOptions{
		Address: string(addr),
		Timeout: &grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()},
		Retry:   grpcx.DefaultClientRetryConfig("connect.ConnectService"),
	})
	if err != nil {
		return nil, fmt.Errorf("创建 connect gRPC 连接失败: %w", err)
	}
	m.clients[addr] = conn
	return connectpb.NewConnectServiceClient(conn), nil
}

// Close 关闭所有连接。
// 保留 ctx 参数以兼容上层统一的生命周期签名；当前 grpc.ClientConn.Close 是本地快速操作，
// 不需要也不接收 ctx 控制，若未来引入需要超时的收敛逻辑（如等待 in-flight RPC 结束）再启用。
func (m *ClientManager) Close(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for addr, conn := range m.clients {
		if conn == nil {
			delete(m.clients, addr)
			continue
		}
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭 connect 连接失败: %w", err)
		}
		delete(m.clients, addr)
	}
	return firstErr
}

// Sender 封装 connect 推送调用。
type Sender struct {
	manager     *ClientManager
	userTimeout time.Duration
}

// NewSender 创建 sender。
func NewSender(manager *ClientManager, userTimeout time.Duration) *Sender {
	return &Sender{manager: manager, userTimeout: userTimeout}
}

// PushToUser 向指定 connect 节点上的用户在线设备推送。
func (s *Sender) PushToUser(ctx context.Context, connectAddr, userUUID string, envelope *connectpb.MessageEnvelope) (int32, error) {
	client, err := s.manager.Get(connectAddr)
	if err != nil {
		return 0, err
	}
	callCtx := ctx
	cancel := func() {}
	if s.userTimeout > 0 {
		// 每次下行投递都套一层短超时。
		// 某个 connect 节点卡住时，不应拖慢整个 Kafka 消费循环。
		callCtx, cancel = context.WithTimeout(ctx, s.userTimeout)
	}
	defer cancel()

	resp, err := client.PushToUser(callCtx, &connectpb.PushToUserRequest{
		UserUuid: userUUID,
		Message:  envelope,
	})
	if err != nil {
		return 0, fmt.Errorf("调用 connect PushToUser 失败: %w", err)
	}
	if resp == nil {
		return 0, nil
	}
	return resp.DeliveredCount, nil
}

// PushToDevice 向指定 connect 节点上的指定设备推送。
func (s *Sender) PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error {
	client, err := s.manager.Get(connectAddr)
	if err != nil {
		return err
	}
	callCtx := ctx
	cancel := func() {}
	if s.userTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, s.userTimeout)
	}
	defer cancel()

	resp, err := client.PushToDevice(callCtx, &connectpb.PushToDeviceRequest{
		UserUuid: userUUID,
		DeviceId: deviceID,
		Message:  envelope,
	})
	if err != nil {
		return fmt.Errorf("调用 connect PushToDevice 失败: %w", err)
	}
	if resp == nil || !resp.Delivered {
		return fmt.Errorf("设备不在线或推送失败")
	}
	return nil
}
