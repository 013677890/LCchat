//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

// initializeGroupApp 是 group-service 的依赖注入入口。
//
// 设计上保持和 auth / user / relation / msg 一致：
//  1. 手写 wire.go 只描述依赖图，不包含实际构造细节；
//  2. 生成后的 wire_gen.go 才是编译期真正参与构建的实现；
//  3. 这样既方便阅读依赖结构，也方便后续扩展 provider set。
func initializeGroupApp() (*GroupApp, error) {
	wire.Build(groupAppProviderSet)
	return nil, nil
}
