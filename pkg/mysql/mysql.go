package mysql

import (
	"errors"
	"strings"

	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	gosqlmysql "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	gmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

var global *gorm.DB

// DB 返回已初始化的全局 *gorm.DB（未初始化时为 nil）。
func DB() *gorm.DB { return global }

// ReplaceGlobal 设置全局 *gorm.DB，便于全局调用 DB()。
func ReplaceGlobal(db *gorm.DB) { global = db }

// Build 基于配置初始化 GORM，并注册读写分离：
// - 当前读/写同库；若配置了从库 DSN，将注册 dbresolver 以区分读写。
// - 连接池参数、日志级别、慢查询阈值等在此集中设置。
func Build(log *zap.Logger, cfg config.MySQLConfig) (*gorm.DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("mysql dsn is empty")
	}

	// 使用当前服务显式注入的 zap logger，避免绑定尚未安装的进程级全局 logger。
	gormLog := newGormLogger(log, cfg.LogLevel)

	// 为每个 DSN 补齐 socket 级超时（timeout/readTimeout/writeTimeout）。
	// 这是「DB 挂死导致消费者分区永久冻结」的根因兜底：与 ctx 无关，由驱动在 socket 层强制。
	writeDSN := ensureDSNTimeouts(cfg.DSN, cfg)

	db, err := gorm.Open(gmysql.Open(writeDSN), &gorm.Config{
		Logger:                                   gormLog,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}

	// 组装读库列表；为空则回退到主库，实现「形式上读写分离，实际同库」。
	var readDialectors []gorm.Dialector
	for _, ro := range cfg.ReadOnlyDSNs {
		if dsn := strings.TrimSpace(ro); dsn != "" {
			readDialectors = append(readDialectors, gmysql.Open(ensureDSNTimeouts(dsn, cfg)))
		}
	}

	if len(readDialectors) == 0 {
		readDialectors = append(readDialectors, gmysql.Open(writeDSN))
	}

	// 注册 dbresolver，实现读写分离策略（默认随机）。
	if err := db.Use(dbresolver.Register(dbresolver.Config{
		Sources:  []gorm.Dialector{gmysql.Open(writeDSN)}, // 写库（主）
		Replicas: readDialectors,                          // 读库（从），当前回退主库
		Policy:   dbresolver.RandomPolicy{},               // 读流量分配策略
	})); err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 连接池参数。
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxIdle > 0 {
		sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdle)
	}
	if cfg.ConnMaxLife > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLife)
	}

	return db, nil
}

// ensureDSNTimeouts 为 DSN 补齐驱动级 socket 超时（timeout/readTimeout/writeTimeout）。
//
// 这是防止「DB 挂死（主库 failover 后的半开连接、网络黑洞、长锁等待、DB 宕机时新建连接）
// 拖住调用方乃至冻结消费者分区」的根因兜底——它在 socket 层强制生效，与 ctx 是否带 deadline 无关。
//
// 仅在 DSN 未显式指定对应超时时才补默认值，因此运维可在 DSN 里写更宽的值（如迁移/大表场景）来覆盖。
// 解析失败时保持原 DSN 不变，绝不阻断启动（最坏退回到无显式超时，由调用方 ctx 兜底）。
func ensureDSNTimeouts(dsn string, cfg config.MySQLConfig) string {
	parsed, err := gosqlmysql.ParseDSN(dsn)
	if err != nil {
		logger.Warn(nil, "解析 MySQL DSN 失败，跳过 socket 超时补齐",
			logger.ErrorField("error", err),
		)
		return dsn
	}
	if parsed.Timeout == 0 && cfg.ConnTimeout > 0 {
		parsed.Timeout = cfg.ConnTimeout
	}
	if parsed.ReadTimeout == 0 && cfg.ReadTimeout > 0 {
		parsed.ReadTimeout = cfg.ReadTimeout
	}
	if parsed.WriteTimeout == 0 && cfg.WriteTimeout > 0 {
		parsed.WriteTimeout = cfg.WriteTimeout
	}
	return parsed.FormatDSN()
}
