package kafka

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewProducerConfiguresReliableKeyedWriter 验证 Producer 默认采用可靠确认与 key 感知分区策略。
func TestNewProducerConfiguresReliableKeyedWriter(t *testing.T) {
	producer := NewProducer([]string{"127.0.0.1:9092"}, "test-topic")

	assert.Equal(t, kafkago.RequireAll, producer.writer.RequiredAcks)
	_, ok := producer.writer.Balancer.(*keyAwareBalancer)
	require.True(t, ok)
}

// TestKeyAwareBalancerKeepsSameKeyInSamePartition 验证同 key 消息稳定落到同一分区。
func TestKeyAwareBalancerKeepsSameKeyInSamePartition(t *testing.T) {
	balancer := newKeyAwareBalancer()
	msg := kafkago.Message{Key: []byte("conv-1")}
	partitions := []int{0, 1, 2, 3}

	first := balancer.Balance(msg, partitions...)
	for i := 0; i < 10; i++ {
		assert.Equal(t, first, balancer.Balance(msg, partitions...))
	}
}
