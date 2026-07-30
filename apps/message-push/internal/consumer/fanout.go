package consumer

import (
	"context"
	"sort"
	"sync"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// fanoutSummary 汇总所有已启动节点的实际投递结果。
// ctx 取消后尚未启动或尚未尝试的目标不计入失败数，与原串行循环 break 的语义一致。
type fanoutSummary struct {
	SuccessCount int
	FailedCount  int
	Nodes        []nodeFanoutResult
}

// nodeFanoutResult 用于最终结构化日志，保留节点维度以快速识别慢节点或故障节点。
type nodeFanoutResult struct {
	ConnectAddr  string `json:"connect_addr"`
	DeviceCount  int    `json:"device_count"`
	SuccessCount int    `json:"success_count"`
	FailedCount  int    `json:"failed_count"`
}

// nodeFanout 是单个 connect 节点的执行单元。
// users 保持首次路由出现顺序，节点 worker 会依次处理，避免对同一连接制造额外并发。
type nodeFanout struct {
	connectAddr string
	deviceCount int
	users       []*userFanout
	userIndexes map[string]int
}

// userFanout 表示同一 connect 节点上的一个用户目标。
// allowUserPush 为 true 时，一次 PushToUser 可覆盖 routes 中的全部已知设备。
type userFanout struct {
	userUUID      string
	routes        []route.DeviceRoute
	allowUserPush bool
}

// fanoutAccumulator 用短临界区合并节点结果。
// gRPC 调用全部发生在锁外，慢节点不会阻塞其他节点写入统计。
type fanoutAccumulator struct {
	mu      sync.Mutex
	summary fanoutSummary
}

// pushEventTargets 按 connect 节点执行有界并发，并等待所有已启动节点完成。
// 信号量在创建 goroutine 之前获取，因此限制的不只是活跃 RPC，也限制了活跃 goroutine 数；
// 节点数远超配置时，调度循环会等待空位，不会一次性创建大量阻塞协程。
func (h *EventHandler) pushEventTargets(
	ctx context.Context,
	eventType string,
	targets []deliveryTarget,
	envelope *connectpb.MessageEnvelope,
) fanoutSummary {
	// 先完成纯内存分组，之后每个 nodeFanout 都只会被一个 goroutine 读取，无需节点内加锁。
	nodes := groupTargetsByNode(targets)
	accumulator := &fanoutAccumulator{}

	// 容量取 min(节点数, 配置值)，避免异常超大配置导致无意义的大 channel 分配。
	semaphore := make(chan struct{}, h.fanoutConcurrency(len(nodes)))
	var waitGroup sync.WaitGroup

launchLoop:
	for _, node := range nodes {
		select {
		case semaphore <- struct{}{}:
			// 先占并发槽再启动 worker，确保任何时刻活跃节点都不超过上限。
			waitGroup.Add(1)
			go func(currentNode *nodeFanout) {
				defer waitGroup.Done()
				defer func() { <-semaphore }()

				accumulator.add(h.pushNode(ctx, eventType, currentNode, envelope))
			}(node)
		case <-ctx.Done():
			// 停止调度新节点；已启动节点会在 RPC 和节点内循环中观察同一个 ctx。
			break launchLoop
		}
	}

	// 即使 ctx 已取消也必须等待已启动 worker 收敛，避免 Handle 返回后仍修改统计或继续投递。
	waitGroup.Wait()
	summary := accumulator.snapshot()
	metrics.DeliveredDevices.Observe(float64(summary.SuccessCount))
	return summary
}

// fanoutConcurrency 返回本次事件实际需要的节点并发数。
// maxFanoutConcurrency 在构造阶段已经强制校验；这里再次检查内部不变量，
// 防止包内直接构造非法 EventHandler 后创建零容量信号量并永久阻塞。
func (h *EventHandler) fanoutConcurrency(nodeCount int) int {
	if h.maxFanoutConcurrency <= 0 {
		panic("message-push: maxFanoutConcurrency 必须大于零")
	}
	if h.maxFanoutConcurrency > nodeCount {
		return nodeCount
	}
	return h.maxFanoutConcurrency
}

// pushNode 在单个 connect 节点内串行执行按用户或按设备推送。
// 节点内串行可避免同一 gRPC 连接上的瞬时请求洪峰；并发收益主要来自不同 connect 节点，
// 而同用户多设备的调用压缩由 PushToUser 完成。
func (h *EventHandler) pushNode(
	ctx context.Context,
	eventType string,
	node *nodeFanout,
	envelope *connectpb.MessageEnvelope,
) nodeFanoutResult {
	result := nodeFanoutResult{
		ConnectAddr: node.connectAddr,
		DeviceCount: node.deviceCount,
	}
	for _, user := range node.users {
		if ctx.Err() != nil {
			break
		}
		if user.allowUserPush {
			// 只有路由收集阶段确认“完整用户目标”后才允许广播，不能仅因设备数大于一就批量。
			successCount, failedCount := pushUser(ctx, h.sender, eventType, node.connectAddr, user, envelope)
			result.SuccessCount += successCount
			result.FailedCount += failedCount
			continue
		}
		successCount, failedCount := h.pushDevices(ctx, eventType, node.connectAddr, user.routes, envelope)
		result.SuccessCount += successCount
		result.FailedCount += failedCount
	}
	return result
}

// pushDevices 在同一 connect 节点内逐设备串行推送。
// 每次调用前检查 ctx，避免预算到期后对剩余设备发起一串必然失败的 RPC。
func (h *EventHandler) pushDevices(
	ctx context.Context,
	eventType string,
	connectAddr string,
	routes []route.DeviceRoute,
	envelope *connectpb.MessageEnvelope,
) (int, int) {
	successCount := 0
	failedCount := 0
	for _, deviceRoute := range routes {
		if ctx.Err() != nil {
			break
		}
		if pushDevice(ctx, h.sender, eventType, connectAddr, deviceRoute, envelope) {
			successCount++
		} else {
			failedCount++
		}
	}
	return successCount, failedCount
}

// pushDevice 推送单设备并保留原有设备级指标粒度。
// PushToDevice 返回 nil 才表示目标设备成功入队；离线和 RPC 错误都按失败处理。
func pushDevice(
	ctx context.Context,
	sender deviceSender,
	eventType string,
	connectAddr string,
	deviceRoute route.DeviceRoute,
	envelope *connectpb.MessageEnvelope,
) bool {
	pushStart := time.Now()
	err := sender.PushToDevice(
		ctx,
		connectAddr,
		deviceRoute.UserUUID,
		deviceRoute.DeviceID,
		envelope,
	)
	if err == nil {
		metrics.PushToDeviceTotal.WithLabelValues(eventType, "success").Inc()
		metrics.ObservePushToDeviceDuration(pushStart, "success")
		return true
	}
	metrics.PushToDeviceTotal.WithLabelValues(eventType, "error").Inc()
	metrics.ObservePushToDeviceDuration(pushStart, "error")
	logger.Warn(ctx, "message-push 调用 connect PushToDevice 失败",
		logger.String("target_user_uuid", deviceRoute.UserUUID),
		logger.String("device_id", deviceRoute.DeviceID),
		logger.String("connect_addr", connectAddr),
		logger.ErrorField("error", err),
	)
	return false
}

// pushUser 按用户调用一次 PushToUser，并把 DeliveredCount 折算为设备成功/失败数。
// expectedCount 来自 Redis 路由快照，DeliveredCount 来自 connect 执行时的实时连接集合；
// 二者可能因连接上下线存在差异，因此按数量而不是设备身份进行结果归并。
func pushUser(
	ctx context.Context,
	sender eventSender,
	eventType string,
	connectAddr string,
	user *userFanout,
	envelope *connectpb.MessageEnvelope,
) (int, int) {
	pushStart := time.Now()
	deliveredCount, err := sender.PushToUser(ctx, connectAddr, user.userUUID, envelope)
	result, successCount, failedCount := classifyUserPush(len(user.routes), int(deliveredCount), err)
	metrics.PushToUserTotal.WithLabelValues(eventType, result).Inc()
	metrics.ObservePushToUserDuration(pushStart, result)
	if result != "success" {
		logUserPushIssue(ctx, connectAddr, user.userUUID, len(user.routes), successCount, err)
	}
	return successCount, failedCount
}

// classifyUserPush 根据路由快照和返回计数区分成功、部分成功与失败。
// RPC 成功但零投递等价于原 PushToDevice 的“设备不在线”，应计为失败并允许整体重试；
// 返回数大于预期表示快照后新增连接，按实际成功数记录而不制造虚假失败。
func classifyUserPush(expectedCount, deliveredCount int, err error) (string, int, int) {
	if err != nil || deliveredCount <= 0 {
		return "error", 0, expectedCount
	}
	if deliveredCount < expectedCount {
		return "partial", deliveredCount, expectedCount - deliveredCount
	}
	return "success", deliveredCount, 0
}

// logUserPushIssue 记录 PushToUser 调用失败或部分投递，便于定位节点级异常。
// PushToUser 不返回设备明细，因此日志只记录预期数和实际数，避免伪造失败设备身份。
func logUserPushIssue(
	ctx context.Context,
	connectAddr string,
	userUUID string,
	expectedCount int,
	deliveredCount int,
	err error,
) {
	if err != nil {
		logger.Warn(ctx, "message-push 调用 connect PushToUser 未完整投递",
			logger.String("target_user_uuid", userUUID),
			logger.String("connect_addr", connectAddr),
			logger.Int("expected_device_count", expectedCount),
			logger.Int("delivered_count", deliveredCount),
			logger.ErrorField("error", err),
		)
		return
	}
	logger.Warn(ctx, "message-push 调用 connect PushToUser 未完整投递",
		logger.String("target_user_uuid", userUUID),
		logger.String("connect_addr", connectAddr),
		logger.Int("expected_device_count", expectedCount),
		logger.Int("delivered_count", deliveredCount),
	)
}

// groupTargetsByNode 按 connect 节点和用户两级分组，并保持节点输出稳定。
// 同一 user+node 只要存在完整用户目标，就可对该组使用 PushToUser；完整目标本身已覆盖
// 该节点上快照中的所有设备，因此与同组的精确设备目标取并集仍不会扩大业务投递范围。
func groupTargetsByNode(targets []deliveryTarget) []*nodeFanout {
	nodeIndexes := make(map[string]int)
	nodes := make([]*nodeFanout, 0)
	for _, target := range targets {
		node := findOrAppendNode(target.Route.ConnectGRPCAddr, nodeIndexes, &nodes)
		user := node.findOrAppendUser(target.Route.UserUUID)
		user.routes = append(user.routes, target.Route)
		// OR 合并仅发生在同一节点的同一用户组内；跨节点安全性已在去重阶段隔离。
		user.allowUserPush = user.allowUserPush || target.AllowUserPush
		node.deviceCount++
	}
	sort.Slice(nodes, func(left, right int) bool {
		return nodes[left].connectAddr < nodes[right].connectAddr
	})
	return nodes
}

// findOrAppendNode 查找节点分组，不存在时按首次出现顺序创建。
// 索引 map 避免大群路由按节点分组时退化成 O(n²) 扫描。
func findOrAppendNode(
	connectAddr string,
	nodeIndexes map[string]int,
	nodes *[]*nodeFanout,
) *nodeFanout {
	if index, exists := nodeIndexes[connectAddr]; exists {
		return (*nodes)[index]
	}
	node := &nodeFanout{
		connectAddr: connectAddr,
		userIndexes: make(map[string]int),
	}
	nodeIndexes[connectAddr] = len(*nodes)
	*nodes = append(*nodes, node)
	return node
}

// findOrAppendUser 查找节点内用户分组，不存在时创建。
// 用户索引只属于当前节点，确保同一用户分布在多个 connect 时分别调用对应节点。
func (n *nodeFanout) findOrAppendUser(userUUID string) *userFanout {
	if index, exists := n.userIndexes[userUUID]; exists {
		return n.users[index]
	}
	user := &userFanout{userUUID: userUUID}
	n.userIndexes[userUUID] = len(n.users)
	n.users = append(n.users, user)
	return user
}

// add 合并单节点结果，互斥锁只保护短小的统计更新。
// 统计集中写入可避免在每次设备 RPC 后竞争全局锁。
func (a *fanoutAccumulator) add(result nodeFanoutResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.summary.SuccessCount += result.SuccessCount
	a.summary.FailedCount += result.FailedCount
	a.summary.Nodes = append(a.summary.Nodes, result)
}

// snapshot 返回稳定排序的最终统计副本。
// 节点完成顺序受网络时延影响，排序后日志与测试输出不会随机抖动。
func (a *fanoutAccumulator) snapshot() fanoutSummary {
	a.mu.Lock()
	defer a.mu.Unlock()
	summary := a.summary
	summary.Nodes = append([]nodeFanoutResult(nil), a.summary.Nodes...)
	sort.Slice(summary.Nodes, func(left, right int) bool {
		return summary.Nodes[left].ConnectAddr < summary.Nodes[right].ConnectAddr
	})
	return summary
}
