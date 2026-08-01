//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// WSClient 是一个带事件缓存的 WebSocket 测试客户端。
// 服务端事件可能在等待另一个事件期间到达，因此不能简单地丢弃“不匹配”的帧。
type WSClient struct {
	Name   string
	Conn   *websocket.Conn
	Origin string

	mu      sync.Mutex
	pending []*connectpb.MessageEnvelope
	frames  chan *connectpb.MessageEnvelope

	// stopCh 用于通知读协程停止；readDone 用于等待读协程退出。
	stopCh   chan struct{}
	readDone chan struct{}
	stopOnce sync.Once

	// stateMu 和 failedErr 用来记录底层连接第一次发生的不可恢复读错误。
	// gorilla/websocket 在连接读失败后不允许再次调用 ReadMessage；如果测试继续
	// 等待后续事件，直接重读会触发 "repeated read on failed websocket connection"
	// 的 panic。记录错误后，后续读取统一返回第一次错误，便于测试报告真正原因。
	stateMu   sync.Mutex
	failedErr error
}

// OpenWS 使用登录返回的 access token 和同一 device_id 建立二进制 WebSocket。
// 不设置 Origin 时模拟非浏览器客户端；设置 LCCHAT_CONNECT_WS_ORIGIN 后则覆盖 Origin。
func OpenWS(ctx context.Context, cfg Config, name, token, deviceID string) (*WSClient, error) {
	base, err := url.Parse(strings.TrimRight(cfg.ConnectWSBase, "/") + "/ws")
	if err != nil {
		return nil, fmt.Errorf("解析 WebSocket 地址失败: %w", err)
	}
	query := base.Query()
	query.Set("token", token)
	query.Set("device_id", deviceID)
	base.RawQuery = query.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: cfg.RequestTimeout}
	header := http.Header{}
	if cfg.ConnectWSOrigin != "" {
		header.Set("Origin", cfg.ConnectWSOrigin)
	}
	connection, response, err := dialer.DialContext(ctx, base.String(), header)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("%s WebSocket 握手失败: HTTP %d: %w", name, response.StatusCode, err)
		}
		return nil, fmt.Errorf("%s WebSocket 握手失败: %w", name, err)
	}
	client := &WSClient{
		Name:     name,
		Conn:     connection,
		Origin:   cfg.ConnectWSOrigin,
		frames:   make(chan *connectpb.MessageEnvelope, 256),
		stopCh:   make(chan struct{}),
		readDone: make(chan struct{}),
	}
	go client.readLoop()
	return client, nil
}

// Send 将业务 payload 序列化到 MessageEnvelope。客户端废弃的 type=message 不在测试调用方中使用。
func (w *WSClient) Send(messageType string, payload proto.Message, seq int64) error {
	if err := w.getFailed(); err != nil {
		return err
	}
	data, err := marshalPayload(payload)
	if err != nil {
		return err
	}
	envelope := &connectpb.MessageEnvelope{Type: messageType, Data: data, Seq: seq}
	raw, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("序列化 WebSocket envelope 失败: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.Conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		return fmt.Errorf("%s 写入 WebSocket 失败: %w", w.Name, err)
	}
	return nil
}

func marshalPayload(payload proto.Message) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	return proto.Marshal(payload)
}

// Heartbeat 主动发送心跳并等待 heartbeat_ack，同时也会保留期间收到的其他事件。
func (w *WSClient) Heartbeat(ctx context.Context) error {
	if err := w.Send("heartbeat", nil, 0); err != nil {
		return err
	}
	_, err := w.ReadUntil(ctx, "heartbeat_ack")
	return err
}

