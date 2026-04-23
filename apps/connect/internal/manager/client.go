package manager

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultSendQueueSize = 64
	// wsWriteTimeout 单次写操作超时，避免慢连接长期阻塞写协程。
	wsWriteTimeout = 5 * time.Second
	// wsPongWait 读取超时窗口：若该时间内未收到任何数据或 Pong，判定连接失活。
	wsPongWait = 60 * time.Second
	// wsPingPeriod 主动 Ping 周期，通常小于 pongWait，确保超时窗口持续被刷新。
	wsPingPeriod = wsPongWait * 9 / 10
	// wsMaxMessageSize 限制单条上行消息大小，防止超大包导致内存风险。
	wsMaxMessageSize = 1 << 20 // 1MB
	// wsBatchDrainLimit 单次唤醒最多额外清空的排队消息数。
	// 目的：在高峰期减少 goroutine 调度与锁竞争开销。
	wsBatchDrainLimit = 16
)

// MessageHandler 定义上行消息回调。
// 参数 raw 为客户端原始二进制载荷（通常是 JSON 编码后的字节）。
type MessageHandler func(raw []byte)

// CloseHandler 定义连接关闭回调。
// 用于在 read/write 循环退出后执行清理逻辑（例如从 manager 注销）。
type CloseHandler func()

type outboundFrame struct {
	messageType int
	payload     []byte
}

// Client 封装单条 WebSocket 连接。
// 设计要点：
// - send 队列用于削峰，避免业务 goroutine 直接阻塞在网络写；
// - done 用于统一关闭信号，读写循环都监听该信号退出；
// - once 保证 Close 幂等，避免重复 close channel/panic。
type Client struct {
	conn     *websocket.Conn
	userUUID string
	deviceID string
	send     chan outboundFrame
	done     chan struct{}
	once     sync.Once
}

// NewClient 创建连接包装对象。
func NewClient(conn *websocket.Conn, userUUID, deviceID string) *Client {
	return &Client{
		conn:     conn,
		userUUID: userUUID,
		deviceID: deviceID,
		send:     make(chan outboundFrame, defaultSendQueueSize),
		done:     make(chan struct{}),
	}
}

func (c *Client) UserUUID() string {
	return c.userUUID
}

func (c *Client) DeviceID() string {
	return c.deviceID
}

// Done 返回连接关闭信号通道。
// 外部可通过监听该通道感知连接生命周期结束。
func (c *Client) Done() <-chan struct{} {
	return c.done
}

// Enqueue 将文本帧投递到写队列。
// 返回值语义：
// - true：已成功入队；
// - false：连接已关闭或队列已满。
func (c *Client) Enqueue(msg []byte) bool {
	return c.enqueue(websocket.TextMessage, msg)
}

// EnqueueBinary 将二进制帧投递到写队列。
func (c *Client) EnqueueBinary(msg []byte) bool {
	return c.enqueue(websocket.BinaryMessage, msg)
}

func (c *Client) enqueue(messageType int, msg []byte) bool {
	if len(msg) == 0 {
		return true
	}
	frame := outboundFrame{
		messageType: messageType,
		payload:     append([]byte(nil), msg...),
	}
	select {
	case <-c.done:
		return false
	case c.send <- frame:
		return true
	default:
		return false
	}
}

// Run 启动读写循环并阻塞等待 readLoop 结束。
// 行为说明：
// - writeLoop 在独立 goroutine 中运行；
// - readLoop 在当前 goroutine 运行，通常由其错误/断连触发整体退出；
// - 退出时保证调用 Close 和 onClose，确保资源回收。
func (c *Client) Run(ctx context.Context, onMessage MessageHandler, onClose CloseHandler) {
	defer func() {
		c.Close()
		if onClose != nil {
			onClose()
		}
	}()

	c.conn.SetReadLimit(wsMaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	go c.writeLoop(ctx)
	c.readLoop(onMessage)
}

// Close 幂等关闭连接。
// 关闭顺序：
// 1. 关闭 done 信号，通知读写循环退出；
// 2. 关闭底层 websocket 连接释放网络资源。
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// CloseGracefully 先向客户端发送 CloseGoingAway 帧，再关闭连接。
// 用于优雅停机场景：客户端收到 GoingAway 后知道服务端正在维护，
// 可立即尝试重连到其他节点，而不是当作异常断线处理。
func (c *Client) CloseGracefully() {
	deadline := time.Now().Add(wsWriteTimeout)
	msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down")
	_ = c.conn.WriteControl(websocket.CloseMessage, msg, deadline)
	c.Close()
}

// readLoop 持续读取客户端上行帧并交由 onMessage 处理。
// 注意：ReadMessage 是阻塞调用，不使用 select 轮询 ctx/done。
// 退出依赖连接关闭（Close）或网络读错误。
func (c *Client) readLoop(onMessage MessageHandler) {
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		if onMessage != nil {
			onMessage(raw)
		}
	}
}

// writeLoop 持续从 send 队列取消息写入客户端。
// 同时按固定周期发送 Ping 保活，收到 Pong 后由读协程刷新读超时。
func (c *Client) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 通过关闭底层连接打断 readLoop 的阻塞 ReadMessage。
			c.Close()
			return
		case <-c.done:
			return
		case frame := <-c.send:
			if err := c.writeBatch(frame); err != nil {
				c.Close()
				return
			}
		case <-ticker.C:
			if err := c.writePing(); err != nil {
				c.Close()
				return
			}
		}
	}
}

// writeBatch 先发送当前消息，再尽量清空队列中已积压的消息。
// 说明：每条业务消息仍保持独立 WebSocket 帧语义，避免破坏上层协议解析。
func (c *Client) writeBatch(first outboundFrame) error {
	if err := c.writeFrame(first); err != nil {
		return err
	}

	for i := 0; i < wsBatchDrainLimit; i++ {
		select {
		case frame := <-c.send:
			if err := c.writeFrame(frame); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return nil
}

// writeFrame 使用 NextWriter 发送单条帧。
func (c *Client) writeFrame(frame outboundFrame) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	writer, err := c.conn.NextWriter(frame.messageType)
	if err != nil {
		return err
	}
	if _, err = writer.Write(frame.payload); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

// writePing 发送协议层 Ping 保活包。
func (c *Client) writePing() error {
	deadline := time.Now().Add(wsWriteTimeout)
	_ = c.conn.SetWriteDeadline(deadline)
	return c.conn.WriteControl(websocket.PingMessage, nil, deadline)
}
