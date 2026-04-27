package util

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"context"
	"net"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// Context 相关的 key 常量
const (
	ContextKeyDeviceID = ctxmeta.KeyDeviceID
	ContextKeyClientIP = ctxmeta.KeyClientIP
	ContextKeyUserUUID = ctxmeta.KeyUserUUID
)

// GetDeviceIDFromContext 从 context 中获取 device_id
// device_id 应该由 interceptor 从请求头或 metadata 中提取并注入到 context
func GetDeviceIDFromContext(ctx context.Context) string {
	// 尝试从 context value 中获取
	if deviceID := ctxmeta.DeviceID(ctx); deviceID != "" {
		return deviceID
	}

	// 尝试从 gRPC metadata 中获取
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(ctxmeta.MetadataDeviceID); len(values) > 0 {
			return values[0]
		}
		// 兼容历史写法
		if values := md.Get("device-id"); len(values) > 0 {
			return values[0]
		}
	}

	// 如果都没有，返回一个默认值（建议在 interceptor 中强制要求传递）
	return ""
}

// GetClientIPFromContext 从 context 中获取客户端 IP
func GetClientIPFromContext(ctx context.Context) string {
	// 尝试从 context value 中获取（如果 interceptor 已经解析并注入）
	if clientIP := ctxmeta.ClientIP(ctx); clientIP != "" {
		return clientIP
	}

	// 尝试从 gRPC metadata 中获取（如果是通过网关转发，可能会有 X-Real-IP 或 X-Forwarded-For）
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		// 优先使用 X-Real-IP
		if values := md.Get(ctxmeta.MetadataXRealIP); len(values) > 0 {
			return values[0]
		}
		// 其次使用 X-Forwarded-For 的第一个 IP
		if values := md.Get(ctxmeta.MetadataXForwardedFor); len(values) > 0 {
			return values[0]
		}
		// 最后回退 client_ip metadata
		if values := md.Get(ctxmeta.MetadataClientIP); len(values) > 0 {
			return values[0]
		}
	}

	// 最后尝试从 peer 中获取直连 IP
	if p, ok := peer.FromContext(ctx); ok {
		if tcpAddr, ok := p.Addr.(*net.TCPAddr); ok {
			return tcpAddr.IP.String()
		}
		// 如果不是 TCP，尝试解析字符串
		return parseIPFromAddr(p.Addr.String())
	}

	return ""
}

// parseIPFromAddr 从地址字符串中解析 IP（格式：ip:port）
func parseIPFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr // 如果解析失败，直接返回原始地址
	}
	return host
}

// GetUserUUIDFromContext 从 context 中获取用户 UUID（用于认证后的接口）
func GetUserUUIDFromContext(ctx context.Context) string {
	return ctxmeta.UserUUID(ctx)
}
