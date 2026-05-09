package grpcx

import "os"

// EnableDevelopmentReflection 统一约定各服务是否开启 gRPC reflection。
// 当前仓库使用 GIN_MODE 作为进程运行环境标识：空值或 debug 视为开发态，其他模式默认关闭。
func EnableDevelopmentReflection() bool {
	ginMode := os.Getenv("GIN_MODE")
	return ginMode == "" || ginMode == "debug"
}
