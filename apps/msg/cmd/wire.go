//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

func initializeMsgApp() (*MsgApp, error) {
	wire.Build(msgAppProviderSet)
	return nil, nil
}
