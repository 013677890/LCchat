package mysql

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"ChatServer/config"
	"ChatServer/pkg/logger"

	"go.uber.org/zap"
	gmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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
func Build(cfg config.MySQLConfig) (*gorm.DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("mysql dsn is empty")
	}

	// 构建 gorm 日志（默认走 stdout；若已有 zap 全局 logger，复用 zap）。
	gormLog := newGormLogger(cfg.LogLevel)

	db, err := gorm.Open(gmysql.Open(cfg.DSN), &gorm.Config{
		Logger: gormLog,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}

	// 组装读库列表；为空则回退到主库，实现「形式上读写分离，实际同库」。
	var readDialectors []gorm.Dialector
	for _, ro := range cfg.ReadOnlyDSNs {
		if dsn := strings.TrimSpace(ro); dsn != "" {
			readDialectors = append(readDialectors, gmysql.Open(dsn))
		}
	}
	if len(readDialectors) == 0 {
		readDialectors = append(readDialectors, gmysql.Open(cfg.DSN))
	}

	// 注册 dbresolver，实现读写分离策略（默认随机）。
	if err := db.Use(dbresolver.Register(dbresolver.Config{
		Sources:  []gorm.Dialector{gmysql.Open(cfg.DSN)}, // 写库（主）
		Replicas: readDialectors,                         // 读库（从），当前回退主库
		Policy:   dbresolver.RandomPolicy{},              // 读流量分配策略
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

// newGormLogger 构造 gorm logger，优先复用 zap 的全局 logger。
func newGormLogger(level string) gormlogger.Interface {
	logLevel := parseLogLevel(level)

	var base *log.Logger
	if zl := logger.L(); zl != nil {
		base = zapAsStdLog(zl)
	} else {
		// 退化到 stdout 打印
		base = log.New(os.Stdout, "gorm ", log.LstdFlags)
	}

	return gormlogger.New(
		base,
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond, // 慢查询阈值
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true, // 避免打印完整 SQL 参数
		},
	)
}

// parseLogLevel 将字符串解析为 gorm 日志级别，默认 warn。
func parseLogLevel(level string) gormlogger.LogLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "info":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}

// zapAsStdLog 将 zap Logger 转换为 *log.Logger，供 gorm logger 使用。
func zapAsStdLog(zl *zap.Logger) *log.Logger {
	return zap.NewStdLog(zl)
}
