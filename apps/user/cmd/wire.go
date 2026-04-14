//go:build wireinject
// +build wireinject

package main

import (
	"github.com/013677890/LCchat-Backend/apps/user/internal/handler"
	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	"github.com/google/wire"
)

func initializeUserApp() (*UserApp, error) {
	wire.Build(
		provideUserLoggerConfig,
		provideUserAsyncConfig,
		provideUserMySQLConfig,
		provideUserRedisConfig,
		provideUserKafkaConfig,
		provideUserLogger,
		provideUserAsyncPool,
		provideUserAsyncReleaseTimeout,
		provideUserMySQLDB,
		provideUserRedisClient,
		provideUserKafkaProducer,
		provideUserRedisRetryConsumer,
		provideUserGRPCAddress,
		provideUserMetricsAddress,
		provideUserGRPCShutdownTimeout,
		provideUserMetricsServer,
		provideUserSnowflakeNode,
		provideVerifyEmailConfig,
		provideDeviceActiveConfig,
		provideUserRegistration,
		provideUserGRPCServer,
		provideUserGRPCListener,
		repository.NewAuthRepository,
		repository.NewUserRepository,
		repository.NewFriendRepository,
		repository.NewApplyRepository,
		repository.NewBlacklistRepository,
		repository.NewDeviceRepository,
		service.NewAuthService,
		service.NewUserService,
		service.NewFriendService,
		service.NewBlacklistService,
		service.NewDeviceService,
		handler.NewAuthHandler,
		handler.NewUserHandler,
		handler.NewFriendHandler,
		handler.NewBlacklistHandler,
		handler.NewDeviceHandler,
		NewUserApp,
	)
	return nil, nil
}
