//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

func initializeAuthApp() (*AuthApp, error) {
	wire.Build(authAppProviderSet)
	return nil, nil
}
