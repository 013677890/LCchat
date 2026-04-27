//go:build wireinject
// +build wireinject

package main

import "github.com/google/wire"

// initializeGatewayApp 负责把 gateway 的配置、基础设施、gRPC 客户端、Service、Handler、Router 与 App 生命周期对象串起来。
//
// 约束：
// - 这里只描述依赖关系，不做任何真正的网络监听；
// - 真正的启动动作放到 GatewayApp.Run；
// - 这样 Wire 只负责“装配”，不负责“运行”。
func initializeGatewayApp() (*GatewayApp, error) {
	wire.Build(gatewayAppProviderSet)
	return nil, nil
}
