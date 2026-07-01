package config

import "time"

// MySQLConfig 描述 MySQL 连接与读写分离（可选）的基础参数。
// 本项目当前读/写同库，可提前预留从库 DSN，后续接入只需改配置。
type MySQLConfig struct {
	// 基础连接
	DSN          string        `json:"dsn" yaml:"dsn"`                   // 主库 DSN（必须）
	ReadOnlyDSNs []string      `json:"readOnlyDsns" yaml:"readOnlyDsns"` // 从库 DSN 列表（可为空，默认回退主库）
	MaxOpenConns int           `json:"maxOpenConns" yaml:"maxOpenConns"` // 最大打开连接数
	MaxIdleConns int           `json:"maxIdleConns" yaml:"maxIdleConns"` // 最大空闲连接数
	ConnMaxIdle  time.Duration `json:"connMaxIdle" yaml:"connMaxIdle"`   // 连接最大空闲时间
	ConnMaxLife  time.Duration `json:"connMaxLife" yaml:"connMaxLife"`   // 连接最长存活时间
	LogLevel     string        `json:"logLevel" yaml:"logLevel"`         // gorm 日志级别: silent|error|warn|info

	// 驱动 socket 级超时：与 ctx deadline 无关，由 go-sql-driver 在 socket 层强制执行。
	// 这是防止「DB 挂死(半开连接/网络黑洞/长锁)导致消费者分区永久冻结」的根因兜底，
	// 缺省由 pkg/mysql 在构建 DSN 时按需补齐；DSN 里已显式写明的值优先生效，便于运维按表/迁移调宽。
	ConnTimeout  time.Duration `json:"connTimeout" yaml:"connTimeout"`   // 建连(dial)超时，对应 DSN timeout
	ReadTimeout  time.Duration `json:"readTimeout" yaml:"readTimeout"`   // socket 读超时，对应 DSN readTimeout
	WriteTimeout time.Duration `json:"writeTimeout" yaml:"writeTimeout"` // socket 写超时，对应 DSN writeTimeout
}

// DefaultMySQLConfig 返回便于本地开发的默认配置：读写同一个 DSN。
func DefaultMySQLConfig() MySQLConfig {
	dsn := getenvString("MYSQL_DSN", "")
	if dsn == "" {
		user := getenvString("MYSQL_USER", "root")
		password := getenvString("MYSQL_PASSWORD", "root")
		host := getenvString("MYSQL_HOST", "mysql")
		port := getenvString("MYSQL_PORT", "3306")
		database := getenvString("MYSQL_DATABASE", "chat_server")
		dsn = user + ":" + password + "@tcp(" + host + ":" + port + ")/" + database + "?charset=utf8mb4&parseTime=True&loc=Local"
	}

	return MySQLConfig{
		// 优先使用环境变量 MYSQL_DSN，其次按 MYSQL_HOST/MYSQL_PORT/... 组装
		DSN:          dsn,
		ReadOnlyDSNs: []string{},
		MaxOpenConns: 50,
		MaxIdleConns: 10,
		ConnMaxIdle:  10 * time.Minute,
		ConnMaxLife:  1 * time.Hour,
		LogLevel:     getenvString("MYSQL_LOG_LEVEL", "warn"),
		// IM 查询应为亚秒级：读写 10s 给足余量又能把 DB 挂死收敛到秒级；建连 5s 兜住「DB 宕机时新建连接卡死」。
		// 注意：在线 DDL / 大表迁移可能超过 readTimeout，迁移须走单独连接或临时调宽。
		ConnTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}
