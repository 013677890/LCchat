package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// KafkaFetchDuration Kafka 拉取消息耗时（秒）
	KafkaFetchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "message_push",
		Subsystem: "kafka",
		Name:      "fetch_duration_seconds",
		Help:      "Kafka 拉取消息耗时分布",
		Buckets:   prometheus.DefBuckets,
	})

	// KafkaCommitErrors Kafka offset 提交失败计数
	KafkaCommitErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "message_push",
		Subsystem: "kafka",
		Name:      "commit_errors_total",
		Help:      "Kafka offset 提交失败总数",
	})

	// HandleDuration 单条消息处理耗时（秒）
	HandleDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "message_push",
		Subsystem: "handler",
		Name:      "duration_seconds",
		Help:      "单条消息处理耗时分布",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"result"}) // result: success / retriable_error / permanent_error

	// HandleRetries 消息处理重试次数分布
	HandleRetries = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "message_push",
		Subsystem: "handler",
		Name:      "retries",
		Help:      "单条消息本地重试次数分布",
		Buckets:   []float64{0, 1, 2, 3},
	}, []string{"final_result"}) // final_result: success / failed

	// EventTypeSkipped 跳过的事件类型计数
	EventTypeSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "message_push",
		Subsystem: "handler",
		Name:      "event_type_skipped_total",
		Help:      "按事件类型跳过的消息总数",
	}, []string{"event_type", "reason"}) // reason: unsupported_type / unsupported_conv_type / validation_error

	// RouteHitRate 路由命中情况
	RouteHitRate = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "message_push",
		Subsystem: "route",
		Name:      "lookup_total",
		Help:      "路由查询结果统计",
	}, []string{"event_type", "result"}) // event_type: MSG_PUSH / MSG_RECALL / MSG_MARK_READ, result: hit / miss / error

	// PushToDeviceDuration connect PushToDevice RPC 耗时（秒）
	PushToDeviceDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "message_push",
		Subsystem: "connect",
		Name:      "push_to_device_duration_seconds",
		Help:      "connect PushToDevice RPC 耗时分布",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .15, .2, .3, .5},
	}, []string{"result"}) // result: success / error

	// PushToDeviceTotal connect PushToDevice 调用计数
	PushToDeviceTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "message_push",
		Subsystem: "connect",
		Name:      "push_to_device_total",
		Help:      "connect PushToDevice 调用总数",
	}, []string{"event_type", "result"}) // event_type: MSG_PUSH / MSG_RECALL / MSG_MARK_READ, result: success / error

	// DeliveredDevices 成功投递的设备数
	DeliveredDevices = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "message_push",
		Subsystem: "handler",
		Name:      "delivered_devices",
		Help:      "单条消息成功投递的设备数分布",
		Buckets:   []float64{0, 1, 2, 3, 5, 10, 20},
	})

	// MessagesDroppedAfterRetry 本地重试耗尽后仍失败、被迫提交 offset 而丢弃的消息计数（按事件类型）。
	// msg.push 是 best-effort 实时通道，被丢弃的事件依赖客户端按 seq 拉取兜底；
	// 但本指标持续非零通常意味着 Redis 路由或 connect 节点持续异常，应作为告警信号
	// （区别于"用户离线"这类正常跳过——后者根本不会进入重试/丢弃路径）。
	MessagesDroppedAfterRetry = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "message_push",
		Subsystem: "handler",
		Name:      "dropped_after_retry_total",
		Help:      "本地重试耗尽后被迫提交 offset 而丢弃的消息总数（依赖客户端 seq 拉取兜底）",
	}, []string{"event_type"}) // event_type: MSG_PUSH / MSG_RECALL / MSG_MARK_READ / MSG_READ_RECEIPT / unknown
)

// ObserveHandleDuration 记录处理耗时
func ObserveHandleDuration(start time.Time, result string) {
	HandleDuration.WithLabelValues(result).Observe(time.Since(start).Seconds())
}

// ObservePushToDeviceDuration 记录 PushToDevice 耗时
func ObservePushToDeviceDuration(start time.Time, result string) {
	PushToDeviceDuration.WithLabelValues(result).Observe(time.Since(start).Seconds())
}
