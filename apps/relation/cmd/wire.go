//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

func initializeRelationApp() (*RelationApp, error) {
	wire.Build(relationAppProviderSet)
	return nil, nil
}
