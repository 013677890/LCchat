package handler

import (
	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	"github.com/013677890/LCchat-Backend/apps/connect/internal/svc"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// CheckOrigin 按环境变量 CONNECT_ALLOWED_ORIGINS 白名单校验来源。
	// 格式：逗号分隔的域名列表，如 "https://app.example.com,https://admin.example.com"。
	// 本地开发时可设为 "*" 放开所有来源；生产环境必须显式配置白名单。
	CheckOrigin: buildCheckOrigin(),
}

// buildCheckOrigin 构建 WebSocket Origin 校验函数。
func buildCheckOrigin() func(*http.Request) bool {
	raw := strings.TrimSpace(os.Getenv("CONNECT_ALLOWED_ORIGINS"))
	if raw == "" {
		// 未配置时默认拒绝跨域，仅允许同源（Origin 为空或与 Host 一致）。
		return func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			return origin == "http://"+r.Host || origin == "https://"+r.Host
		}
	}
	if raw == "*" {
		// 显式配置 "*" 时放开所有来源（仅用于本地开发）。
		return func(_ *http.Request) bool { return true }
	}
	// 解析白名单并构建快速查找表。
	allowed := make(map[string]struct{})
	for _, origin := range strings.Split(raw, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		_, ok := allowed[origin]
		return ok
	}
}

// WSHandler 负责处理 /ws 接入请求。
// 职责边界：
// - 处理 Gin/HTTP 层参数、升级与错误响应；
// - 调用 svc 完成鉴权与消息解析；
// - 调用 manager 维护连接生命周期。
type WSHandler struct {
	connManager *manager.ConnectionManager
	connectSvc  *svc.ConnectService
}

// NewWSHandler 创建 WebSocket 入口处理器。
func NewWSHandler(connManager *manager.ConnectionManager, connectSvc *svc.ConnectService) *WSHandler {
	return &WSHandler{
		connManager: connManager,
		connectSvc:  connectSvc,
	}
}

// ServeWS 处理 WebSocket 握手与接入。
// 执行流程：
// 1. 从 query 中读取 token/device_id，并获取 client_ip。
// 2. 调用 connectSvc.Authenticate 做鉴权。
// 3. 构建连接级 context（注入 trace/user/device/ip）。
// 4. 完成协议升级并进入连接处理主循环。
func (h *WSHandler) ServeWS(c *gin.Context) {
	token := c.Query("token")
	deviceID := c.Query("device_id")
	clientIP := ctxmeta.ClientIPFromGin(c)
	if clientIP == "" {
		clientIP = c.ClientIP()
	}

	session, err := h.connectSvc.Authenticate(c.Request.Context(), token, deviceID, clientIP)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}

	connCtx := context.Background()
	if traceID := ctxmeta.TraceIDFromGin(c); traceID != "" {
		connCtx = ctxmeta.WithTraceID(connCtx, traceID)
	}
	connCtx = ctxmeta.WithUserUUID(connCtx, session.UserUUID)
	connCtx = ctxmeta.WithDeviceID(connCtx, session.DeviceID)
	connCtx = ctxmeta.WithClientIP(connCtx, session.ClientIP)

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warn(connCtx, "WebSocket 升级失败",
			logger.ErrorField("error", err),
		)
		return
	}

	h.handleConnection(connCtx, conn, session)
}

