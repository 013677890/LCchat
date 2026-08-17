package consumer

import (
	"context"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
)

func TestUserCreatedConsumerTreatsInvalidPayloadAsPermanent(t *testing.T) {
	consumer := &UserCreatedConsumer{}
	err := consumer.handle(context.Background(), []byte("{"))
	if err == nil || !kafka.IsPermanent(err) {
		t.Fatalf("非法 user_created payload 应进入死信语义: %v", err)
	}
}

func TestAccountDeletedConsumerTreatsInvalidPayloadAsPermanent(t *testing.T) {
	consumer := &AccountDeletedConsumer{}
	err := consumer.handle(context.Background(), []byte("{"))
	if err == nil || !kafka.IsPermanent(err) {
		t.Fatalf("非法 account.deleted payload 应进入死信语义: %v", err)
	}
}
