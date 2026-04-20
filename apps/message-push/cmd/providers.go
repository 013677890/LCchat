package main

import (
	"os"
	"time"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/consumer"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/google/wire"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type messagePushRouteTTL time.Duration

type messagePushConnectUserTimeout time.Duration

func provideMessagePushLoggerConfig() config.LoggerConfig {
	return config.DefaultLoggerConfig()
}

func provideMessagePushRedisConfig() config.RedisConfig {
	return config.DefaultRedisConfig()
}

func provideMessagePushKafkaConfig() config.KafkaConfig {
	return config.DefaultKafkaConfig()
}

func provideMessagePushLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	return logger.Build(cfg)
}

func provideMessagePushRedisClient(_ *zap.Logger, cfg config.RedisConfig) (*goredis.Client, error) {
	return pkgredis.Build(cfg)
}

func provideMessagePushGroupID() string {
	if v := os.Getenv("KAFKA_MSG_PUSH_GROUP_ID"); v != "" {
		return v
	}
	return "message-push-consumer-group"
}

func provideMessagePushRouteTTL() messagePushRouteTTL {
	if v := os.Getenv("MESSAGE_PUSH_ROUTE_TTL_SECONDS"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			return messagePushRouteTTL(d)
		}
	}
	return messagePushRouteTTL(180 * time.Second)
}

func provideMessagePushConnectUserTimeout() messagePushConnectUserTimeout {
	if v := os.Getenv("MESSAGE_PUSH_CONNECT_TIMEOUT_USER_MS"); v != "" {
		if d, err := time.ParseDuration(v + "ms"); err == nil {
			return messagePushConnectUserTimeout(d)
		}
	}
	return messagePushConnectUserTimeout(150 * time.Millisecond)
}

func provideRouteRepository(client *goredis.Client, ttl messagePushRouteTTL) *route.RedisRepository {
	return route.NewRedisRepository(client, time.Duration(ttl))
}

func provideConnectClientManager() *connectcli.ClientManager {
	return connectcli.NewClientManager()
}

func provideConnectSender(manager *connectcli.ClientManager, timeout messagePushConnectUserTimeout) *connectcli.Sender {
	return connectcli.NewSender(manager, time.Duration(timeout))
}

func provideEventHandler(routes *route.RedisRepository, sender *connectcli.Sender) *consumer.EventHandler {
	return consumer.NewEventHandler(routes, sender)
}

func providePushConsumer(cfg config.KafkaConfig, groupID string, handler *consumer.EventHandler) *consumer.Consumer {
	return consumer.NewConsumer(cfg.Brokers, cfg.MsgPushTopic, groupID, handler)
}

var messagePushProviderSet = wire.NewSet(
	provideMessagePushLoggerConfig,
	provideMessagePushRedisConfig,
	provideMessagePushKafkaConfig,
	provideMessagePushLogger,
	provideMessagePushRedisClient,
	provideMessagePushGroupID,
	provideMessagePushRouteTTL,
	provideMessagePushConnectUserTimeout,
	provideRouteRepository,
	provideConnectClientManager,
	provideConnectSender,
	provideEventHandler,
	providePushConsumer,
	NewMessagePushApp,
)
