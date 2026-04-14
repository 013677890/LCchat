//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

func initializeConnectApp() (*ConnectApp, error) {
	wire.Build(connectProviderSet)
	return nil, nil
}
