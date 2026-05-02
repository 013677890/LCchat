package config

import "time"

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers []string `json:"brokers" yaml:"brokers"` // Kafka broker 地址列表

	// Producer 配置
	ProducerConfig KafkaProducerConfig `json:"producer" yaml:"producer"`

	// Consumer 配置
	ConsumerConfig KafkaConsumerConfig `json:"consumer" yaml:"consumer"`

	// Redis 重试队列配置
	RedisRetryTopic     string `json:"redisRetryTopic" yaml:"redisRetryTopic"`         // Redis 重试队列 topic
	AccountDeletedTopic string `json:"accountDeletedTopic" yaml:"accountDeletedTopic"` // 账号注销事件 topic
	MsgPushTopic        string `json:"msgPushTopic" yaml:"msgPushTopic"`               // 消息推送 topic（msg-service → Push-Job）
}

// KafkaProducerConfig Kafka 生产者配置
type KafkaProducerConfig struct {
	BatchSize    int           `json:"batchSize" yaml:"batchSize"`       // 批量发送大小
	BatchTimeout time.Duration `json:"batchTimeout" yaml:"batchTimeout"` // 批量发送超时
	MaxAttempts  int           `json:"maxAttempts" yaml:"maxAttempts"`   // 最大重试次数
	WriteTimeout time.Duration `json:"writeTimeout" yaml:"writeTimeout"` // 写入超时
}

// KafkaConsumerConfig Kafka 消费者配置
type KafkaConsumerConfig struct {
	GroupID           string        `json:"groupId" yaml:"groupId"`                     // 消费者组 ID
	MinBytes          int           `json:"minBytes" yaml:"minBytes"`                   // 最小读取字节数
	MaxBytes          int           `json:"maxBytes" yaml:"maxBytes"`                   // 最大读取字节数
	MaxWait           time.Duration `json:"maxWait" yaml:"maxWait"`                     // 最大等待时间
	CommitInterval    time.Duration `json:"commitInterval" yaml:"commitInterval"`       // 提交间隔
	StartOffset       int64         `json:"startOffset" yaml:"startOffset"`             // 起始偏移量 (-1:最新, -2:最早)
	HeartbeatInterval time.Duration `json:"heartbeatInterval" yaml:"heartbeatInterval"` // 心跳间隔
	SessionTimeout    time.Duration `json:"sessionTimeout" yaml:"sessionTimeout"`       // 会话超时
	RebalanceTimeout  time.Duration `json:"rebalanceTimeout" yaml:"rebalanceTimeout"`   // 重平衡超时
}

// DefaultKafkaConfig 返回本地开发的默认配置
func DefaultKafkaConfig() KafkaConfig {
	brokers := splitCSV(getenvString("KAFKA_BROKERS", "kafka:9092"))
	if len(brokers) == 0 {
		brokers = []string{"kafka:9092"}
	}

	return KafkaConfig{
		Brokers:             brokers,
		RedisRetryTopic:     getenvString("KAFKA_RETRY_TOPIC", "redis-retry-queue"),
		AccountDeletedTopic: getenvString("KAFKA_ACCOUNT_DELETED_TOPIC", "account.deleted"),
		MsgPushTopic:        getenvString("KAFKA_MSG_PUSH_TOPIC", "msg.push"),

		ProducerConfig: KafkaProducerConfig{
			BatchSize:    100,
			BatchTimeout: 10 * time.Millisecond,
			MaxAttempts:  3,
			WriteTimeout: 10 * time.Second,
		},

		ConsumerConfig: KafkaConsumerConfig{
			GroupID:           getenvString("KAFKA_RETRY_GROUP_ID", "redis-retry-consumer-group"),
			MinBytes:          1,        // 1 字节即可触发读取
			MaxBytes:          10 << 20, // 10MB
			MaxWait:           500 * time.Millisecond,
			CommitInterval:    1 * time.Second,
			StartOffset:       -1, // 从最新消息开始消费
			HeartbeatInterval: 3 * time.Second,
			SessionTimeout:    10 * time.Second,
			RebalanceTimeout:  60 * time.Second,
		},
	}
}
