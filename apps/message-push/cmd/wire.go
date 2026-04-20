//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

func initializeMessagePushApp() (*MessagePushApp, error) {
	wire.Build(messagePushProviderSet)
	return nil, nil
}