// handleConnection 承载单个连接的完整生命周期。
// 关键语义：
// - 同设备重复连接时，用新连接替换旧连接；
// - 连接建立/断开分别触发 OnConnect/OnDisconnect；
// - 日志里保留 user_uuid/device_id 便于排障。
func (h *WSHandler) handleConnection(ctx context.Context, conn *websocket.Conn, session *svc.Session) {
	client := manager.NewClient(conn, session.UserUUID, session.DeviceID)
	replaced := h.connManager.Register(client)
	if replaced != nil {
		replaced.Close()
	}

	h.connectSvc.OnConnect(ctx, session)
	logger.Info(ctx, "WebSocket 连接已建立",
		logger.String("user_uuid", session.UserUUID),
		logger.String("device_id", session.DeviceID),
		logger.String("client_ip", session.ClientIP),
		logger.Int("online_count", h.connManager.Count()),
	)

	client.Run(ctx, func(raw []byte) {
		h.handleMessage(ctx, client, session, raw)
	}, func() {
		h.connManager.Unregister(client)
		h.connectSvc.OnDisconnect(ctx, session)
		logger.Info(ctx, "WebSocket 连接已断开",
			logger.String("user_uuid", session.UserUUID),
			logger.String("device_id", session.DeviceID),
			logger.Int("online_count", h.connManager.Count()),
		)
	})
}

// handleMessage 处理客户端上行帧。
// 当前支持：
// - heartbeat: 更新活跃时间并返回 heartbeat_ack；
// - message: 预留消息链路（当前仅回 message_ack 占位）。
func (h *WSHandler) handleMessage(ctx context.Context, client *manager.Client, session *svc.Session, raw []byte) {
	envelope, err := h.connectSvc.ParseEnvelope(raw)
	if err != nil {
		h.sendErrorFrame(ctx, client, consts.CodeConnectMessageFormatError)
		return
	}

	switch envelope.Type {
	case "heartbeat":
		h.connectSvc.OnHeartbeat(ctx, session)
		ack, marshalErr := h.connectSvc.MarshalEnvelope("heartbeat_ack", nil)
		if marshalErr != nil {
			logger.Warn(ctx, "心跳应答序列化失败",
				logger.ErrorField("error", marshalErr),
			)
			return
		}
		if !client.Enqueue(ack) {
			client.Close()
		}
	case "message":
		// TODO: 接入 msg 服务进行消息路由与持久化，并返回投递结果回执。
		ack, marshalErr := h.connectSvc.MarshalEnvelope("message_ack", nil)
		if marshalErr == nil && !client.Enqueue(ack) {
			client.Close()
		}
	default:
		h.sendErrorFrame(ctx, client, consts.CodeConnectMessageTypeNotSupport)
	}
}

// sendErrorFrame 发送 ws 协议层错误帧。
// 发送失败通常表示连接不可写，此时主动关闭连接避免资源泄漏。
func (h *WSHandler) sendErrorFrame(ctx context.Context, client *manager.Client, code int) {
	payload, err := h.connectSvc.MarshalEnvelope("error", svc.ErrorData{
		Code:    code,
		Message: consts.GetMessage(code),
	})
	if err != nil {
		logger.Warn(ctx, "错误帧序列化失败",
			logger.Int("code", code),
			logger.ErrorField("error", err),
		)
		return
	}
	if !client.Enqueue(payload) {
		client.Close()
	}
}

// writeAuthError 将鉴权错误映射为 HTTP 握手阶段错误响应。
// 说明：握手前还未升级为 WebSocket，因此用 HTTP JSON 返回更直观。
func (h *WSHandler) writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, svc.ErrTokenRequired):
		h.writeHTTPError(c, http.StatusBadRequest, consts.CodeConnectTokenRequired)
	case errors.Is(err, svc.ErrDeviceIDRequired):
		h.writeHTTPError(c, http.StatusBadRequest, consts.CodeConnectDeviceIDRequired)
	case errors.Is(err, svc.ErrTokenInvalid):
		h.writeHTTPError(c, http.StatusUnauthorized, consts.CodeInvalidToken)
	default:
		h.writeHTTPError(c, http.StatusInternalServerError, consts.CodeInternalError)
	}
}

// writeHTTPError 按统一 Response 结构输出 HTTP 错误响应。
func (h *WSHandler) writeHTTPError(c *gin.Context, httpStatus, code int) {
	c.Set("business_code", code)
	c.JSON(httpStatus, result.Response{
		Code:      code,
		Message:   consts.GetMessage(code),
		Data:      nil,
		TraceId:   c.GetString("trace_id"),
		Timestamp: time.Now().Unix(),
	})
}
