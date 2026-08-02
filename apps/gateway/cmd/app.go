package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
	pkgredis "github.com/013677890/LCchat-Backend/pkg/redis"
	"github.com/panjf2000/ants/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// GatewayApp 统一承接 gateway 进程级生命周期。
//
// 设计约束：
// 1. 构造阶段只负责“拿到对象并校验”，不启动任何阻塞逻辑；
// 2. Run 阶段只负责启动 HTTP 对外入口；
// 3. Shutdown 阶段严格按“先停入口、再停后台同步、最后释放底层资源”的顺序回收；
// 4. 这样 main.go 只保留信号监听与 Run/Shutdown 编排，避免重新回到手工装配时代。
type GatewayApp struct {
	logger              *zap.Logger
	httpServer          *http.Server
	asyncPool           *ants.Pool
	asyncReleaseTimeout time.Duration
	redisClient         *goredis.Client
	authServiceConn     gatewayAuthConn
	userServiceConn     gatewayUserConn
	relationServiceConn gatewayRelationConn
	msgServiceConn      gatewayMsgConn
	groupServiceConn    gatewayGroupConn
	minioClient         *pkgminio.MinIOClient
}

// NewGatewayApp 只做资源聚合与基本合法性校验。
//
// 注意：
//   - 这里不启动 HTTP Server；
//   - 这里不启动 goroutine；
//   - 这里不做阻塞等待；
//   - provider 中允许降级为 nil 的资源（如 Redis / MinIO）会被保留到 App 中，
//     由运行期逻辑决定是否启用对应能力。
func NewGatewayApp(
	log *zap.Logger,
	httpServer *http.Server,
	asyncPool *ants.Pool,
	asyncReleaseTimeout gatewayAsyncReleaseTimeout,
	redisClient *goredis.Client,
	authServiceConn gatewayAuthConn,
	userServiceConn gatewayUserConn,
	relationServiceConn gatewayRelationConn,
	msgServiceConn gatewayMsgConn,
	groupServiceConn gatewayGroupConn,
	minioClient *pkgminio.MinIOClient,
) (*GatewayApp, error) {
	if log == nil {
		return nil, errors.New("gateway logger 未初始化")
	}
	if httpServer == nil {
		return nil, errors.New("gateway http server 未初始化")
	}
	if asyncPool == nil {
		return nil, errors.New("gateway async pool 未初始化")
	}
	if authServiceConn.value == nil {
		return nil, errors.New("gateway auth-service gRPC 连接未初始化")
	}
	if userServiceConn.value == nil {
		return nil, errors.New("gateway user-service gRPC 连接未初始化")
	}
	if relationServiceConn.value == nil {
		return nil, errors.New("gateway relation-service gRPC 连接未初始化")
	}
	if msgServiceConn.value == nil {
		return nil, errors.New("gateway msg-service gRPC 连接未初始化")
	}
	if groupServiceConn.value == nil {
		return nil, errors.New("gateway group-service gRPC 连接未初始化")
	}

	return &GatewayApp{
		logger:              log,
		httpServer:          httpServer,
		asyncPool:           asyncPool,
		asyncReleaseTimeout: time.Duration(asyncReleaseTimeout),
		redisClient:         redisClient,
		authServiceConn:     authServiceConn,
		userServiceConn:     userServiceConn,
		relationServiceConn: relationServiceConn,
		msgServiceConn:      msgServiceConn,
		groupServiceConn:    groupServiceConn,
		minioClient:         minioClient,
	}, nil
}

// Run 启动 gateway 的对外 HTTP 入口。
//
// 关键边界：
// - 全局 logger / Redis / 异步池 / MinIO 与限流器在 Listen 之前完成挂载；
// - 真正阻塞主 goroutine 的只有 ListenAndServe；
// - main 通过并发调用 Run + 监听系统信号实现统一编排。
func (a *GatewayApp) Run(ctx context.Context) error {
	a.installGatewayGlobals(ctx)

	logger.Info(ctx, "Gateway 服务启动中",
		logger.String("address", a.httpServer.Addr),
	)

	if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("运行 HTTP 服务失败: %w", err)
	}
	return nil
}

// Shutdown 优雅关闭 gateway，并按依赖方向回收资源。
//
// 顺序说明：
// 1. 先关 HTTP Server，阻止新请求继续进入；
// 2. 再等待异步池任务收敛，避免底层连接关闭后任务仍访问依赖；
// 3. 再释放 gRPC 连接、Redis 等基础资源；
// 4. 最后同步 logger，保证退出前尽量落盘完整日志。
func (a *GatewayApp) Shutdown(ctx context.Context) error {
	var errs []error

	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("关闭 Gateway HTTP 服务失败: %w", err))
		}
	}

	if a.asyncPool != nil {
		var err error
		if async.Pool() == a.asyncPool {
			err = async.Release()
		} else {
			err = async.ReleasePool(a.asyncPool, a.asyncReleaseTimeout)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("释放 Async 协程池失败: %w", err))
		}
	}

	if a.msgServiceConn.value != nil {
		if err := a.msgServiceConn.value.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 msg-service gRPC 连接失败: %w", err))
		}
	}

	if a.groupServiceConn.value != nil {
		if err := a.groupServiceConn.value.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 group-service gRPC 连接失败: %w", err))
		}
	}

	if a.authServiceConn.value != nil {
		if err := a.authServiceConn.value.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 auth-service gRPC 连接失败: %w", err))
		}
	}

	if a.relationServiceConn.value != nil {
		if err := a.relationServiceConn.value.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 relation-service gRPC 连接失败: %w", err))
		}
	}

	if a.userServiceConn.value != nil {
		if err := a.userServiceConn.value.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 user-service gRPC 连接失败: %w", err))
		}
	}

	if a.redisClient != nil {
		if err := a.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Redis 客户端失败: %w", err))
		}
	}

	if a.logger != nil {
		_ = a.logger.Sync()
	}

	return errors.Join(errs...)
}

func (a *GatewayApp) installGatewayGlobals(ctx context.Context) {
	logger.ReplaceGlobal(a.logger)
	if a.redisClient != nil {
		pkgredis.ReplaceGlobal(a.redisClient)
	}
	async.SetContextPropagator(func(parent context.Context) context.Context {
		return ctxmeta.ChildContextFromParent(parent)
	})
	async.ReplaceGlobalWithReleaseTimeout(a.asyncPool, a.asyncReleaseTimeout)
	if a.minioClient != nil {
		pkgminio.ReplaceGlobal(a.minioClient)
	}

	middleware.InitRedisRateLimiter(10.0, 20, a.redisClient)
	logger.Info(ctx, "Redis IP 限流器初始化完成",
		logger.Float64("rate", 10.0),
		logger.Int("burst", 20),
		logger.String("blacklist_key", rediskey.GatewayIPBlacklistKey()),
	)
}
