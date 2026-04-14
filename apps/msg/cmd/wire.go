//go:build wireinject
// +build wireinject

package main

import (
	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/msg/internal/usecase"
	"github.com/google/wire"
)

func initializeMsgApp() (*MsgApp, error) {
	wire.Build(
		provideLoggerConfig,
		provideAsyncConfig,
		provideMySQLConfig,
		provideRedisConfig,
		provideKafkaConfig,
		provideLogger,
		provideAsyncPool,
		provideAsyncReleaseTimeout,
		provideMySQLDB,
		provideRedisClient,
		provideKafkaProducer,
		provideMsgProducer,
		provideMsgConfig,
		provideMsgService,
		provideGRPCAddress,
		provideMetricsAddress,
		provideGRPCShutdownTimeout,
		provideSnowflakeNode,
		provideMsgRegistration,
		provideMsgGRPCServer,
		provideMsgGRPCListener,
		provideMetricsServer,
		message.NewRepository,
		conversation.NewRepository,
		conversation.NewService,
		usecase.NewSendMessageWorkflow,
		usecase.NewRecallMessageWorkflow,
		usecase.NewMarkReadWorkflow,
		handler.NewMsgHandler,
		NewMsgApp,
	)
	return nil, nil
}
