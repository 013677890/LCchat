package realtime

import (
	"context"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
)

// pushRealtimeRoutes 逐设备串行调用 connect PushToDevice，并返回成功/失败数量。
//
// 与 msgpush 节点有界并发 + PushToUser 不同：realtime 目标种类多、单事件规模通常更小，
// 当前刻意保持「逐设备 PushToDevice」——简单、结果可按设备归因，也避免在未证明瓶颈前
// 引入与消息下行相同的复杂扇出内核。
//
// 语义对齐：
//   - 投递前再去重一次，防止目标解析路径漏去重导致重复 RPC；
//   - ctx 取消/超时时立即停止后续设备，未尝试的不计失败（与 msgpush 一致）；
//   - 设备级指标粒度与 msgpush 的 PushToDevice 路径共用同一组 Prometheus 标签。
func (h *Handler) pushRealtimeRoutes(
	ctx context.Context,
	eventType string,
	routes []route.DeviceRoute,
	envelope *connectpb.MessageEnvelope,
) (int, int) {
	routes = dedupeDeviceRoutes(routes)
	var successCount, failedCount int
	for _, deviceRoute := range routes {
		if ctx.Err() != nil {
			// 处理预算到期/关停：停止继续扇出，避免对剩余设备做无谓的快失败调用。
			break
		}
		pushStart := time.Now()
		pushErr := h.sender.PushToDevice(
			ctx,
			deviceRoute.ConnectGRPCAddr,
			deviceRoute.UserUUID,
			deviceRoute.DeviceID,
			envelope,
		)
		if pushErr != nil {
			failedCount++
			metrics.PushToDeviceTotal.WithLabelValues(eventType, "error").Inc()
			metrics.ObservePushToDeviceDuration(pushStart, "error")
			logger.Warn(ctx, "message-push realtime.push 调用 connect PushToDevice 失败",
				logger.String("target_user_uuid", deviceRoute.UserUUID),
				logger.String("device_id", deviceRoute.DeviceID),
				logger.String("connect_addr", deviceRoute.ConnectGRPCAddr),
				logger.ErrorField("error", pushErr),
			)
			continue
		}
		successCount++
		metrics.PushToDeviceTotal.WithLabelValues(eventType, "success").Inc()
		metrics.ObservePushToDeviceDuration(pushStart, "success")
	}

	metrics.DeliveredDevices.Observe(float64(successCount))
	return successCount, failedCount
}
