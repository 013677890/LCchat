//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

// initializeMessagePushApp 定义 Wire 注入入口。
// 实际初始化代码由 `wire` 命令生成到 `wire_gen.go`。
func initializeMessagePushApp() (*MessagePushApp, error) {
	wire.Build(messagePushProviderSet)
	return nil, nil
}
