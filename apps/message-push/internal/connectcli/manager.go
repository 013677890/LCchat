package connectcli

import (
	"context"
	"fmt"
	"sync"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("创建 connect gRPC 连接失败: %w", err)
	}
	m.clients[addr] = conn
	return connectpb.NewConnectServiceClient(conn), nil
}

// Close 关闭所有连接。
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