// ReadUntil 等待指定 type 的 envelope。
//
// 事件链路是异步的，等待期间每 4 秒发送一次 heartbeat，防止长时间等待事件时
// 被 Connect 误判为不活跃；收到的非目标事件会进入 pending 队列而不会丢失。
// 底层连接由 readLoop 独占读取，本方法只从 frames 通道消费事件，避免多个调用方
// 或读超时重复调用 gorilla/websocket 的 ReadMessage。
func (w *WSClient) ReadUntil(ctx context.Context, expected string) (*connectpb.MessageEnvelope, error) {
	if err := w.getFailed(); err != nil {
		return nil, err
	}
	heartbeat := time.NewTicker(4 * time.Second)
	defer heartbeat.Stop()
	for {
		if envelope := w.takePending(expected); envelope != nil {
			return envelope, nil
		}
		select {
		case envelope := <-w.frames:
			if envelope.Type == expected {
				return envelope, nil
			}
			w.mu.Lock()
			w.pending = append(w.pending, envelope)
			w.mu.Unlock()
		case <-heartbeat.C:
			if err := w.Send("heartbeat", nil, 0); err != nil {
				return nil, err
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("%s 等待 %s 超时: %w", w.Name, expected, ctx.Err())
		case <-w.readDone:
			if err := w.getFailed(); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%s WebSocket 读协程已结束", w.Name)
		}
	}
}

// ReadAny 读取下一帧，主要用于验证旧连接在同设备新连接建立后被关闭。
func (w *WSClient) ReadAny(ctx context.Context) (*connectpb.MessageEnvelope, error) {
	if envelope := w.takeAnyPending(); envelope != nil {
		return envelope, nil
	}
	return w.nextFrame(ctx)
}

// WaitClosed 等待服务端主动关闭连接。读协程收到关闭帧后会退出，
// 本方法只等待 readDone，不会再次触碰底层 WebSocket 读接口。
func (w *WSClient) WaitClosed(ctx context.Context) error {
	// 之前的读取如果已经确认底层连接失败，说明连接已经不可用，
	// WaitClosed 无需再次读取；直接把它视为已关闭，避免重复读触发 panic。
	if err := w.getFailed(); err != nil {
		return nil
	}
	select {
	case <-w.readDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s 在等待关闭期间超时: %w", w.Name, ctx.Err())
	}
}

// AssertNoType 在一个短窗口内确认某类事件没有到达。
// 使用 context 控制等待，不通过反复修改底层读超时来实现轮询，避免破坏连接读状态。
func (w *WSClient) AssertNoType(ctx context.Context, forbidden string) error {
	if err := w.getFailed(); err != nil {
		return err
	}
	for {
		if envelope := w.takePending(forbidden); envelope != nil {
			return fmt.Errorf("收到不应出现的事件 %s", forbidden)
		}
		envelope, err := w.nextFrame(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if envelope.Type == forbidden {
			return fmt.Errorf("收到不应出现的事件 %s", forbidden)
		}
		w.mu.Lock()
		w.pending = append(w.pending, envelope)
		w.mu.Unlock()
	}
}

// readLoop 是每个 WebSocket 客户端唯一的底层读取者。
// WebSocket 库要求连接读失败后不能再次读取，因此把原始读操作集中在这里，
// 上层测试通过 channel 和 context 实现可取消、可超时的等待。
func (w *WSClient) readLoop() {
	defer close(w.readDone)
	for {
		_, raw, err := w.Conn.ReadMessage()
		if err != nil {
			if w.isStopping() {
				return
			}
			w.markFailed(fmt.Errorf("%s 读取 WebSocket 失败: %w", w.Name, err))
			return
		}
		var envelope connectpb.MessageEnvelope
		if err := proto.Unmarshal(raw, &envelope); err != nil {
			w.markFailed(fmt.Errorf("%s 收到非法 protobuf envelope: %w", w.Name, err))
			return
		}
		select {
		case w.frames <- &envelope:
		case <-w.stopCh:
			return
		}
	}
}

func (w *WSClient) nextFrame(ctx context.Context) (*connectpb.MessageEnvelope, error) {
	if err := w.getFailed(); err != nil {
		return nil, err
	}
	select {
	case envelope := <-w.frames:
		return envelope, nil
	case <-w.readDone:
		if err := w.getFailed(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s WebSocket 读协程已结束", w.Name)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *WSClient) takePending(expected string) *connectpb.MessageEnvelope {
	w.mu.Lock()
	defer w.mu.Unlock()
	for index, envelope := range w.pending {
		if envelope.Type != expected {
			continue
		}
		w.pending = append(w.pending[:index], w.pending[index+1:]...)
		return envelope
	}
	return nil
}

func (w *WSClient) takeAnyPending() *connectpb.MessageEnvelope {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return nil
	}
	envelope := w.pending[0]
	w.pending = w.pending[1:]
	return envelope
}

func (w *WSClient) getFailed() error {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	return w.failedErr
}

func (w *WSClient) markFailed(err error) error {
	if err == nil {
		return nil
	}
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if w.failedErr == nil {
		w.failedErr = err
	}
	return w.failedErr
}

func (w *WSClient) isStopping() bool {
	select {
	case <-w.stopCh:
		return true
	default:
		return false
	}
}

func (w *WSClient) Close() error {
	if w == nil || w.Conn == nil {
		return nil
	}
	w.stopOnce.Do(func() { close(w.stopCh) })
	return w.Conn.Close()
}
