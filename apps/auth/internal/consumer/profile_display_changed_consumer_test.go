package consumer

import (
	"context"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
)

func TestProfileDisplayChangedConsumerTreatsInvalidPayloadAsPermanent(t *testing.T) {
	consumer := &ProfileDisplayChangedConsumer{}
	err := consumer.handle(context.Background(), []byte("{"))
	if err == nil || !kafka.IsPermanent(err) {
		t.Fatalf("非法 payload 应进入死信语义: %v", err)
	}
}
