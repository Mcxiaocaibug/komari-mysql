package flags

import "strings"

const (
	DatabaseTypeSQLite = "sqlite"
	DatabaseTypeMySQL  = "mysql"
)

var (
	// 数据库配置
	DatabaseType string // 数据库类型：sqlite, mysql
	DatabaseFile string // SQLite数据库文件路径
	DatabaseDSN  string // MySQL DSN；设置后优先于拆分连接参数
	DatabaseHost string // MySQL 主机地址
	DatabasePort string // MySQL 端口
	DatabaseUser string // MySQL 用户名
	DatabasePass string // MySQL 密码
	DatabaseName string // MySQL 数据库名

	Listen string
)

func NormalizeDatabaseType(databaseType string) string {
	databaseType = strings.ToLower(strings.TrimSpace(databaseType))
	if databaseType == "" {
		return DatabaseTypeSQLite
	}
	return databaseType
}

func ApplyDatabaseTypeNormalization() string {
	DatabaseType = NormalizeDatabaseType(DatabaseType)
	return DatabaseType
}

func IsSQLite() bool {
	return NormalizeDatabaseType(DatabaseType) == DatabaseTypeSQLite
}

func IsMySQL() bool {
	return NormalizeDatabaseType(DatabaseType) == DatabaseTypeMySQL
}

func SupportedDatabaseTypes() string {
	return DatabaseTypeSQLite + ", " + DatabaseTypeMySQL
}
