package consumer

import "github.com/segmentio/kafka-go"

func newKafkaReader(brokers []string, topic, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		MinBytes: 1,
		MaxBytes: 10 << 20,
	})
}
