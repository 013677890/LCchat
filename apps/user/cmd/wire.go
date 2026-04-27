//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

func initializeUserApp() (*UserApp, error) {
	wire.Build(userAppProviderSet)
	return nil, nil
}
